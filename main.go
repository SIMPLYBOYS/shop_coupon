package main

import (
	"database/sql"
	"log"

	"github.com/SIMPLYBOYS/shopcoupon/api"
	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	_ "github.com/lib/pq" // Postgres driver
	"github.com/willf/bloom"
)

var dbPool *sql.DB

const (
	dbDriver              = "postgres"                                                                     // Database driver
	dbSource              = "postgresql://root:mysecretpassword@localhost:5433/shopcoupon?sslmode=disable" // Database source
	serverAddress         = "0.0.0.0:8080"                                                                 // Server address
	maxConcurrentRequests = 1000                                                                           // Maximum concurrent requests
)

// reservationBloomFilter is a Bloom filter used for storing reservations.
var reservationBloomFilter = bloom.NewWithEstimates(1000000, 0.001)

// grabBloomFilter is a Bloom filter used for storing grab requests.
var grabBloomFilter = bloom.NewWithEstimates(1000000, 0.001)

// grabRequestChan is a channel for handling grab requests.
var grabRequestChan = make(chan *struct{ UserId int }, maxConcurrentRequests)

func main() {
	// Open a connection to the database
	dbPool, err := sql.Open(dbDriver, dbSource)
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	// Set the maximum number of open connections
	dbPool.SetMaxOpenConns(150)

	// Set the maximum number of idle connections
	dbPool.SetMaxIdleConns(5)

	// Create a new store (database interface)
	store := db.NewStore(dbPool)

	// Create a new server instance
	server := api.NewServer(store, reservationBloomFilter, grabBloomFilter, grabRequestChan, maxConcurrentRequests)

	// Start the server
	err = server.Start(serverAddress)
	if err != nil {
		log.Fatal("cannot start server:", err)
	}
}
