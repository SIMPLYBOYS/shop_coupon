package api

import (
	"context"
	"sync/atomic"
	"time"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	u "github.com/SIMPLYBOYS/shopcoupon/util"
	"github.com/gin-gonic/gin"
	"github.com/willf/bloom"
)

// timeConfig holds time window configuration with atomic access for concurrency safety
type timeConfig struct {
	startReserveTime atomic.Int64
	endReserveTime   atomic.Int64
	startGrabTime    atomic.Int64
	endGrabTime      atomic.Int64
}

// couponTimeConfig is the package-level time configuration with atomic access
var couponTimeConfig = &timeConfig{}

type Server struct {
	store                 *db.Store
	router                *gin.Engine
	bloomFilterForGrab    *bloom.BloomFilter // Bloom filter for grab requests
	bloomFilterForReserve *bloom.BloomFilter // Bloom filter for reservation requests
	grabRequestChan       chan *struct{ UserId int }
	numWorkers            int
}

const (
	ReserveStartHour = 13
	ReserveStartMin  = 59
	GrabStartHour    = 14
	GrabStartMin     = 11
)

// resetBloomFilterDaily resets the Bloom filters for grab and reserve requests daily
func resetBloomFilterDaily(ctx context.Context, bfr *bloom.BloomFilter, bfg *bloom.BloomFilter) {
	for {
		now := time.Now()
		next := now.Add(time.Hour * 24)
		next = time.Date(next.Year(), next.Month(), next.Day(), 23, 10, 0, 0, next.Location())
		t := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			bfr.ClearAll() // Clear the existing filter in place
			bfg.ClearAll() // Clear the existing filter in place
		}
	}
}

// couponClockTimer updates the time windows for reservation and grab requests
func couponClockTimer(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			startReserve := u.GetSpecificTime(ReserveStartHour, ReserveStartMin, 0).Unix()
			couponTimeConfig.startReserveTime.Store(startReserve)
			couponTimeConfig.endReserveTime.Store(startReserve + 5*60)

			startGrab := u.GetSpecificTime(GrabStartHour, GrabStartMin, 0).Unix()
			couponTimeConfig.startGrabTime.Store(startGrab)
			couponTimeConfig.endGrabTime.Store(startGrab + 60)
		}
	}
}

// NewServer creates a new server instance
func NewServer(store *db.Store, bfr *bloom.BloomFilter, bfg *bloom.BloomFilter, grabRC chan *struct{ UserId int }, numWorkers int) *Server {
	server := &Server{
		store:                 store,
		bloomFilterForReserve: bfr,
		bloomFilterForGrab:    bfg,
		grabRequestChan:       grabRC,
		numWorkers:            numWorkers,
	}

	router := gin.Default()

	// Public routes (no authentication required)
	router.GET("/coupon/:code", server.getCoupon)
	router.POST("/user", server.createUser)

	// Authenticated routes (require valid X-User-ID header)
	authenticated := router.Group("/", authMiddleware())
	{
		authenticated.GET("/user/:id", server.getUser)
		authenticated.GET("/reservation/:user_id", server.getCouponReservation)
	}

	// Time-restricted routes (only available during special time windows)
	router.POST("/reserve", specialTime, server.createCouponReservation)
	router.POST("/grab", specialTime, server.handleGrabRequest)

	server.router = router

	return server
}

// errorResponse creates a gin.H map for error responses
func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

// Start starts background goroutines and the HTTP server.
// The context is used for graceful shutdown of background tasks.
func (s *Server) Start(ctx context.Context, address string) error {
	go couponClockTimer(ctx)
	go reservationListener(ctx, s.store)
	go resetBloomFilterDaily(ctx, s.bloomFilterForReserve, s.bloomFilterForGrab)
	go handleGrabbing(ctx, s.store, s.grabRequestChan, s.numWorkers)

	return s.router.Run(address)
}
