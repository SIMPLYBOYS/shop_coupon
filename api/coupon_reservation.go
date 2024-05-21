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
func reservationListener(store *db.Store) {
	for {
		now := time.Now().Unix()
		time.Sleep(3 * time.Second)

		if now < startReserveTime || now >= endReserveTime {
			continue
		}

		handleReservations(store)
	}
}

// handleReservations processes the coupon reservations
func handleReservations(store *db.Store) {
	log.Default().Printf("handleReservations ===============>")
	reservations, err := store.Queries.ListCouponReservations(context.Background())
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

		couponsGenerated := generateCouponsForReservations(store, reservations[i:end], numCoupons)
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

	UserID := int(req.UserID) // Convert int32 to int
	userIdStr := strconv.Itoa(UserID)

	if s.bloomFilterForReserve.TestString(userIdStr) { // Check if the user has already reserved
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("User already reserved")))
		return
	}
	couponReservation, err := s.store.CreateCouponReservation(ctx, int32(UserID))

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	s.bloomFilterForReserve.AddString(userIdStr) // Add the user to the reservation Bloom filter
	ctx.JSON(http.StatusOK, couponReservation)
}

// generateCouponsForReservations generates coupons for the given reservations
func generateCouponsForReservations(store *db.Store, reservations []db.CouponReservations, numCoupons int) int {
	couponsGenerated := 0
	tx, err := store.DB.Begin() // Start a transaction
	if err != nil {
		log.Println("generateCouponsForReservations error:", err)
		return 0
	}
	defer tx.Rollback()

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
				if couponsGenerated >= numCoupons {
					err = store.Queries.MarkCouponReservationAsProcessed(context.Background(), reservation.ID)
					if err != nil {
						log.Println("generateCouponsForReservations error:", err)
					}
					continue
				}

				arg := db.CreateCouponParams{
					Code:       u.GenerateCouponCode(),      // Generate a unique coupon code
					Discount:   "0.25",                      // 25% discount
					ExpiryDate: time.Now().AddDate(0, 0, 7), // Expiry date is 7 days from now
				}

				_, err = store.Queries.CreateCoupon(context.Background(), arg)
				if err != nil {
					log.Println("generateCouponsForReservations error:", err)
					continue
				}

				couponsGenerated++
				err = store.Queries.MarkCouponReservationAsProcessed(context.Background(), reservation.ID)
				if err != nil {
					log.Println("generateCouponsForReservations error:", err)
				}
			}
		}(i, end)
	}

	wg.Wait()

	err = tx.Commit() // Commit the transaction
	if err != nil {
		log.Println("generateCouponsForReservations error:", err)
		return couponsGenerated
	}

	return couponsGenerated
}
