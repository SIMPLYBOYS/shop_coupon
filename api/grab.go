package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
)

type getGrabRequest struct {
	UserID int `json:"user_id" binding:"required,min=1"` // The user ID for the grab request
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

// selectWinners selects the winners from the reserved users based on their weights
func selectWinners(reservedUsers map[int]int, numWinners int) []int {
	users := make([]User, 0, len(reservedUsers))
	totalWeight := 0
	for userID, weight := range reservedUsers {
		totalWeight += weight
		users = append(users, User{ID: userID, Weight: weight})
	}

	// Ensure we don't try to select more winners than available users
	if numWinners > len(users) {
		numWinners = len(users)
	}

	winners := make([]int, 0, numWinners)
	remainingWeight := totalWeight
	const maxRetries = 3

	for len(winners) < numWinners && len(users) > 0 {
		// Ensure remainingWeight is positive before calling rand.Int
		if remainingWeight <= 0 {
			log.Printf("selectWinners: remainingWeight is %d, stopping selection", remainingWeight)
			break
		}

		var target *big.Int
		var err error
		for retry := 0; retry < maxRetries; retry++ {
			target, err = rand.Int(rand.Reader, big.NewInt(int64(remainingWeight)))
			if err == nil {
				break
			}
			log.Printf("selectWinners rand.Int error (retry %d/%d): %v", retry+1, maxRetries, err)
		}
		if err != nil {
			log.Printf("selectWinners: rand.Int failed after %d retries, selecting first available user", maxRetries)
			// Fallback: select the first available user
			target = big.NewInt(0)
		}

		j := 0
		cumWeight := 0
		for j < len(users) {
			cumWeight += users[j].Weight
			if cumWeight > int(target.Int64()) {
				break
			}
			j++
		}

		// Safety check: ensure j is within bounds
		if j >= len(users) {
			j = len(users) - 1
		}

		remainingWeight -= users[j].Weight
		winners = append(winners, users[j].ID)
		users[j], users[len(users)-1] = users[len(users)-1], users[j]
		users = users[:len(users)-1]
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

	userIDStr := strconv.Itoa(req.UserID)
	if s.bloomFilterForGrab.TestString(userIDStr) { // Check if the user has already grabbed
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("User already grabbed")))
		return
	}

	reservation, err := s.store.Queries.GetCouponReservation(ctx, int32(req.UserID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	s.bloomFilterForGrab.AddString(userIDStr) // Add the user to the grab Bloom filter

	var zeroValue db.CouponReservations
	if reservation == zeroValue { // Check if the reservation exists
		ctx.JSON(http.StatusNotFound, gin.H{"error": "no reservation found"})
		return
	}

	grabRequest := &struct {
		UserId int
	}{
		UserId: int(reservation.UserID),
	}

	select {
	case s.grabRequestChan <- grabRequest: // Send the grab request to the channel
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

type User struct {
	ID     int
	Weight int
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

// isWithinGrabWindow checks if the current time is within the grab window
func isWithinGrabWindow(now int64) bool {
	return now >= couponTimeConfig.startGrabTime.Load() && now < couponTimeConfig.endGrabTime.Load()
}

// receiveGrabRequests receives grab requests from the channel
func receiveGrabRequests(grabRequestChan <-chan *struct {
	UserId int
}, numCoupons int) map[int]int {
	reservedUsers := make(map[int]int)
	for i := 0; i < numCoupons; i++ {
		select {
		case grabRequest := <-grabRequestChan:
			reservedUsers[grabRequest.UserId]++
		case <-time.After(3 * time.Second):
			log.Default().Printf("timeout")
		default:
			break
		}
	}
	return reservedUsers
}

// updateCouponsForWinners updates the coupons for winners
// Note: Each coupon update is independent, so we don't use a transaction here.
func updateCouponsForWinners(ctx context.Context, store *db.Store, workPool chan struct{}, coupons []db.Coupons, winners []int) {
	var winnersMutex sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < len(winners); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-workPool // Get a worker from the pool

			winnersMutex.Lock()
			userId := 0
			if index < len(winners) {
				userId = winners[index]
			}
			winnersMutex.Unlock()

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
			_, err := store.Queries.UpdateCoupon(ctx, arg) // Update the coupon
			if err != nil {
				log.Println("handleGrabbing error:", err)
			}

			workPool <- struct{}{} // Return the worker to the pool
		}(i)
	}

	wg.Wait()
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

// handleGrabbing handles the grabbing process for the coupons
func handleGrabbing(ctx context.Context, store *db.Store, grabRequestChan <-chan *struct {
	UserId int
}, numWorkers int) {

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

			if !isWithinGrabWindow(now) { // Check if it's within the grab time window
				continue
			}

			if len(coupons) == 0 { // If there are no coupons, fetch available coupons
				coupons, err = store.Queries.ListAvailableCoupons(ctx, time.Now())
				if err != nil {
					log.Println("handleGrabbing error:", err)
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
