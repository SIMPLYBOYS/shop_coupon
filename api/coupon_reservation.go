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

// Reservation processing constants. Changes affect coupon distribution volume and DB load.
const (
	maxReservationsPerCycle = 10000 // Maximum reservations to process per cycle
	couponWinnerRatio      = 0.2   // 20% of reservations will receive coupons
	reservationBatchSize   = 3000  // Batch size for processing reservations
)

// handleReservations processes the coupon reservations.
// Uses a limit to prevent DoS attacks from overwhelming the system with unlimited reservations.
func handleReservations(ctx context.Context, store *db.Store) {
	log.Default().Printf("handleReservations ===============>")
	reservations, err := store.Queries.ListCouponReservationsWithLimit(ctx, maxReservationsPerCycle)
	if err != nil {
		log.Printf("handleReservations: listing reservations: %v", err)
		return
	}

	numReservations := len(reservations)
	numCoupons := int(float64(numReservations) * couponWinnerRatio)

	// Process reservations in batches
	for i := 0; i < numReservations; i += reservationBatchSize {
		end := i + reservationBatchSize
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
		if errors.Is(err, errUnauthorized) {
			ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		} else {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		}
		return
	}

	if userID != req.UserID {
		ctx.JSON(http.StatusForbidden, errorResponse(errAccessDenied))
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

// partitionReservations splits reservations into winners (who get coupons) and non-winners.
// numCoupons is clamped to [0, len(reservations)], so winners may be fewer than requested.
func partitionReservations(reservations []db.CouponReservations, numCoupons int) (winners, nonWinners []db.CouponReservations) {
	if numCoupons < 0 {
		numCoupons = 0
	}
	if numCoupons > len(reservations) {
		numCoupons = len(reservations)
	}

	// Use three-index slice to limit capacity, preventing append on winners
	// from silently overwriting nonWinners data.
	winners = reservations[:numCoupons:numCoupons]
	nonWinners = reservations[numCoupons:]
	return winners, nonWinners
}

// generateCodesForWinners generates coupon codes for each winner reservation.
// Returns the codes, corresponding reservation IDs, and IDs that failed code generation.
// Failed reservations are left unprocessed so they can be retried on the next cycle.
func generateCodesForWinners(winners []db.CouponReservations) (codes []string, reservationIDs []int32, failedIDs []int32) {
	codes = make([]string, 0, len(winners))
	reservationIDs = make([]int32, 0, len(winners))

	for _, w := range winners {
		code, err := u.GenerateCouponCode()
		if err != nil {
			log.Printf("generateCodesForWinners: GenerateCouponCode error for reservation %d: %v", w.ID, err)
			failedIDs = append(failedIDs, w.ID)
			continue
		}
		codes = append(codes, code)
		reservationIDs = append(reservationIDs, w.ID)
	}
	return codes, reservationIDs, failedIDs
}

// createCouponsForWinners creates coupons and marks winner reservations as processed atomically.
// Falls back to individual creation if the batch operation fails.
// Panics if codes and reservationIDs have different lengths (indicates a programming error).
func createCouponsForWinners(ctx context.Context, store *db.Store, codes []string, reservationIDs []int32) int {
	if len(codes) != len(reservationIDs) {
		log.Panicf("createCouponsForWinners: codes length (%d) != reservationIDs length (%d)", len(codes), len(reservationIDs))
	}
	if len(codes) == 0 {
		return 0
	}

	batchParams := db.BatchCreateCouponsParams{
		Codes:      codes,
		Discount:   "0.25",                      // 25% discount
		ExpiryDate: time.Now().AddDate(0, 0, 7), // Expiry date is 7 days from now
	}

	created, err := store.BatchCreateCouponsWithTx(ctx, batchParams, reservationIDs)
	if err != nil {
		log.Printf("createCouponsForWinners: BatchCreateCouponsWithTx error: %v", err)
		return fallbackCreateCouponsIndividually(ctx, store, codes, reservationIDs, batchParams)
	}

	log.Printf("createCouponsForWinners: created %d coupons and marked %d reservations in transaction", created, len(reservationIDs))
	return int(created)
}

// markNonWinnersProcessed marks non-winner reservations as processed.
// Falls back to individual marking if the batch operation fails.
// Returns the number of reservations successfully marked.
func markNonWinnersProcessed(ctx context.Context, store *db.Store, nonWinners []db.CouponReservations) int {
	if len(nonWinners) == 0 {
		return 0
	}

	ids := make([]int32, len(nonWinners))
	for i, nw := range nonWinners {
		ids[i] = nw.ID
	}

	err := store.Queries.BatchMarkCouponReservationsAsProcessed(ctx, ids)
	if err != nil {
		log.Printf("markNonWinnersProcessed: batch error: %v", err)
		successCount := 0
		for _, id := range ids {
			if markErr := store.Queries.MarkCouponReservationAsProcessed(ctx, id); markErr != nil {
				log.Printf("markNonWinnersProcessed: fallback error for reservation %d: %v", id, markErr)
			} else {
				successCount++
			}
		}
		log.Printf("markNonWinnersProcessed: fallback marked %d/%d non-winner reservations", successCount, len(ids))
		return successCount
	}

	return len(ids)
}

// generateCouponsForReservations orchestrates coupon generation for a batch of reservations.
// It partitions reservations, generates codes, creates coupons for winners,
// and marks non-winners as processed.
func generateCouponsForReservations(ctx context.Context, store *db.Store, reservations []db.CouponReservations, numCoupons int) int {
	if len(reservations) == 0 {
		return 0
	}

	winners, nonWinners := partitionReservations(reservations, numCoupons)
	codes, winnerIDs, failedIDs := generateCodesForWinners(winners)

	if len(failedIDs) > 0 {
		log.Printf("generateCouponsForReservations: %d reservations had code generation failures, will retry next cycle", len(failedIDs))
	}

	couponsGenerated := createCouponsForWinners(ctx, store, codes, winnerIDs)
	markNonWinnersProcessed(ctx, store, nonWinners)

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
