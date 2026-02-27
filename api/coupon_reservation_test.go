package api

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Test constants for readability and maintainability
const (
	validUserID       = "1"
	anotherUserID     = "2"
	largeValidUserID  = "999999"
	int32MaxValue     = "2147483647" // math.MaxInt32
	overflowUserID    = "9999999999999999999"
	invalidUserID     = "invalid"
	negativeUserID    = "-1"
	zeroUserID        = "0"
)

func init() {
	// Set gin to test mode once for all tests in this package
	gin.SetMode(gin.TestMode)
}

// setupAuthTestRouter creates a test router with auth middleware and a handler
// that mimics the authorization logic of getCouponReservation.
// This tests the auth middleware + authorization check flow without database dependencies.
func setupAuthTestRouter() *gin.Engine {
	router := gin.New()

	authenticated := router.Group("/", authMiddleware())
	{
		authenticated.GET("/reservation/:user_id", func(c *gin.Context) {
			var req getCouponReservationRequest
			if err := c.ShouldBindUri(&req); err != nil {
				c.JSON(http.StatusBadRequest, errorResponse(err))
				return
			}

			userID, err := getUserIDFromContext(c)
			if err != nil {
				if errors.Is(err, errUnauthorized) {
					c.JSON(http.StatusUnauthorized, errorResponse(err))
				} else {
					c.JSON(http.StatusInternalServerError, errorResponse(err))
				}
				return
			}

			if userID != req.UserID {
				c.JSON(http.StatusForbidden, errorResponse(errAccessDenied))
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "authorized", "user_id": userID})
		})
	}

	return router
}

// TestGetCouponReservation_Authorization tests that the getCouponReservation endpoint
// properly enforces authentication and authorization to prevent IDOR attacks.
// IDOR (Insecure Direct Object Reference) allows attackers to access resources
// belonging to other users by manipulating the user_id parameter.
func TestGetCouponReservation_Authorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userIDHeader   string
		requestUserID  string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing auth header returns 401",
			userIDHeader:   "",
			requestUserID:  validUserID,
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing X-User-ID header",
		},
		{
			name:           "invalid auth header returns 400",
			userIDHeader:   invalidUserID,
			requestUserID:  validUserID,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "negative user ID in header returns 400",
			userIDHeader:   negativeUserID,
			requestUserID:  validUserID,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "zero user ID in header returns 400",
			userIDHeader:   zeroUserID,
			requestUserID:  validUserID,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "IDOR attempt - accessing other user's reservation returns 403",
			userIDHeader:   validUserID,
			requestUserID:  anotherUserID,
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
		{
			name:           "IDOR attempt - different user IDs returns 403",
			userIDHeader:   "100",
			requestUserID:  "200",
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
	}

	router := setupAuthTestRouter()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "/reservation/"+tc.requestUserID, nil)
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

// TestGetCouponReservation_URIValidation tests that invalid URI parameters
// are properly rejected with appropriate error responses.
func TestGetCouponReservation_URIValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userIDHeader   string
		requestUserID  string
		expectedStatus int
	}{
		{
			name:           "invalid user_id in URI returns 400",
			userIDHeader:   validUserID,
			requestUserID:  "invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "negative user_id in URI returns 400",
			userIDHeader:   validUserID,
			requestUserID:  "-1",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "zero user_id in URI returns 400",
			userIDHeader:   validUserID,
			requestUserID:  "0",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "float user_id in URI returns 400",
			userIDHeader:   validUserID,
			requestUserID:  "1.5",
			expectedStatus: http.StatusBadRequest,
		},
	}

	router := setupAuthTestRouter()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "/reservation/"+tc.requestUserID, nil)
			require.NoError(t, err)
			req.Header.Set("X-User-ID", tc.userIDHeader)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
		})
	}
}

// TestGetCouponReservation_ValidAccess tests that authenticated users
// can successfully access their own coupon reservations.
func TestGetCouponReservation_ValidAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		userIDHeader  string
		requestUserID string
	}{
		{
			name:          "user can access their own reservation",
			userIDHeader:  validUserID,
			requestUserID: validUserID,
		},
		{
			name:          "user with ID 100 can access reservation 100",
			userIDHeader:  "100",
			requestUserID: "100",
		},
		{
			name:          "user with large ID can access their own reservation",
			userIDHeader:  largeValidUserID,
			requestUserID: largeValidUserID,
		},
		{
			name:          "user with int32 max value can access their own reservation",
			userIDHeader:  int32MaxValue,
			requestUserID: int32MaxValue,
		},
	}

	router := setupAuthTestRouter()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "/reservation/"+tc.requestUserID, nil)
			require.NoError(t, err)
			req.Header.Set("X-User-ID", tc.userIDHeader)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "authorized")
		})
	}
}

// TestAuthMiddleware tests the authentication middleware in isolation.
// It validates header parsing, range checking, and proper error responses.
func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userIDHeader   string
		expectedStatus int
	}{
		{
			name:           "missing header returns 401",
			userIDHeader:   "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "valid header returns 200",
			userIDHeader:   "123",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid non-numeric header returns 400",
			userIDHeader:   invalidUserID,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "zero value header returns 400",
			userIDHeader:   zeroUserID,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "negative value header returns 400",
			userIDHeader:   negativeUserID,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "overflow value header returns 400",
			userIDHeader:   overflowUserID,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "int32 max value is valid",
			userIDHeader:   int32MaxValue,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "int32 max value + 1 returns 400",
			userIDHeader:   strconv.FormatInt(math.MaxInt32+1, 10),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router := gin.New()
			router.Use(authMiddleware())
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req, err := http.NewRequest(http.MethodGet, "/test", nil)
			require.NoError(t, err)

			if tc.userIDHeader != "" {
				req.Header.Set("X-User-ID", tc.userIDHeader)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
		})
	}
}

// TestGetUserIDFromContext tests the helper function that extracts
// the authenticated user ID from the Gin context.
func TestGetUserIDFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns unauthorized error when user ID not in context", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		userID, err := getUserIDFromContext(c)

		require.Error(t, err)
		require.True(t, errors.Is(err, errUnauthorized))
		require.Equal(t, int32(0), userID)
	})

	t.Run("returns user ID when present in context", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(authUserIDKey, int32(42))

		userID, err := getUserIDFromContext(c)

		require.NoError(t, err)
		require.Equal(t, int32(42), userID)
	})

	t.Run("returns internal server error when user ID has wrong type", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(authUserIDKey, "not-an-int")

		userID, err := getUserIDFromContext(c)

		require.Error(t, err)
		require.True(t, errors.Is(err, errInternalServerError))
		require.Equal(t, int32(0), userID)
	})

	t.Run("handles int32 max value correctly", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(authUserIDKey, int32(math.MaxInt32))

		userID, err := getUserIDFromContext(c)

		require.NoError(t, err)
		require.Equal(t, int32(math.MaxInt32), userID)
	})
}
