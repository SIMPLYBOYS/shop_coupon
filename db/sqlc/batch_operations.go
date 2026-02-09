package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// MaxBatchSize limits the number of records per batch to prevent SQL statement overflow
const MaxBatchSize = 1000

// BatchCreateCouponsParams holds parameters for batch coupon creation
type BatchCreateCouponsParams struct {
	Codes      []string
	Discount   string
	ExpiryDate time.Time
}

// BatchCreateCoupons creates multiple coupons in a single database operation
// using a multi-row INSERT statement for better performance.
// If the batch size exceeds MaxBatchSize, it will be processed in chunks.
func (q *Queries) BatchCreateCoupons(ctx context.Context, params BatchCreateCouponsParams) (int64, error) {
	if len(params.Codes) == 0 {
		return 0, nil
	}

	var totalCreated int64

	// Process in chunks to avoid SQL statement size limits
	for start := 0; start < len(params.Codes); start += MaxBatchSize {
		end := start + MaxBatchSize
		if end > len(params.Codes) {
			end = len(params.Codes)
		}

		chunk := params.Codes[start:end]
		created, err := q.batchCreateCouponsChunk(ctx, chunk, params.Discount, params.ExpiryDate)
		if err != nil {
			return totalCreated, fmt.Errorf("batch create coupons chunk [%d:%d] failed: %w", start, end, err)
		}
		totalCreated += created
	}

	return totalCreated, nil
}

// batchCreateCouponsChunk creates a chunk of coupons (internal helper)
func (q *Queries) batchCreateCouponsChunk(ctx context.Context, codes []string, discount string, expiryDate time.Time) (int64, error) {
	if len(codes) == 0 {
		return 0, nil
	}

	// Build multi-row INSERT statement
	valueStrings := make([]string, 0, len(codes))
	valueArgs := make([]interface{}, 0, len(codes)*3)

	for i, code := range codes {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d)", i*3+1, i*3+2, i*3+3))
		valueArgs = append(valueArgs, code, discount, expiryDate)
	}

	query := fmt.Sprintf(
		"INSERT INTO coupons (code, discount, expiry_date) VALUES %s",
		strings.Join(valueStrings, ", "),
	)

	result, err := q.db.ExecContext(ctx, query, valueArgs...)
	if err != nil {
		return 0, fmt.Errorf("batch create coupons chunk exec: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("batch create coupons chunk rows affected: %w", err)
	}

	return rowsAffected, nil
}

// BatchMarkCouponReservationsAsProcessed marks multiple coupon reservations as processed
// using a single UPDATE query with ANY clause for better performance
func (q *Queries) BatchMarkCouponReservationsAsProcessed(ctx context.Context, ids []int32) error {
	if len(ids) == 0 {
		return nil
	}

	query := "UPDATE coupon_reservations SET is_processed = TRUE WHERE id = ANY($1)"
	_, err := q.db.ExecContext(ctx, query, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("batch mark reservations as processed: %w", err)
	}

	return nil
}

// BatchAssignCouponsToUsersParams holds parameters for batch coupon assignment
type BatchAssignCouponsToUsersParams struct {
	CouponIDs []int32
	UserIDs   []int32
}

// BatchAssignCouponsToUsers assigns multiple coupons to users in a single database operation
// using unnest to efficiently update multiple rows
func (q *Queries) BatchAssignCouponsToUsers(ctx context.Context, params BatchAssignCouponsToUsersParams) (int64, error) {
	if len(params.CouponIDs) == 0 || len(params.UserIDs) == 0 {
		return 0, nil
	}

	if len(params.CouponIDs) != len(params.UserIDs) {
		return 0, fmt.Errorf("coupon_ids and user_ids length mismatch: %d != %d", len(params.CouponIDs), len(params.UserIDs))
	}

	// Use int4[] (integer[]) for PostgreSQL type safety with Go int32
	query := `
		UPDATE coupons
		SET user_id = batch.user_id, is_used = true
		FROM (
			SELECT unnest($1::int4[]) AS coupon_id, unnest($2::int4[]) AS user_id
		) AS batch
		WHERE coupons.id = batch.coupon_id AND coupons.user_id IS NULL
	`

	result, err := q.db.ExecContext(ctx, query, pq.Array(params.CouponIDs), pq.Array(params.UserIDs))
	if err != nil {
		return 0, fmt.Errorf("batch assign coupons to users: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("batch assign coupons to users rows affected: %w", err)
	}

	return rowsAffected, nil
}

// BatchCreateCouponsWithTx creates multiple coupons in a transaction for atomicity
// Uses ReadCommitted isolation level for balance between consistency and performance
func (s *Store) BatchCreateCouponsWithTx(ctx context.Context, params BatchCreateCouponsParams, reservationIDs []int32) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.Queries.WithTx(tx)

	// Create coupons
	created, err := qtx.BatchCreateCoupons(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("batch create coupons: %w", err)
	}

	// Mark reservations as processed
	if len(reservationIDs) > 0 {
		err = qtx.BatchMarkCouponReservationsAsProcessed(ctx, reservationIDs)
		if err != nil {
			return 0, fmt.Errorf("mark reservations as processed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return created, nil
}

