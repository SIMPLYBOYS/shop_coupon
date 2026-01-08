package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/SIMPLYBOYS/shopcoupon/api"
	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	_ "github.com/lib/pq" // Postgres driver
	"github.com/willf/bloom"
)

var dbPool *sql.DB

const (
	dbDriver              = "postgres"    // Database driver
	serverAddress         = "0.0.0.0:8080" // Server address
	maxConcurrentRequests = 1000           // Maximum concurrent requests
)

// getDBSource returns the database connection string from environment variable
func getDBSource() string {
	dbSource := os.Getenv("DATABASE_URL")
	if dbSource == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	return dbSource
}

// reservationBloomFilter is a Bloom filter used for storing reservations.
var reservationBloomFilter = bloom.NewWithEstimates(1000000, 0.001)

// grabBloomFilter is a Bloom filter used for storing grab requests.
var grabBloomFilter = bloom.NewWithEstimates(1000000, 0.001)

// grabRequestChan is a channel for handling grab requests.
var grabRequestChan = make(chan *struct{ UserId int }, maxConcurrentRequests)

func main() {

	// Open a connection to the database
	dbPool, err := sql.Open(dbDriver, getDBSource())

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
		log.Printf("Error starting server: %v", err)
		return
	}
}
