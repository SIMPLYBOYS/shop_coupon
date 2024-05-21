-- coupon_reservations.sql

-- name: GetCouponReservation :one
SELECT * FROM coupon_reservations WHERE user_id = $1 LIMIT 1 FOR UPDATE;

-- name: GetCouponReservationsByTimerRange :many
SELECT * FROM coupon_reservations WHERE reserved_at BETWEEN $1 AND $2 AND is_processed = FALSE FOR UPDATE;

-- name: GetCouponReservationsByUserIdAndTimestamp :many
SELECT * FROM coupon_reservations WHERE user_id = $1 AND reserved_at BETWEEN $2 AND $3 AND is_processed = TRUE FOR UPDATE;

-- name: ListCouponReservations :many
SELECT * FROM coupon_reservations WHERE is_processed = FALSE ORDER BY id FOR UPDATE;

-- name: CreateCouponReservation :one
INSERT INTO coupon_reservations (user_id, reserved_at, is_processed) VALUES ($1, NOW(), FALSE) RETURNING *;

-- name: DeleteCouponReservations :exec
DELETE FROM coupon_reservations WHERE user_id = $1 AND reserved_at BETWEEN $2 AND $3;

-- name: DeleteAllCouponReservations :exec
DELETE FROM coupon_reservations;

-- name: MarkCouponReservationAsProcessed :exec
UPDATE coupon_reservations SET is_processed = TRUE WHERE id = $1;
