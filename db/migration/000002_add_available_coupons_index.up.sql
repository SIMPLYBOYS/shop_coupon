-- Create a partial index for ListAvailableCoupons query optimization
-- This index covers: WHERE user_id IS NULL AND expiry_date >= $1 ORDER BY id
CREATE INDEX idx_coupons_available ON coupons (expiry_date, id) WHERE user_id IS NULL;
