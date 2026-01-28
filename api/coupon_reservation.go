package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	u "github.com/SIMPLYBOYS/shopcoupon/util"
	"github.com/gin-gonic/gin"
)

type getCouponReservationRequest struct {
	UserID int32 `uri:"user_id" binding:"required,min=1"` // Request struct for getting coupon reservation
}

// reservationListener listens for reservation requests and handles them
func reservationListener(ctx context.Context, store *db.Store) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()
			if now < couponTimeConfig.startReserveTime.Load() || now >= couponTimeConfig.endReserveTime.Load() {
				continue
			}
			handleReservations(ctx, store)
		}
	}
}

// handleReservations processes the coupon reservations
// Uses a limit to prevent DoS attacks from overwhelming the system with unlimited reservations
func handleReservations(ctx context.Context, store *db.Store) {
	const maxReservationsPerCycle = 10000

	log.Default().Printf("handleReservations ===============>")
	reservations, err := store.Queries.ListCouponReservationsWithLimit(ctx, maxReservationsPerCycle)
	if err != nil {
		log.Println("handleReservations error:", err)
		return
	}

	numReservations := len(reservations)
	numCoupons := int(float64(numReservations) * 0.2) // 20% of reservations will receive coupons

	// Process reservations in batches
	batchSize := 3000 // Adjust batch size as needed
	for i := 0; i < numReservations; i += batchSize {
		end := i + batchSize
		if end > numReservations {
			end = numReservations
		}

		couponsGenerated := generateCouponsForReservations(ctx, store, reservations[i:end], numCoupons)
		numCoupons -= couponsGenerated
	}
}

// getCouponReservation returns the coupon reservation for a given user
func (s *Server) getCouponReservation(ctx *gin.Context) {
	var req getCouponReservationRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Authorization check: ensure user can only access their own reservation
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		} else {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		}
		return
	}

	if userID != req.UserID {
		ctx.JSON(http.StatusForbidden, errorResponse(ErrAccessDenied))
		return
	}

	couponReservation, err := s.store.GetCouponReservation(ctx, req.UserID)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, couponReservation)
}

type createCouponReservationRequest struct {
	UserID int32 `json:"user_id" binding:"required,min=1"` // Request struct for creating coupon reservation
}

// createCouponReservation creates a new coupon reservation
func (s *Server) createCouponReservation(ctx *gin.Context) {
	var req createCouponReservationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Authorization check: ensure user can only create reservation for themselves
	if err := checkUserAuthorization(ctx, req.UserID); err != nil {
		return
	}

	userIDStr := strconv.FormatInt(int64(req.UserID), 10)

	// Quick rejection check using Bloom filter (read lock for better concurrency)
	s.reserveBloomMu.RLock()
	alreadyReserved := s.bloomFilterForReserve.TestString(userIDStr)
	s.reserveBloomMu.RUnlock()

	if alreadyReserved {
		ctx.JSON(http.StatusBadRequest, errorResponse(newPublicError("user already reserved")))
		return
	}

	// DB is the source of truth - unique constraint prevents duplicates
	couponReservation, err := s.store.CreateCouponReservation(ctx, req.UserID)
	if err != nil {
		// Handle unique constraint violation (user already has a reservation)
		// This catches cases where Bloom filter was cleared or race conditions occurred
		if isUniqueViolationError(err) {
			log.Printf("createCouponReservation: user %d already reserved (caught by DB, bloom filter bypass)", req.UserID)
			// Re-add to Bloom filter to prevent repeated DB hits on subsequent requests
			s.reserveBloomMu.Lock()
			s.bloomFilterForReserve.AddString(userIDStr)
			s.reserveBloomMu.Unlock()
			ctx.JSON(http.StatusBadRequest, errorResponse(newPublicError("user already reserved")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Add to Bloom filter only after successful DB operation (write lock)
	// This ensures user can retry if DB fails
	s.reserveBloomMu.Lock()
	s.bloomFilterForReserve.AddString(userIDStr)
	s.reserveBloomMu.Unlock()

	ctx.JSON(http.StatusOK, couponReservation)
}

// generateCouponsForReservations generates coupons for the given reservations
// Uses batch operations to minimize database round trips:
// - Batch INSERT for creating coupons
// - Batch UPDATE for marking reservations as processed
func generateCouponsForReservations(ctx context.Context, store *db.Store, reservations []db.CouponReservations, numCoupons int) int {
	if len(reservations) == 0 {
		return 0
	}

	// Determine how many coupons we can actually create
	couponsToCreate := numCoupons
	if couponsToCreate > len(reservations) {
		couponsToCreate = len(reservations)
	}

	// Collect all reservation IDs to mark as processed
	allReservationIDs := make([]int32, len(reservations))
	for i, r := range reservations {
		allReservationIDs[i] = r.ID
	}

	// Generate coupon codes for winners (up to numCoupons)
	couponCodes := make([]string, 0, couponsToCreate)
	couponReservationIDs := make([]int32, 0, couponsToCreate)

	for i := 0; i < couponsToCreate && i < len(reservations); i++ {
		couponCode, err := u.GenerateCouponCode()
		if err != nil {
			log.Printf("generateCouponsForReservations GenerateCouponCode error for reservation %d: %v", reservations[i].ID, err)
			continue
		}
		couponCodes = append(couponCodes, couponCode)
		couponReservationIDs = append(couponReservationIDs, reservations[i].ID)
	}

	couponsGenerated := 0

	// Batch create coupons if we have any codes
	if len(couponCodes) > 0 {
		batchParams := db.BatchCreateCouponsParams{
			Codes:      couponCodes,
			Discount:   "0.25",                      // 25% discount
			ExpiryDate: time.Now().AddDate(0, 0, 7), // Expiry date is 7 days from now
		}

		created, err := store.Queries.BatchCreateCoupons(ctx, batchParams)
		if err != nil {
			log.Printf("generateCouponsForReservations BatchCreateCoupons error: %v", err)
			// Fall back to individual creation if batch fails
			for i, code := range couponCodes {
				arg := db.CreateCouponParams{
					Code:       code,
					Discount:   batchParams.Discount,
					ExpiryDate: batchParams.ExpiryDate,
				}
				_, err := store.Queries.CreateCoupon(ctx, arg)
				if err != nil {
					log.Printf("generateCouponsForReservations CreateCoupon fallback error for reservation %d: %v", couponReservationIDs[i], err)
				} else {
					couponsGenerated++
				}
			}
		} else {
			couponsGenerated = int(created)
		}
	}

	// Batch mark all reservations as processed
	if len(allReservationIDs) > 0 {
		err := store.Queries.BatchMarkCouponReservationsAsProcessed(ctx, allReservationIDs)
		if err != nil {
			log.Printf("generateCouponsForReservations BatchMarkCouponReservationsAsProcessed error: %v", err)
			// Fall back to individual marking if batch fails
			for _, id := range allReservationIDs {
				if markErr := store.Queries.MarkCouponReservationAsProcessed(ctx, id); markErr != nil {
					log.Printf("generateCouponsForReservations MarkAsProcessed fallback error for reservation %d: %v", id, markErr)
				}
			}
		}
	}

	return couponsGenerated
}
