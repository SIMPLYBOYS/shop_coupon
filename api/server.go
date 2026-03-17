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
	"github.com/lib/pq"
	"github.com/willf/bloom"
)

// timeConfig holds time window configuration with atomic access for concurrency safety
type timeConfig struct {
	startReserveTime atomic.Int64
	endReserveTime   atomic.Int64
	startGrabTime    atomic.Int64
	endGrabTime      atomic.Int64
}

// GrabRequest represents a user's grab request.
type GrabRequest struct {
	UserID int
}

type Server struct {
	store                 *db.Store
	router                *gin.Engine
	bloomFilterForGrab    *bloom.BloomFilter // Bloom filter for grab requests
	bloomFilterForReserve *bloom.BloomFilter // Bloom filter for reservation requests
	grabBloomMu           sync.RWMutex       // RWMutex to protect bloomFilterForGrab operations
	reserveBloomMu        sync.RWMutex       // RWMutex to protect bloomFilterForReserve operations
	grabRequestChan       chan *GrabRequest
	numWorkers            int
	rateLimiter           *rateLimiter // Rate limiter for API requests
	timeConfig            *timeConfig  // Time window configuration (injected dependency)
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
func (s *Server) couponClockTimer(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			startReserve := u.GetSpecificTime(ReserveStartHour, ReserveStartMin, 0).Unix()
			s.timeConfig.startReserveTime.Store(startReserve)
			s.timeConfig.endReserveTime.Store(startReserve + 5*60)

			startGrab := u.GetSpecificTime(GrabStartHour, GrabStartMin, 0).Unix()
			s.timeConfig.startGrabTime.Store(startGrab)
			s.timeConfig.endGrabTime.Store(startGrab + 60)
		}
	}
}

// NewServer creates a new server instance
func NewServer(store *db.Store, bfr *bloom.BloomFilter, bfg *bloom.BloomFilter, grabRC chan *GrabRequest, numWorkers int) *Server {
	server := &Server{
		store:                 store,
		bloomFilterForReserve: bfr,
		bloomFilterForGrab:    bfg,
		grabRequestChan:       grabRC,
		numWorkers:            numWorkers,
		rateLimiter:           newRateLimiter(60, time.Minute), // 60 requests per minute per IP
		timeConfig:            &timeConfig{},                   // Initialize time config (injected dependency)
	}

	router := gin.Default()

	// Apply rate limiting globally to prevent DoS and brute force attacks
	router.Use(server.rateLimitMiddleware())

	// Public routes (no authentication required)
	router.POST("/user", server.createUser)

	// Authenticated routes (require valid X-User-ID header)
	authenticated := router.Group("/", authMiddleware())
	{
		authenticated.GET("/user/:id", server.getUser)
		authenticated.GET("/reservation/:user_id", server.getCouponReservation)
		authenticated.GET("/coupon/:code", server.getCoupon)
	}

	// Time-restricted + Authenticated routes (check time window first for early rejection)
	authenticatedTimeRestricted := router.Group("/", server.specialTimeMiddleware(), authMiddleware())
	{
		authenticatedTimeRestricted.POST("/reserve", server.createCouponReservation)
		authenticatedTimeRestricted.POST("/grab", server.handleGrabRequest)
	}

	server.router = router

	return server
}

// PostgreSQL error codes
const (
	pqUniqueViolation = "23505" // unique_violation error code
)

// isUniqueViolationError checks if the error is a PostgreSQL unique constraint violation.
// This is used to detect when a duplicate entry is attempted (e.g., user already reserved).
func isUniqueViolationError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == pqUniqueViolation
	}
	return false
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
	case errors.Is(err, errUnauthorized):
		return gin.H{"error": "unauthorized"}
	case errors.Is(err, errAccessDenied):
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
	go s.couponClockTimer(ctx)
	go s.reservationListener(ctx)
	go s.resetBloomFilterDaily(ctx)
	go handleGrabbing(ctx, s.store, s.grabRequestChan, s.numWorkers, s.timeConfig)
	s.rateLimiter.startCleanup(ctx)

	return s.router.Run(address)
}
