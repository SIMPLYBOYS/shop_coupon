// Code generated by sqlc. DO NOT EDIT.
// source: coupons.sql

package db

import (
	"context"
	"database/sql"
	"time"
)

const claimCoupon = `-- name: ClaimCoupon :one
UPDATE coupons
SET user_id = $2, is_used = true
WHERE id = $1 AND user_id IS NULL
RETURNING id, code, discount, expiry_date, is_used, user_id, created_at
`

type ClaimCouponParams struct {
	ID     int32         `json:"id"`
	UserID sql.NullInt32 `json:"user_id"`
}

func (q *Queries) ClaimCoupon(ctx context.Context, arg ClaimCouponParams) (Coupons, error) {
	row := q.db.QueryRowContext(ctx, claimCoupon, arg.ID, arg.UserID)
	var i Coupons
	err := row.Scan(
		&i.ID,
		&i.Code,
		&i.Discount,
		&i.ExpiryDate,
		&i.IsUsed,
		&i.UserID,
		&i.CreatedAt,
	)
	return i, err
}

const createCoupon = `-- name: CreateCoupon :one
INSERT INTO coupons (code, discount, expiry_date)
VALUES ($1, $2, $3)
RETURNING id, code, discount, expiry_date, is_used, user_id, created_at
`

type CreateCouponParams struct {
	Code       string    `json:"code"`
	Discount   string    `json:"discount"`
	ExpiryDate time.Time `json:"expiry_date"`
}

func (q *Queries) CreateCoupon(ctx context.Context, arg CreateCouponParams) (Coupons, error) {
	row := q.db.QueryRowContext(ctx, createCoupon, arg.Code, arg.Discount, arg.ExpiryDate)
	var i Coupons
	err := row.Scan(
		&i.ID,
		&i.Code,
		&i.Discount,
		&i.ExpiryDate,
		&i.IsUsed,
		&i.UserID,
		&i.CreatedAt,
	)
	return i, err
}

const deleteCoupon = `-- name: DeleteCoupon :exec
DELETE FROM coupons
WHERE id = $1
`

func (q *Queries) DeleteCoupon(ctx context.Context, id int32) error {
	_, err := q.db.ExecContext(ctx, deleteCoupon, id)
	return err
}

const getCoupon = `-- name: GetCoupon :one

SELECT id, code, discount, expiry_date, is_used, user_id, created_at FROM coupons
WHERE code = $1
LIMIT 1
`

// coupon.sql
func (q *Queries) GetCoupon(ctx context.Context, code string) (Coupons, error) {
	row := q.db.QueryRowContext(ctx, getCoupon, code)
	var i Coupons
	err := row.Scan(
		&i.ID,
		&i.Code,
		&i.Discount,
		&i.ExpiryDate,
		&i.IsUsed,
		&i.UserID,
		&i.CreatedAt,
	)
	return i, err
}

const listAvailableCoupons = `-- name: ListAvailableCoupons :many
SELECT id, code, discount, expiry_date, is_used, user_id, created_at FROM coupons
WHERE user_id IS NULL AND expiry_date >= $1
ORDER BY id
`

func (q *Queries) ListAvailableCoupons(ctx context.Context, expiryDate time.Time) ([]Coupons, error) {
	rows, err := q.db.QueryContext(ctx, listAvailableCoupons, expiryDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Coupons
	for rows.Next() {
		var i Coupons
		if err := rows.Scan(
			&i.ID,
			&i.Code,
			&i.Discount,
			&i.ExpiryDate,
			&i.IsUsed,
			&i.UserID,
			&i.CreatedAt,
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

const listCoupons = `-- name: ListCoupons :many
SELECT id, code, discount, expiry_date, is_used, user_id, created_at FROM coupons
ORDER BY id
`

func (q *Queries) ListCoupons(ctx context.Context) ([]Coupons, error) {
	rows, err := q.db.QueryContext(ctx, listCoupons)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Coupons
	for rows.Next() {
		var i Coupons
		if err := rows.Scan(
			&i.ID,
			&i.Code,
			&i.Discount,
			&i.ExpiryDate,
			&i.IsUsed,
			&i.UserID,
			&i.CreatedAt,
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

const updateCoupon = `-- name: UpdateCoupon :one
UPDATE coupons
SET code = $2, discount = $3, expiry_date = $4, is_used = $5, user_id = $6
WHERE id = $1
RETURNING id, code, discount, expiry_date, is_used, user_id, created_at
`

type UpdateCouponParams struct {
	ID         int32         `json:"id"`
	Code       string        `json:"code"`
	Discount   string        `json:"discount"`
	ExpiryDate time.Time     `json:"expiry_date"`
	IsUsed     bool          `json:"is_used"`
	UserID     sql.NullInt32 `json:"user_id"`
}

func (q *Queries) UpdateCoupon(ctx context.Context, arg UpdateCouponParams) (Coupons, error) {
	row := q.db.QueryRowContext(ctx, updateCoupon,
		arg.ID,
		arg.Code,
		arg.Discount,
		arg.ExpiryDate,
		arg.IsUsed,
		arg.UserID,
	)
	var i Coupons
	err := row.Scan(
		&i.ID,
		&i.Code,
		&i.Discount,
		&i.ExpiryDate,
		&i.IsUsed,
		&i.UserID,
		&i.CreatedAt,
	)
	return i, err
}
