-- Drop the indexes
DROP INDEX IF EXISTS idx_coupon_reservations_user_id_reserved_at;
DROP INDEX IF EXISTS idx_coupons_user_id_created_at;
DROP INDEX IF EXISTS idx_coupons_code;
DROP INDEX IF EXISTS idx_users_email;

-- Drop the tables
DROP TABLE IF EXISTS coupon_reservations;
DROP TABLE IF EXISTS coupons;
DROP TABLE IF EXISTS users;