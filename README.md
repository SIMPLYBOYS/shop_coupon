# Flash Coupon Delivery Mechanism Design and Implementation

## Requirement Description

1. Coupons are issued daily at 23:00, requiring users to reserve 1-5 minutes in advance. The number of coupons is 20% of the number of reserved users.
2. During the coupon grab, each user can only grab once, and there is only 1 minute for the grab. How to ensure that the user's probability of grabbing the coupon is as close to 20% as possible?

## Architecture Design

### Overall Process

1. Reservation Stage: Users make reservations between 22:55 and 23:00.
2. Distribution Stage: At 23:00, calculate the number of coupons based on the number of reserved users (20% of reserved users) and generate corresponding coupons.
3. Grab Stage: From 23:00 to 23:01, users can grab the coupons, and each user can only grab once.

### Technology Stack

- Programming Language: Go
- Framework: Gin
- Database: PostgreSQL
- SQL Generation Tool: sqlc

## Installation and Configuration

### Install Dependencies

Ensure you have installed the following software:

- Go
- PostgreSQL (in Docker Environment)
- sqlc
- K6 (for load testing)

Then install the Go dependencies:

```sh
go mod download
```

### Set up the database

```sh
make postgres
make createdb
make migrationup
```

### Run the server

```sh
make server
```

### Load Testing with K6

```sh
k6 run script
```
