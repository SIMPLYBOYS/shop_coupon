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

// reservationListener listens for reservation requests and handles them.
// This is a method on Server to access timeConfig via dependency injection.
func (s *Server) reservationListener(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()
			if now < s.timeConfig.startReserveTime.Load() || now >= s.timeConfig.endReserveTime.Load() {
				continue
			}
			handleReservations(ctx, s.store)
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
// Uses batch operations with transaction protection to ensure atomicity:
// - Batch INSERT for creating coupons
// - Batch UPDATE for marking reservations as processed
// Both operations are wrapped in a transaction to prevent data inconsistency
func generateCouponsForReservations(ctx context.Context, store *db.Store, reservations []db.CouponReservations, numCoupons int) int {
	if len(reservations) == 0 {
		return 0
	}

	// Determine how many coupons we can actually create
	couponsToCreate := numCoupons
	if couponsToCreate > len(reservations) {
		couponsToCreate = len(reservations)
	}

	// Separate reservations into winners (get coupons) and non-winners (just mark processed)
	winnerReservationIDs := make([]int32, 0, couponsToCreate)
	nonWinnerReservationIDs := make([]int32, 0, len(reservations)-couponsToCreate)

	// Generate coupon codes for winners (up to numCoupons)
	couponCodes := make([]string, 0, couponsToCreate)
	failedCodeGenIDs := make([]int32, 0) // Track reservations where code generation failed

	for i := 0; i < len(reservations); i++ {
		if i < couponsToCreate {
			// This reservation should get a coupon
			couponCode, err := u.GenerateCouponCode()
			if err != nil {
				log.Printf("generateCouponsForReservations GenerateCouponCode error for reservation %d: %v", reservations[i].ID, err)
				// Don't mark as processed - allow retry on next cycle
				failedCodeGenIDs = append(failedCodeGenIDs, reservations[i].ID)
				continue
			}
			couponCodes = append(couponCodes, couponCode)
			winnerReservationIDs = append(winnerReservationIDs, reservations[i].ID)
		} else {
			// This reservation doesn't get a coupon, just mark as processed
			nonWinnerReservationIDs = append(nonWinnerReservationIDs, reservations[i].ID)
		}
	}

	couponsGenerated := 0

	// Use transaction to create coupons and mark winner reservations as processed atomically
	if len(couponCodes) > 0 {
		batchParams := db.BatchCreateCouponsParams{
			Codes:      couponCodes,
			Discount:   "0.25",                      // 25% discount
			ExpiryDate: time.Now().AddDate(0, 0, 7), // Expiry date is 7 days from now
		}

		created, err := store.BatchCreateCouponsWithTx(ctx, batchParams, winnerReservationIDs)
		if err != nil {
			log.Printf("generateCouponsForReservations BatchCreateCouponsWithTx error: %v", err)
			// Fall back to individual creation with immediate marking
			couponsGenerated = fallbackCreateCouponsIndividually(ctx, store, couponCodes, winnerReservationIDs, batchParams)
		} else {
			couponsGenerated = int(created)
			log.Printf("generateCouponsForReservations: created %d coupons and marked %d reservations in transaction", created, len(winnerReservationIDs))
		}
	}

	// Mark non-winner reservations as processed (separate from winners)
	if len(nonWinnerReservationIDs) > 0 {
		err := store.Queries.BatchMarkCouponReservationsAsProcessed(ctx, nonWinnerReservationIDs)
		if err != nil {
			log.Printf("generateCouponsForReservations BatchMarkNonWinners error: %v", err)
			// Fall back to individual marking
			successCount := 0
			for _, id := range nonWinnerReservationIDs {
				if markErr := store.Queries.MarkCouponReservationAsProcessed(ctx, id); markErr != nil {
					log.Printf("generateCouponsForReservations MarkAsProcessed fallback error for reservation %d: %v", id, markErr)
				} else {
					successCount++
				}
			}
			log.Printf("generateCouponsForReservations: fallback marked %d/%d non-winner reservations", successCount, len(nonWinnerReservationIDs))
		}
	}

	// Log failed code generations (these will be retried on next cycle)
	if len(failedCodeGenIDs) > 0 {
		log.Printf("generateCouponsForReservations: %d reservations had code generation failures, will retry next cycle", len(failedCodeGenIDs))
	}

	return couponsGenerated
}

// fallbackCreateCouponsIndividually creates coupons one by one when batch operation fails
// Each successful coupon creation immediately marks its reservation as processed
func fallbackCreateCouponsIndividually(ctx context.Context, store *db.Store, codes []string, reservationIDs []int32, params db.BatchCreateCouponsParams) int {
	couponsGenerated := 0

	for i, code := range codes {
		if i >= len(reservationIDs) {
			break
		}

		arg := db.CreateCouponParams{
			Code:       code,
			Discount:   params.Discount,
			ExpiryDate: params.ExpiryDate,
		}

		_, err := store.Queries.CreateCoupon(ctx, arg)
		if err != nil {
			log.Printf("fallbackCreateCouponsIndividually CreateCoupon error for reservation %d: %v", reservationIDs[i], err)
			// Don't mark as processed - allow retry
			continue
		}

		// Only mark as processed after successful coupon creation
		if markErr := store.Queries.MarkCouponReservationAsProcessed(ctx, reservationIDs[i]); markErr != nil {
			log.Printf("CRITICAL: coupon created but MarkAsProcessed failed for reservation %d: %v", reservationIDs[i], markErr)
		}
		couponsGenerated++
	}

	log.Printf("fallbackCreateCouponsIndividually: created %d/%d coupons", couponsGenerated, len(codes))
	return couponsGenerated
}
