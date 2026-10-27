// Package middleware holds the API gateway's HTTP middleware.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Context keys under which the authenticated identity is stored.
const (
	ContextUserID = "user_id"
	ContextEmail  = "email"
	ContextRole   = "role"
)

// Claims are the claims the gateway issues and expects.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// JWTAuth authenticates requests using a bearer token.
func JWTAuth(secret string) gin.HandlerFunc {
	key := []byte(secret)

	return func(c *gin.Context) {
		token, err := bearerToken(c.GetHeader("Authorization"))
		if err != nil {
			response.Abort(c, http.StatusUnauthorized, "Authorization required", err.Error())
			return
		}

		var claims Claims
		// Pinning the accepted algorithm stops a token signed with "none", or
		// with an asymmetric algorithm confusion trick, from being accepted.
		parsed, err := jwt.ParseWithClaims(token, &claims,
			func(*jwt.Token) (any, error) { return key, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !parsed.Valid {
			response.Abort(c, http.StatusUnauthorized, "Invalid token",
				"Token is invalid or expired")
			return
		}

		userID, err := uuid.Parse(claims.UserID)
		if err != nil || userID == uuid.Nil {
			// A well-signed token without a usable subject cannot authorise
			// anything, and must not fall through as the zero UUID.
			response.Abort(c, http.StatusUnauthorized, "Invalid token",
				"Token does not identify a user")
			return
		}

		c.Set(ContextUserID, userID)
		c.Set(ContextEmail, claims.Email)
		c.Set(ContextRole, claims.Role)

		c.Next()
	}
}

// bearerToken extracts the credential from an Authorization header.
func bearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("missing Authorization header")
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") || strings.TrimSpace(token) == "" {
		return "", errors.New("authorization header must be: Bearer <token>")
	}
	return strings.TrimSpace(token), nil
}

// UserID returns the authenticated user's ID. It reports false when the request
// did not pass through JWTAuth, so handlers never assume an identity.
func UserID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get(ContextUserID)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := value.(uuid.UUID)
	return id, ok
}
