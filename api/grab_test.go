package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupAuthTimeRestrictedTestRouter creates a test router with auth middleware
// that uses the same authorization logic as handleGrabRequest and createCouponReservation.
// Note: We don't include specialTime middleware in tests as it depends on global time config.
func setupAuthTimeRestrictedTestRouter() *gin.Engine {
	router := gin.New()

	authenticated := router.Group("/", authMiddleware())
	{
		// Simulated /grab handler with auth check using helper function
		authenticated.POST("/grab", func(c *gin.Context) {
			var req getGrabRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, errorResponse(err))
				return
			}

			if err := checkUserAuthorization(c, req.UserID); err != nil {
				return
			}

			authUserID, _ := getUserIDFromContext(c)
			c.JSON(http.StatusOK, gin.H{"status": "authorized", "user_id": authUserID})
		})

		// Simulated /reserve handler with auth check using helper function
		authenticated.POST("/reserve", func(c *gin.Context) {
			var req createCouponReservationRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, errorResponse(err))
				return
			}

			if err := checkUserAuthorization(c, req.UserID); err != nil {
				return
			}

			authUserID, _ := getUserIDFromContext(c)
			c.JSON(http.StatusOK, gin.H{"status": "authorized", "user_id": authUserID})
		})
	}

	return router
}

// TestGrabEndpoint_Authentication tests that the /grab endpoint requires authentication.
func TestGrabEndpoint_Authentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userIDHeader   string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing auth header returns 401",
			userIDHeader:   "",
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing X-User-ID header",
		},
		{
			name:           "invalid auth header returns 400",
			userIDHeader:   "invalid",
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "IDOR attempt - grabbing for another user returns 403",
			userIDHeader:   "1",
			requestBody:    map[string]interface{}{"user_id": 2},
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
		{
			name:           "valid auth - user can grab for themselves",
			userIDHeader:   "1",
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router := setupAuthTimeRestrictedTestRouter()

			body, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "/grab", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			if tc.userIDHeader != "" {
				req.Header.Set("X-User-ID", tc.userIDHeader)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
			if tc.expectedError != "" {
				require.Contains(t, recorder.Body.String(), tc.expectedError)
			}
		})
	}
}

// TestReserveEndpoint_Authentication tests that the /reserve endpoint requires authentication.
func TestReserveEndpoint_Authentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userIDHeader   string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing auth header returns 401",
			userIDHeader:   "",
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing X-User-ID header",
		},
		{
			name:           "invalid auth header returns 400",
			userIDHeader:   "invalid",
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid X-User-ID header",
		},
		{
			name:           "IDOR attempt - reserving for another user returns 403",
			userIDHeader:   "1",
			requestBody:    map[string]interface{}{"user_id": 2},
			expectedStatus: http.StatusForbidden,
			expectedError:  "access denied",
		},
		{
			name:           "valid auth - user can reserve for themselves",
			userIDHeader:   "1",
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "valid auth with large user ID",
			userIDHeader:   "999999",
			requestBody:    map[string]interface{}{"user_id": 999999},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router := setupAuthTimeRestrictedTestRouter()

			body, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "/reserve", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			if tc.userIDHeader != "" {
				req.Header.Set("X-User-ID", tc.userIDHeader)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
			if tc.expectedError != "" {
				require.Contains(t, recorder.Body.String(), tc.expectedError)
			}
		})
	}
}

// TestTimeRestrictedEndpoints_InvalidRequestBody tests that endpoints properly
// validate the request body.
func TestTimeRestrictedEndpoints_InvalidRequestBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		endpoint    string
		requestBody string
	}{
		{
			name:        "grab with missing user_id",
			endpoint:    "/grab",
			requestBody: `{}`,
		},
		{
			name:        "grab with invalid user_id",
			endpoint:    "/grab",
			requestBody: `{"user_id": 0}`,
		},
		{
			name:        "grab with negative user_id",
			endpoint:    "/grab",
			requestBody: `{"user_id": -1}`,
		},
		{
			name:        "reserve with missing user_id",
			endpoint:    "/reserve",
			requestBody: `{}`,
		},
		{
			name:        "reserve with invalid user_id",
			endpoint:    "/reserve",
			requestBody: `{"user_id": 0}`,
		},
		{
			name:        "reserve with negative user_id",
			endpoint:    "/reserve",
			requestBody: `{"user_id": -1}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router := setupAuthTimeRestrictedTestRouter()

			req, err := http.NewRequest(http.MethodPost, tc.endpoint, bytes.NewBufferString(tc.requestBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-ID", "1")

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

// TestBoundaryValues_Int32Max tests that endpoints handle int32 max value correctly.
func TestBoundaryValues_Int32Max(t *testing.T) {
	t.Parallel()

	const int32Max = 2147483647

	tests := []struct {
		name           string
		endpoint       string
		userIDHeader   string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name:           "grab with int32 max user ID",
			endpoint:       "/grab",
			userIDHeader:   "2147483647",
			requestBody:    map[string]interface{}{"user_id": int32Max},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "reserve with int32 max user ID",
			endpoint:       "/reserve",
			userIDHeader:   "2147483647",
			requestBody:    map[string]interface{}{"user_id": int32Max},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "auth header exceeds int32 max returns 400",
			endpoint:       "/grab",
			userIDHeader:   "2147483648", // int32 max + 1
			requestBody:    map[string]interface{}{"user_id": 1},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router := setupAuthTimeRestrictedTestRouter()

			body, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, tc.endpoint, bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-ID", tc.userIDHeader)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.expectedStatus, recorder.Code)
		})
	}
}

// TestCheckUserAuthorization tests the checkUserAuthorization helper function.
func TestCheckUserAuthorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setupContext    func(c *gin.Context)
		requestedUserID int32
		expectError     bool
		expectedStatus  int
	}{
		{
			name: "authorized - matching user IDs",
			setupContext: func(c *gin.Context) {
				c.Set("api_auth_user_id", int32(123))
			},
			requestedUserID: 123,
			expectError:     false,
			expectedStatus:  0, // No response set
		},
		{
			name: "unauthorized - missing auth context",
			setupContext: func(c *gin.Context) {
				// Don't set auth user ID
			},
			requestedUserID: 123,
			expectError:     true,
			expectedStatus:  http.StatusUnauthorized,
		},
		{
			name: "forbidden - mismatched user IDs",
			setupContext: func(c *gin.Context) {
				c.Set("api_auth_user_id", int32(456))
			},
			requestedUserID: 123,
			expectError:     true,
			expectedStatus:  http.StatusForbidden,
		},
		{
			name: "internal error - invalid auth context type",
			setupContext: func(c *gin.Context) {
				c.Set("api_auth_user_id", "not-an-int32")
			},
			requestedUserID: 123,
			expectError:     true,
			expectedStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			tc.setupContext(c)

			err := checkUserAuthorization(c, tc.requestedUserID)

			if tc.expectError {
				require.Error(t, err)
				require.Equal(t, tc.expectedStatus, recorder.Code)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
