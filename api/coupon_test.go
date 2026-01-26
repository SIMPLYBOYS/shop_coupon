package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// mockCoupon simulates a coupon for testing authorization logic
type mockCoupon struct {
	Code   string
	UserID sql.NullInt32
}

// setupCouponTestRouter creates a test router with auth middleware and a handler
// that mimics the authorization logic of getCoupon without database dependencies.
func setupCouponTestRouter(coupons map[string]mockCoupon) *gin.Engine {
	router := gin.New()

	authenticated := router.Group("/", authMiddleware())
	{
		authenticated.GET("/coupon/:code", func(c *gin.Context) {
			var req getCouponRequest
			if err := c.ShouldBindUri(&req); err != nil {
				c.JSON(http.StatusBadRequest, errorResponse(err))
				return
			}

			userID, err := getUserIDFromContext(c)
			if err != nil {
				c.JSON(http.StatusUnauthorized, errorResponse(ErrUnauthorized))
				return
			}

			coupon, exists := coupons[req.Code]
			if !exists {
				c.JSON(http.StatusNotFound, gin.H{"error": "coupon not found"})
				return
			}

			// Authorization check: only coupon owner can view the coupon
			if !coupon.UserID.Valid || coupon.UserID.Int32 != userID {
				c.JSON(http.StatusForbidden, errorResponse(ErrAccessDenied))
				return
			}

			c.JSON(http.StatusOK, coupon)
		})
	}

	return router
}

// TestGetCoupon_Authentication tests that the /coupon/:code endpoint requires authentication.
func TestGetCoupon_Authentication(t *testing.T) {
	t.Parallel()

	coupons := map[string]mockCoupon{
		"COUPON-ABC123": {Code: "COUPON-ABC123", UserID: sql.NullInt32{Int32: 1, Valid: true}},
	}

	tests := []struct {
		name           string
		userIDHeader   string
		couponCode     string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing auth header returns 401",
			userIDHeader:   "",
			couponCode:     "COUPON-ABC123",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing X-User-ID header",
		},
		{
			name:           "invalid auth header returns 400",
			userIDHeader:   "invalid",
			couponCode:     "COUPON-ABC123",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "negative user ID returns 400",
			userIDHeader:   "-1",
			couponCode:     "COUPON-ABC123",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "zero user ID returns 400",
			userIDHeader:   "0",
			couponCode:     "COUPON-ABC123",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
	}

	router := setupCouponTestRouter(coupons)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "/coupon/"+tc.couponCode, nil)
			require.NoError(t, err)

			if tc.userIDHeader != "" {
				req.Header.Set("X-User-ID", tc.userIDHeader)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
			require.Contains(t, recorder.Body.String(), tc.expectedError)
		})
	}
}

// TestGetCoupon_Authorization tests that the /coupon/:code endpoint
// properly enforces authorization - only coupon owner can view their coupon.
func TestGetCoupon_Authorization(t *testing.T) {
	t.Parallel()

	coupons := map[string]mockCoupon{
		"COUPON-USER1": {Code: "COUPON-USER1", UserID: sql.NullInt32{Int32: 1, Valid: true}},
		"COUPON-USER2": {Code: "COUPON-USER2", UserID: sql.NullInt32{Int32: 2, Valid: true}},
		"COUPON-UNASSIGNED": {Code: "COUPON-UNASSIGNED", UserID: sql.NullInt32{Valid: false}},
	}

	tests := []struct {
		name           string
		userIDHeader   string
		couponCode     string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "owner can access their own coupon",
			userIDHeader:   "1",
			couponCode:     "COUPON-USER1",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "user cannot access another user's coupon",
			userIDHeader:   "1",
			couponCode:     "COUPON-USER2",
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
		{
			name:           "user cannot access unassigned coupon",
			userIDHeader:   "1",
			couponCode:     "COUPON-UNASSIGNED",
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
		{
			name:           "different user cannot access unassigned coupon",
			userIDHeader:   "999",
			couponCode:     "COUPON-UNASSIGNED",
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
		{
			name:           "non-existent coupon returns 404",
			userIDHeader:   "1",
			couponCode:     "COUPON-NONEXISTENT",
			expectedStatus: http.StatusNotFound,
			expectedError:  "coupon not found",
		},
	}

	router := setupCouponTestRouter(coupons)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "/coupon/"+tc.couponCode, nil)
			require.NoError(t, err)
			req.Header.Set("X-User-ID", tc.userIDHeader)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
			if tc.expectedError != "" {
				require.Contains(t, recorder.Body.String(), tc.expectedError)
			}
		})
	}
}

// TestGetCoupon_ValidAccess tests successful access scenarios.
func TestGetCoupon_ValidAccess(t *testing.T) {
	t.Parallel()

	coupons := map[string]mockCoupon{
		"COUPON-USER1":   {Code: "COUPON-USER1", UserID: sql.NullInt32{Int32: 1, Valid: true}},
		"COUPON-USER100": {Code: "COUPON-USER100", UserID: sql.NullInt32{Int32: 100, Valid: true}},
		"COUPON-MAXINT":  {Code: "COUPON-MAXINT", UserID: sql.NullInt32{Int32: 2147483647, Valid: true}},
	}

	tests := []struct {
		name         string
		userIDHeader string
		couponCode   string
	}{
		{
			name:         "user 1 can access their coupon",
			userIDHeader: "1",
			couponCode:   "COUPON-USER1",
		},
		{
			name:         "user 100 can access their coupon",
			userIDHeader: "100",
			couponCode:   "COUPON-USER100",
		},
		{
			name:         "user with max int32 ID can access their coupon",
			userIDHeader: "2147483647",
			couponCode:   "COUPON-MAXINT",
		},
	}

	router := setupCouponTestRouter(coupons)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "/coupon/"+tc.couponCode, nil)
			require.NoError(t, err)
			req.Header.Set("X-User-ID", tc.userIDHeader)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusOK, recorder.Code)
		})
	}
}
