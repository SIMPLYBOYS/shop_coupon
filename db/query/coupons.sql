-- coupon.sql

-- name: GetCoupon :one
SELECT * FROM coupons
WHERE code = $1
LIMIT 1;

-- name: ListCoupons :many
SELECT * FROM coupons
ORDER BY id;

-- name: ListAvailableCoupons :many
SELECT * FROM coupons
WHERE user_id IS NULL AND expiry_date >= $1
ORDER BY id;

-- name: CreateCoupon :one
INSERT INTO coupons (code, discount, expiry_date)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateCoupon :one
UPDATE coupons
SET code = $2, discount = $3, expiry_date = $4, is_used = $5, user_id = $6
WHERE id = $1
RETURNING *;

-- name: ClaimCoupon :one
UPDATE coupons
SET user_id = $2, is_used = true
WHERE id = $1 AND user_id IS NULL
RETURNING *;

-- name: DeleteCoupon :exec
DELETE FROM coupons
WHERE id = $1;

-- name: BatchAssignCouponsToUsers :exec
-- Batch update coupons to assign them to users
-- Uses unnest to efficiently update multiple rows in a single query
UPDATE coupons
SET user_id = batch.user_id, is_used = true
FROM (
    SELECT unnest($1::int[]) AS coupon_id, unnest($2::int[]) AS user_id
) AS batch
WHERE coupons.id = batch.coupon_id AND coupons.user_id IS NULL;