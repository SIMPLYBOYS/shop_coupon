package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetCouponReservation_Authorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
			requestUserID:  "1",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing X-User-ID header",
		},
		{
			name:           "invalid auth header returns 400",
			userIDHeader:   "invalid",
			requestUserID:  "1",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "negative user ID in header returns 400",
			userIDHeader:   "-1",
			requestUserID:  "1",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "zero user ID in header returns 400",
			userIDHeader:   "0",
			requestUserID:  "1",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "IDOR attempt - accessing other user's reservation returns 403",
			userIDHeader:   "1",
			requestUserID:  "2",
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()

			// Set up the authenticated route group like in production
			authenticated := router.Group("/", authMiddleware())
			{
				authenticated.GET("/reservation/:user_id", func(c *gin.Context) {
					// Simulate the handler's authorization check
					var req getCouponReservationRequest
					if err := c.ShouldBindUri(&req); err != nil {
						c.JSON(http.StatusBadRequest, errorResponse(err))
						return
					}

					userID, err := getUserIDFromContext(c)
					if err != nil {
						if err.Error() == "unauthorized" {
							c.JSON(http.StatusUnauthorized, errorResponse(err))
						} else {
							c.JSON(http.StatusInternalServerError, errorResponse(err))
						}
						return
					}

					if userID != req.UserID {
						c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
						return
					}

					// If we reach here, authorization passed
					c.JSON(http.StatusOK, gin.H{"status": "authorized"})
				})
			}

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

func TestGetCouponReservation_ValidAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		userIDHeader  string
		requestUserID string
	}{
		{
			name:          "user can access their own reservation",
			userIDHeader:  "1",
			requestUserID: "1",
		},
		{
			name:          "user with ID 100 can access reservation 100",
			userIDHeader:  "100",
			requestUserID: "100",
		},
		{
			name:          "user with large ID can access their own reservation",
			userIDHeader:  "999999",
			requestUserID: "999999",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()

			// Set up the authenticated route group
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
						c.JSON(http.StatusUnauthorized, errorResponse(err))
						return
					}

					if userID != req.UserID {
						c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
						return
					}

					// Authorization passed
					c.JSON(http.StatusOK, gin.H{"status": "authorized", "user_id": userID})
				})
			}

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

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userIDHeader   string
		expectedStatus int
		shouldPass     bool
	}{
		{
			name:           "missing header",
			userIDHeader:   "",
			expectedStatus: http.StatusUnauthorized,
			shouldPass:     false,
		},
		{
			name:           "valid header",
			userIDHeader:   "123",
			expectedStatus: http.StatusOK,
			shouldPass:     true,
		},
		{
			name:           "invalid non-numeric header",
			userIDHeader:   "abc",
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
		},
		{
			name:           "zero value header",
			userIDHeader:   "0",
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
		},
		{
			name:           "negative value header",
			userIDHeader:   "-5",
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
		},
		{
			name:           "overflow value header",
			userIDHeader:   "9999999999999999999",
			expectedStatus: http.StatusBadRequest,
			shouldPass:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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

func TestGetUserIDFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns error when user ID not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		userID, err := getUserIDFromContext(c)

		require.Error(t, err)
		require.Equal(t, "unauthorized", err.Error())
		require.Equal(t, int32(0), userID)
	})

	t.Run("returns user ID when present in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(authUserIDKey, int32(42))

		userID, err := getUserIDFromContext(c)

		require.NoError(t, err)
		require.Equal(t, int32(42), userID)
	})

	t.Run("returns error when user ID has wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(authUserIDKey, "not-an-int")

		userID, err := getUserIDFromContext(c)

		require.Error(t, err)
		require.Equal(t, "internal server error", err.Error())
		require.Equal(t, int32(0), userID)
	})
}
