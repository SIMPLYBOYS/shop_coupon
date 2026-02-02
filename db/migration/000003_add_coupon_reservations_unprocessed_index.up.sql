-- Create a partial index for ListCouponReservations query optimization
-- This index covers: WHERE is_processed = FALSE ORDER BY id
-- Related queries: ListCouponReservations, ListCouponReservationsWithLimit
CREATE INDEX idx_coupon_reservations_unprocessed ON coupon_reservations (id) WHERE is_processed = FALSE;
