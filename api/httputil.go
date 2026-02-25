package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey int

const (
	authUserIDContextKey contextKey = iota
)

// errorMap replaces gin.H as the standard JSON error response type.
type errorMap = map[string]any

// writeJSON writes a JSON response with the given status code and data.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// parsePathInt32 extracts a path parameter by name and returns it as int32.
func parsePathInt32(r *http.Request, param string) (int32, error) {
	raw := r.PathValue(param)
	if raw == "" {
		return 0, fmt.Errorf("missing path parameter: %s", param)
	}
	val, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || val < 1 {
		return 0, fmt.Errorf("invalid path parameter %s: %q", param, raw)
	}
	return int32(val), nil
}

// parsePathString extracts a path parameter by name and returns it as string.
func parsePathString(r *http.Request, param string) (string, error) {
	raw := r.PathValue(param)
	if raw == "" {
		return "", fmt.Errorf("missing path parameter: %s", param)
	}
	return raw, nil
}

// clientIP extracts the client IP address from the request,
// checking X-Forwarded-For first, then falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may contain multiple IPs; the first is the client
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// middlewareChain wraps a handler with a chain of middleware functions.
// Middleware is applied in the order given, so the first middleware is outermost.
func middlewareChain(handler http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	// Apply in reverse so first middleware listed is outermost
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}
