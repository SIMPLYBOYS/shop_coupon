package api

import (
	"sync"
	"time"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	u "github.com/SIMPLYBOYS/shopcoupon/util"
	"github.com/gin-gonic/gin"
	"github.com/willf/bloom"
)

type Server struct {
	store                 *db.Store
	router                *gin.Engine
	bloomFilterForGrab    *bloom.BloomFilter // Bloom filter for grab requests
	bloomFilterForReserve *bloom.BloomFilter // Bloom filter for reservation requests
	grabRequestChan       chan *struct{ UserId int }
}

const (
	ReserveStartHour = 13
	ReserveStartMin  = 59
	GrabStartHour    = 14
	GrabStartMin     = 11
)

var startReserveTime, endReserveTime, startGrabTime, endGrabTime int64
var reservedUsersLock = &sync.Mutex{} // Lock for reserved users

// resetBloomFilterDaily resets the Bloom filters for grab and reserve requests daily
func resetBloomFilterDaily(bfr *bloom.BloomFilter, bfg *bloom.BloomFilter) {
	for {
		now := time.Now()
		next := now.Add(time.Hour * 24)
		next = time.Date(next.Year(), next.Month(), next.Day(), 23, 10, 0, 0, next.Location())
		t := time.NewTimer(next.Sub(now))
		<-t.C
		bfr = bloom.NewWithEstimates(1000000, 0.001)
		bfg = bloom.NewWithEstimates(1000000, 0.001)
	}
}

// couponClockTimer updates the time windows for reservation and grab requests
func couponClockTimer() {
	for {
		time.Sleep(5 * time.Second)
		startReserveTime = u.GetSpecificTime(ReserveStartHour, ReserveStartMin, 0).Unix() // 22:55 ~ 23:00
		endReserveTime = startReserveTime + 5*60
		startGrabTime = u.GetSpecificTime(GrabStartHour, GrabStartMin, 0).Unix() // 23:00 ~ 23:01
		endGrabTime = startGrabTime + 60
	}
}

// NewServer creates a new server instance
func NewServer(store *db.Store, bfr *bloom.BloomFilter, bfg *bloom.BloomFilter, grabRC chan *struct{ UserId int }, numWorkers int) *Server {
	go couponClockTimer()
	go reservationListener(store)
	go resetBloomFilterDaily(bfr, bfg)
	go handleGrabbing(store, grabRC, numWorkers)

	server := &Server{store: store, bloomFilterForReserve: bfr, bloomFilterForGrab: bfg, grabRequestChan: grabRC}
	router := gin.Default()
	router.GET("/user/:id", server.getUser)
	router.GET("/coupon/:code", server.getCoupon)
	router.GET("/reservation/:user_id", server.getCouponReservation)
	router.POST("/reserve", specialTime, server.createCouponReservation)
	router.POST("/grab", specialTime, server.handleGrabRequest)
	router.POST("/user", server.createUser)
	server.router = router

	return server
}

// errorResponse creates a gin.H map for error responses
func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

// Start starts the server and listens on the specified address
func (s *Server) Start(address string) error {
	return s.router.Run(address)
}
