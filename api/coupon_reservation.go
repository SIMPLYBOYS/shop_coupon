package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
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
func handleReservations(ctx context.Context, store *db.Store) {
	log.Default().Printf("handleReservations ===============>")
	reservations, err := store.Queries.ListCouponReservations(ctx)
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

	if s.bloomFilterForReserve.TestString(userIDStr) { // Check if the user has already reserved
		ctx.JSON(http.StatusBadRequest, errorResponse(newPublicError("user already reserved")))
		return
	}
	couponReservation, err := s.store.CreateCouponReservation(ctx, req.UserID)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	s.bloomFilterForReserve.AddString(userIDStr) // Add the user to the reservation Bloom filter
	ctx.JSON(http.StatusOK, couponReservation)
}

// generateCouponsForReservations generates coupons for the given reservations
// Note: Each coupon creation is independent, so we don't use a transaction here.
// Using transactions across goroutines is problematic and not needed for this use case.
func generateCouponsForReservations(ctx context.Context, store *db.Store, reservations []db.CouponReservations, numCoupons int) int {
	couponsGenerated := 0
	var mu sync.Mutex // Mutex to protect couponsGenerated

	var wg sync.WaitGroup
	batchSize := 500 // Adjust batch size as needed
	for i := 0; i < len(reservations); i += batchSize {
		end := i + batchSize
		if end > len(reservations) {
			end = len(reservations)
		}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for j := start; j < end; j++ {
				reservation := reservations[j]

				mu.Lock()
				reachedLimit := couponsGenerated >= numCoupons
				mu.Unlock()

				if reachedLimit {
					err := store.Queries.MarkCouponReservationAsProcessed(ctx, reservation.ID)
					if err != nil {
						log.Println("generateCouponsForReservations error:", err)
					}
					continue
				}

				couponCode, err := u.GenerateCouponCode()
				if err != nil {
					log.Printf("generateCouponsForReservations GenerateCouponCode error for reservation %d: %v", reservation.ID, err)
					if markErr := store.Queries.MarkCouponReservationAsProcessed(ctx, reservation.ID); markErr != nil {
						log.Printf("generateCouponsForReservations MarkAsProcessed error for reservation %d: %v", reservation.ID, markErr)
					}
					continue
				}

				arg := db.CreateCouponParams{
					Code:       couponCode,
					Discount:   "0.25",                      // 25% discount
					ExpiryDate: time.Now().AddDate(0, 0, 7), // Expiry date is 7 days from now
				}

				_, err = store.Queries.CreateCoupon(ctx, arg)
				if err != nil {
					log.Printf("generateCouponsForReservations CreateCoupon error for reservation %d: %v", reservation.ID, err)
					// Mark as processed to prevent infinite retry loop
					if markErr := store.Queries.MarkCouponReservationAsProcessed(ctx, reservation.ID); markErr != nil {
						log.Printf("generateCouponsForReservations MarkAsProcessed error for reservation %d: %v", reservation.ID, markErr)
					}
					continue
				}

				mu.Lock()
				couponsGenerated++
				mu.Unlock()

				err = store.Queries.MarkCouponReservationAsProcessed(ctx, reservation.ID)
				if err != nil {
					// Critical: coupon created but reservation not marked - potential duplicate risk
					log.Printf("CRITICAL: coupon created but MarkAsProcessed failed for reservation %d: %v", reservation.ID, err)
				}
			}
		}(i, end)
	}

	wg.Wait()

	return couponsGenerated
}
