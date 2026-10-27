// Package middleware holds the purchase service's HTTP middleware.
package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
)

// InternalAuth rejects requests that do not present the shared internal token.
//
// The purchase service takes the buyer's identity from the request body and can
// mutate stock directly, so every route it exposes is meant for the API gateway
// alone. Without this guard anyone able to reach the service could order as any
// user or inflate stock at will.
func InternalAuth(token string) gin.HandlerFunc {
	expected := []byte(token)

	return func(c *gin.Context) {
		provided := []byte(c.GetHeader("X-Internal-Token"))

		// Constant-time comparison keeps the check from leaking the token
		// through response timing.
		if subtle.ConstantTimeCompare(provided, expected) != 1 {
			response.Abort(c, http.StatusUnauthorized,
				"Unauthorized", "A valid internal service token is required")
			return
		}
		c.Next()
	}
}
