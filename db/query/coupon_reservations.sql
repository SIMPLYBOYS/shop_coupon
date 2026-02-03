-- coupon_reservations.sql

-- name: GetCouponReservation :one
-- Read-only query - FOR UPDATE removed as there's no subsequent update in the same transaction
SELECT * FROM coupon_reservations WHERE user_id = $1 LIMIT 1;

-- name: GetCouponReservationsByTimerRange :many
-- Uses SKIP LOCKED to allow concurrent processing without blocking other transactions
SELECT * FROM coupon_reservations WHERE reserved_at BETWEEN $1 AND $2 AND is_processed = FALSE FOR UPDATE SKIP LOCKED;

-- name: GetCouponReservationsByUserIdAndTimestamp :many
-- Read-only query for already processed rows - FOR UPDATE removed
SELECT * FROM coupon_reservations WHERE user_id = $1 AND reserved_at BETWEEN $2 AND $3 AND is_processed = TRUE;

-- name: ListCouponReservations :many
-- Uses SKIP LOCKED to allow concurrent processing without blocking reservation creation
SELECT * FROM coupon_reservations WHERE is_processed = FALSE ORDER BY id FOR UPDATE SKIP LOCKED;

-- name: ListCouponReservationsWithLimit :many
-- Uses SKIP LOCKED to allow concurrent processing without blocking reservation creation
SELECT * FROM coupon_reservations WHERE is_processed = FALSE ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED;

-- name: CreateCouponReservation :one
INSERT INTO coupon_reservations (user_id, reserved_at, is_processed) VALUES ($1, NOW(), FALSE) RETURNING *;

-- name: DeleteCouponReservations :exec
DELETE FROM coupon_reservations WHERE user_id = $1 AND reserved_at BETWEEN $2 AND $3;

-- name: DeleteAllCouponReservations :exec
DELETE FROM coupon_reservations;

-- name: MarkCouponReservationAsProcessed :exec
UPDATE coupon_reservations SET is_processed = TRUE WHERE id = $1;

-- NOTE: BatchMarkCouponReservationsAsProcessed is manually implemented in batch_operations.go
-- to provide consistent API with other batch operations
