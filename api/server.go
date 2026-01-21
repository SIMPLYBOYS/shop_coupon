package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	u "github.com/SIMPLYBOYS/shopcoupon/util"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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
	grabBloomMu           sync.RWMutex       // RWMutex to protect bloomFilterForGrab operations
	reserveBloomMu        sync.RWMutex       // RWMutex to protect bloomFilterForReserve operations
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
func (s *Server) resetBloomFilterDaily(ctx context.Context) {
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
			// Protect ClearAll with write lock to ensure thread safety
			s.reserveBloomMu.Lock()
			s.bloomFilterForReserve.ClearAll()
			s.reserveBloomMu.Unlock()

			s.grabBloomMu.Lock()
			s.bloomFilterForGrab.ClearAll()
			s.grabBloomMu.Unlock()
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

	// Time-restricted + Authenticated routes (check time window first for early rejection)
	authenticatedTimeRestricted := router.Group("/", specialTime, authMiddleware())
	{
		authenticatedTimeRestricted.POST("/reserve", server.createCouponReservation)
		authenticatedTimeRestricted.POST("/grab", server.handleGrabRequest)
	}

	server.router = router

	return server
}

// publicError is an error type that is safe to expose to clients
type publicError struct {
	message string
}

func (e *publicError) Error() string {
	return e.message
}

// newPublicError creates a new public error that is safe to expose to clients
func newPublicError(message string) error {
	return &publicError{message: message}
}

// isPublicError checks if an error is safe to expose to clients
func isPublicError(err error) bool {
	var pe *publicError
	return errors.As(err, &pe)
}

// errorResponse creates a gin.H map for error responses.
// It logs only unexpected internal errors and returns sanitized messages to clients.
func errorResponse(err error) gin.H {
	// Check for known safe sentinel errors (no logging needed)
	switch {
	case errors.Is(err, ErrUnauthorized):
		return gin.H{"error": "unauthorized"}
	case errors.Is(err, ErrAccessDenied):
		return gin.H{"error": "access denied"}
	}

	// Check if it's a public error (no logging needed - expected business errors)
	if isPublicError(err) {
		return gin.H{"error": err.Error()}
	}

	// Handle validation errors from gin binding (no logging needed - user input errors)
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		return gin.H{"error": err.Error()}
	}

	// Handle database "not found" errors (no logging needed - expected scenario)
	if errors.Is(err, sql.ErrNoRows) {
		return gin.H{"error": "resource not found"}
	}

	// Log only unexpected internal errors for debugging
	log.Printf("API internal error: %v", err)

	// For all other errors, return a generic message
	return gin.H{"error": "an internal error occurred"}
}

// Start starts background goroutines and the HTTP server.
// The context is used for graceful shutdown of background tasks.
func (s *Server) Start(ctx context.Context, address string) error {
	go couponClockTimer(ctx)
	go reservationListener(ctx, s.store)
	go s.resetBloomFilterDaily(ctx)
	go handleGrabbing(ctx, s.store, s.grabRequestChan, s.numWorkers)

	return s.router.Run(address)
}
