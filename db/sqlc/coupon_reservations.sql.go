// Code generated by sqlc. DO NOT EDIT.
// source: coupon_reservations.sql

package db

import (
	"context"
	"time"
)

const createCouponReservation = `-- name: CreateCouponReservation :one
INSERT INTO coupon_reservations (user_id, reserved_at, is_processed) VALUES ($1, NOW(), FALSE) RETURNING id, user_id, reserved_at, is_processed
`

func (q *Queries) CreateCouponReservation(ctx context.Context, userID int32) (CouponReservations, error) {
	row := q.db.QueryRowContext(ctx, createCouponReservation, userID)
	var i CouponReservations
	err := row.Scan(
		&i.ID,
		&i.UserID,
		&i.ReservedAt,
		&i.IsProcessed,
	)
	return i, err
}

const deleteAllCouponReservations = `-- name: DeleteAllCouponReservations :exec
DELETE FROM coupon_reservations
`

func (q *Queries) DeleteAllCouponReservations(ctx context.Context) error {
	_, err := q.db.ExecContext(ctx, deleteAllCouponReservations)
	return err
}

const deleteCouponReservations = `-- name: DeleteCouponReservations :exec
DELETE FROM coupon_reservations WHERE user_id = $1 AND reserved_at BETWEEN $2 AND $3
`

type DeleteCouponReservationsParams struct {
	UserID       int32     `json:"user_id"`
	ReservedAt   time.Time `json:"reserved_at"`
	ReservedAt_2 time.Time `json:"reserved_at_2"`
}

func (q *Queries) DeleteCouponReservations(ctx context.Context, arg DeleteCouponReservationsParams) error {
	_, err := q.db.ExecContext(ctx, deleteCouponReservations, arg.UserID, arg.ReservedAt, arg.ReservedAt_2)
	return err
}

const getCouponReservation = `-- name: GetCouponReservation :one

SELECT id, user_id, reserved_at, is_processed FROM coupon_reservations WHERE user_id = $1 LIMIT 1 FOR UPDATE
`

// coupon_reservations.sql
func (q *Queries) GetCouponReservation(ctx context.Context, userID int32) (CouponReservations, error) {
	row := q.db.QueryRowContext(ctx, getCouponReservation, userID)
	var i CouponReservations
	err := row.Scan(
		&i.ID,
		&i.UserID,
		&i.ReservedAt,
		&i.IsProcessed,
	)
	return i, err
}

const getCouponReservationsByTimerRange = `-- name: GetCouponReservationsByTimerRange :many
SELECT id, user_id, reserved_at, is_processed FROM coupon_reservations WHERE reserved_at BETWEEN $1 AND $2 AND is_processed = FALSE FOR UPDATE
`

type GetCouponReservationsByTimerRangeParams struct {
	ReservedAt   time.Time `json:"reserved_at"`
	ReservedAt_2 time.Time `json:"reserved_at_2"`
}

func (q *Queries) GetCouponReservationsByTimerRange(ctx context.Context, arg GetCouponReservationsByTimerRangeParams) ([]CouponReservations, error) {
	rows, err := q.db.QueryContext(ctx, getCouponReservationsByTimerRange, arg.ReservedAt, arg.ReservedAt_2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CouponReservations
	for rows.Next() {
		var i CouponReservations
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.ReservedAt,
			&i.IsProcessed,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getCouponReservationsByUserIdAndTimestamp = `-- name: GetCouponReservationsByUserIdAndTimestamp :many
SELECT id, user_id, reserved_at, is_processed FROM coupon_reservations WHERE user_id = $1 AND reserved_at BETWEEN $2 AND $3 AND is_processed = TRUE FOR UPDATE
`

type GetCouponReservationsByUserIdAndTimestampParams struct {
	UserID       int32     `json:"user_id"`
	ReservedAt   time.Time `json:"reserved_at"`
	ReservedAt_2 time.Time `json:"reserved_at_2"`
}

func (q *Queries) GetCouponReservationsByUserIdAndTimestamp(ctx context.Context, arg GetCouponReservationsByUserIdAndTimestampParams) ([]CouponReservations, error) {
	rows, err := q.db.QueryContext(ctx, getCouponReservationsByUserIdAndTimestamp, arg.UserID, arg.ReservedAt, arg.ReservedAt_2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CouponReservations
	for rows.Next() {
		var i CouponReservations
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.ReservedAt,
			&i.IsProcessed,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listCouponReservations = `-- name: ListCouponReservations :many
SELECT id, user_id, reserved_at, is_processed FROM coupon_reservations WHERE is_processed = FALSE ORDER BY id FOR UPDATE
`

func (q *Queries) ListCouponReservations(ctx context.Context) ([]CouponReservations, error) {
	rows, err := q.db.QueryContext(ctx, listCouponReservations)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CouponReservations
	for rows.Next() {
		var i CouponReservations
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.ReservedAt,
			&i.IsProcessed,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const markCouponReservationAsProcessed = `-- name: MarkCouponReservationAsProcessed :exec
UPDATE coupon_reservations SET is_processed = TRUE WHERE id = $1
`

func (q *Queries) MarkCouponReservationAsProcessed(ctx context.Context, id int32) error {
	_, err := q.db.ExecContext(ctx, markCouponReservationAsProcessed, id)
	return err
}