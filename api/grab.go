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

	winners := make([]int, 0, numWinners)
	remainingWeight := totalWeight
	for len(winners) < numWinners {
		target, err := rand.Int(rand.Reader, big.NewInt(int64(remainingWeight)))
		if err != nil {
			// Handle the error appropriately
			continue
		}

		j := 0
		cumWeight := 0
		for {
			cumWeight += users[j].Weight
			if cumWeight > int(target.Int64()) {
				break
			}
			j++
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
	case <-time.After(3 * time.Second): // Timeout after 3 seconds
		ctx.JSON(http.StatusRequestTimeout, gin.H{"error": "request timeout"})
	default:
		ctx.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
	}

	ctx.JSON(http.StatusOK, reservation)
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
	return now >= startGrabTime && now < endGrabTime
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
func updateCouponsForWinners(store *db.Store, workPool chan struct{}, coupons []db.Coupons, winners []int) {
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

			tx, err := store.DB.Begin() // Start a transaction
			if err != nil {
				log.Println("handleGrabbing error:", err)
				return
			}
			defer tx.Rollback()

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
			_, err = store.Queries.UpdateCoupon(context.Background(), arg) // Update the coupon
			if err != nil {
				log.Println("handleGrabbing error:", err)
				return
			}

			err = tx.Commit() // Commit the transaction
			if err != nil {
				log.Println("handleGrabbing error:", err)
				return
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
func handleGrabbing(store *db.Store, grabRequestChan <-chan *struct {
	UserId int
}, numWorkers int) {

	workPool := makeWorkerPool(numWorkers) // Create a pool of workers
	defer closeWorkerPool(workPool)        // Close the worker pool when the function returns

	var coupons []db.Coupons
	var err error
	ctx := context.Background()

	for {
		now := time.Now().Unix()

		if !isWithinGrabWindow(now) { // Check if it's within the grab time window
			time.Sleep(time.Second) // Avoid unnecessary looping
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
		updateCouponsForWinners(store, workPool, coupons, winners)
		coupons = nil // Clear the coupon list for the next iteration
	}
}
