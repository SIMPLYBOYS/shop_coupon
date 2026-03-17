package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
)

type getGrabRequest struct {
	UserID int32 `json:"user_id" binding:"required,min=1"` // The user ID for the grab request
}

func selectWinnersSimple(reservedUsers map[int]int, numWinners int) []int {
	winners := make([]int, 0, numWinners)
	for userID := range reservedUsers {
		winners = append(winners, userID)
		if len(winners) >= numWinners {
			break
		}
	}
	return winners
}

// handleGrabRequest handles the grab request from a user
func (s *Server) handleGrabRequest(ctx *gin.Context) {
	var req getGrabRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Authorization check: ensure user can only grab for themselves
	if err := checkUserAuthorization(ctx, req.UserID); err != nil {
		return
	}

	userIDStr := strconv.FormatInt(int64(req.UserID), 10)

	// Quick rejection check using Bloom filter (read lock for better concurrency)
	s.grabBloomMu.RLock()
	alreadyGrabbed := s.bloomFilterForGrab.TestString(userIDStr)
	s.grabBloomMu.RUnlock()

	if alreadyGrabbed {
		ctx.JSON(http.StatusBadRequest, errorResponse(newPublicError("user already grabbed")))
		return
	}

	reservation, err := s.store.Queries.GetCouponReservation(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "no reservation found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	grabRequest := &GrabRequest{
		UserID: int(reservation.UserID),
	}

	select {
	case s.grabRequestChan <- grabRequest: // Send the grab request to the channel
		// Add to Bloom filter only after successful grab request (write lock)
		// This ensures user can retry if earlier operations fail
		s.grabBloomMu.Lock()
		s.bloomFilterForGrab.AddString(userIDStr)
		s.grabBloomMu.Unlock()
		ctx.JSON(http.StatusOK, gin.H{"message": "grab request accepted"})
		return
	case <-time.After(3 * time.Second): // Timeout after 3 seconds
		ctx.JSON(http.StatusRequestTimeout, gin.H{"error": "request timeout"})
		return
	default:
		ctx.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
		return
	}
}

// makeWorkerPool creates a pool of workers
func makeWorkerPool(numWorkers int) chan struct{} {
	workPool := make(chan struct{}, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workPool <- struct{}{}
	}
	return workPool
}

// closeWorkerPool closes the worker pool
func closeWorkerPool(workPool chan struct{}) {
	close(workPool)
}

// isWithinGrabWindow checks if the current time is within the grab window.
// timeConfig is passed explicitly to avoid global state (per constitution rule 3.2).
func isWithinGrabWindow(now int64, tc *timeConfig) bool {
	return now >= tc.startGrabTime.Load() && now < tc.endGrabTime.Load()
}

// receiveGrabRequests receives grab requests from the channel.
// Uses context.WithTimeout for a bounded total timeout to avoid memory leaks from time.After.
// Blocks until either: all numCoupons requests are received, or timeout (3 seconds) is reached.
// This ensures we give pending requests time to arrive while preventing indefinite blocking.
func receiveGrabRequests(grabRequestChan <-chan *GrabRequest, numCoupons int) map[int]int {
	reservedUsers := make(map[int]int)

	// Total timeout of 3 seconds - prevents indefinite blocking while giving requests time to arrive
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for i := 0; i < numCoupons; i++ {
		select {
		case grabRequest := <-grabRequestChan:
			reservedUsers[grabRequest.UserID]++
		case <-ctx.Done():
			log.Default().Printf("timeout waiting for grab requests, received %d/%d requests from %d unique users",
				i, numCoupons, len(reservedUsers))
			return reservedUsers
		}
	}
	return reservedUsers
}

// updateCouponsForWinners updates the coupons for winners
// Uses batch UPDATE to minimize database round trips instead of individual updates
func updateCouponsForWinners(ctx context.Context, store *db.Store, workPool chan struct{}, coupons []db.Coupons, winners []int) {
	if len(winners) == 0 || len(coupons) == 0 {
		return
	}

	// Prepare batch assignment parameters
	numToAssign := len(winners)
	if numToAssign > len(coupons) {
		numToAssign = len(coupons)
	}

	couponIDs := make([]int32, numToAssign)
	userIDs := make([]int32, numToAssign)

	for i := 0; i < numToAssign; i++ {
		couponIDs[i] = coupons[i].ID
		userIDs[i] = int32(winners[i])
	}

	// Use batch operation for assigning coupons to users
	batchParams := db.BatchAssignCouponsToUsersParams{
		CouponIDs: couponIDs,
		UserIDs:   userIDs,
	}

	rowsAffected, err := store.Queries.BatchAssignCouponsToUsers(ctx, batchParams)
	if err != nil {
		log.Printf("updateCouponsForWinners BatchAssignCouponsToUsers error: %v", err)
		// Fall back to individual updates if batch fails
		fallbackUpdateCouponsForWinners(ctx, store, workPool, coupons, winners)
		return
	}

	log.Printf("updateCouponsForWinners: batch assigned %d coupons to users", rowsAffected)
}

// fallbackUpdateCouponsForWinners provides a fallback mechanism using individual updates
// This is used when the batch operation fails. Uses limited concurrency to avoid
// overwhelming the database if there are connection issues.
func fallbackUpdateCouponsForWinners(ctx context.Context, store *db.Store, workPool chan struct{}, coupons []db.Coupons, winners []int) {
	var wg sync.WaitGroup
	var successCount int32
	var failCount int32
	var mu sync.Mutex

	numToProcess := len(winners)
	if numToProcess > len(coupons) {
		numToProcess = len(coupons)
	}

	for i := 0; i < numToProcess; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Safely get a worker from the pool with context cancellation support
			select {
			case <-workPool:
				// Got a worker, continue
			case <-ctx.Done():
				mu.Lock()
				failCount++
				mu.Unlock()
				log.Printf("fallbackUpdateCouponsForWinners: context cancelled for user %d", winners[index])
				return
			}

			// Safely return the worker to the pool
			defer func() {
				select {
				case workPool <- struct{}{}:
					// Worker returned successfully
				default:
					// Pool is full or closed, ignore
				}
			}()

			userId := winners[index]
			discount := coupons[index].Discount
			if discount == "" {
				discount = "0.25"
			}
			arg := db.UpdateCouponParams{
				ID:         coupons[index].ID,
				Code:       coupons[index].Code,
				ExpiryDate: coupons[index].ExpiryDate,
				Discount:   discount,
				IsUsed:     true,
				UserID:     sql.NullInt32{Int32: int32(userId), Valid: true},
			}
			_, err := store.Queries.UpdateCoupon(ctx, arg)
			if err != nil {
				mu.Lock()
				failCount++
				mu.Unlock()
				if isUniqueViolationError(err) {
					log.Printf("fallbackUpdateCouponsForWinners: user %d already has a coupon (unique constraint)", userId)
				} else {
					log.Printf("fallbackUpdateCouponsForWinners error for user %d: %v", userId, err)
				}
				return
			}

			mu.Lock()
			successCount++
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	log.Printf("fallbackUpdateCouponsForWinners: completed %d/%d updates (%d failed)", successCount, numToProcess, failCount)
}

// collectUnwinners collects the user IDs of the users who did not win
func collectUnwinners(reservedUsers map[int]int, winners []int) []int {
	var unwinners []int
	for userId := range reservedUsers {
		found := false
		for _, winner := range winners {
			if userId == winner {
				found = true
				break
			}
		}
		if !found {
			unwinners = append(unwinners, userId)
		}
	}
	return unwinners
}

// handleGrabbing handles the grabbing process for the coupons.
// timeConfig is passed explicitly to avoid global state (per constitution rule 3.2).
func handleGrabbing(ctx context.Context, store *db.Store, grabRequestChan <-chan *GrabRequest, numWorkers int, tc *timeConfig) {

	workPool := makeWorkerPool(numWorkers) // Create a pool of workers
	defer closeWorkerPool(workPool)        // Close the worker pool when the function returns

	var coupons []db.Coupons
	var err error
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()

			if !isWithinGrabWindow(now, tc) { // Check if it's within the grab time window
				continue
			}

			if len(coupons) == 0 { // If there are no coupons, fetch available coupons
				coupons, err = store.Queries.ListAvailableCoupons(ctx, time.Now())
				if err != nil {
					log.Printf("handleGrabbing: listing available coupons: %v", err)
					continue
				}
			}

			numCoupons := len(coupons)
			log.Default().Printf("in handleGrabbing numCoupons: %d", numCoupons)

			if numCoupons == 0 {
				coupons = nil // Clear the coupon list for the next iteration
				continue
			}

			reservedUsers := receiveGrabRequests(grabRequestChan, numCoupons) // Receive grab requests
			winners := selectWinnersSimple(reservedUsers, numCoupons)
			updateCouponsForWinners(ctx, store, workPool, coupons, winners)
			coupons = nil // Clear the coupon list for the next iteration
		}
	}
}
