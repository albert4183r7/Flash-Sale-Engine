package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flashsale/api-gateway/middleware"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "a-test-secret-at-least-16-chars"

func init() { gin.SetMode(gin.TestMode) }

// sign builds a token with the given claims and signing method.
func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func validClaims(userID uuid.UUID) jwt.MapClaims {
	return jwt.MapClaims{
		"user_id": userID.String(),
		"email":   "buyer@example.com",
		"role":    "user",
		"exp":     time.Now().Add(time.Hour).Unix(),
	}
}

// serve runs a request with the given Authorization header through the
// middleware and reports the status plus the user ID the handler observed.
func serve(t *testing.T, authHeader string) (int, uuid.UUID) {
	t.Helper()

	r := gin.New()
	var seen uuid.UUID
	r.GET("/protected", middleware.JWTAuth(testSecret), func(c *gin.Context) {
		id, ok := middleware.UserID(c)
		if !ok {
			t.Error("UserID() reported no authenticated user inside a protected route")
		}
		seen = id
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w.Code, seen
}

func TestJWTAuthAcceptsValidToken(t *testing.T) {
	userID := uuid.New()
	status, seen := serve(t, "Bearer "+sign(t, jwt.SigningMethodHS256, []byte(testSecret), validClaims(userID)))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if seen != userID {
		t.Errorf("user id in context = %s, want %s", seen, userID)
	}
}

// The scheme is matched case-insensitively, as RFC 7235 requires.
func TestJWTAuthAcceptsAnyCaseScheme(t *testing.T) {
	token := sign(t, jwt.SigningMethodHS256, []byte(testSecret), validClaims(uuid.New()))
	for _, header := range []string{"Bearer " + token, "bearer " + token, "BEARER " + token} {
		if status, _ := serve(t, header); status != http.StatusOK {
			t.Errorf("header %q: status = %d, want %d", header, status, http.StatusOK)
		}
	}
}

func TestJWTAuthRejects(t *testing.T) {
	userID := uuid.New()

	expired := validClaims(userID)
	expired["exp"] = time.Now().Add(-time.Minute).Unix()

	noExpiry := validClaims(userID)
	delete(noExpiry, "exp")

	nilUser := validClaims(uuid.Nil)
	badUser := validClaims(userID)
	badUser["user_id"] = "not-a-uuid"

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"no scheme", sign(t, jwt.SigningMethodHS256, []byte(testSecret), validClaims(userID))},
		{"wrong scheme", "Basic dXNlcjpwYXNz"},
		{"empty token", "Bearer "},
		{"garbage token", "Bearer not.a.token"},
		{"expired", "Bearer " + sign(t, jwt.SigningMethodHS256, []byte(testSecret), expired)},
		// A token with no expiry would never stop working if it leaked.
		{"missing expiry", "Bearer " + sign(t, jwt.SigningMethodHS256, []byte(testSecret), noExpiry)},
		// Signed correctly, but identifies nobody: it must not fall through as
		// the zero UUID, which would match no user's orders but should never be
		// treated as an identity at all.
		{"nil user id", "Bearer " + sign(t, jwt.SigningMethodHS256, []byte(testSecret), nilUser)},
		{"unparseable user id", "Bearer " + sign(t, jwt.SigningMethodHS256, []byte(testSecret), badUser)},
		{"wrong secret", "Bearer " + sign(t, jwt.SigningMethodHS256, []byte("some-other-secret-value"), validClaims(userID))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/protected", middleware.JWTAuth(testSecret), func(c *gin.Context) {
				t.Error("handler ran for a request that should have been rejected")
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}

			var body struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not the standard envelope: %v", err)
			}
			if body.Success {
				t.Error("rejected request reported success")
			}
		})
	}
}

// A token signed with "none" carries no proof of anything. Accepting one would
// let anybody mint an identity, so the middleware pins the algorithm.
func TestJWTAuthRejectsUnsignedToken(t *testing.T) {
	token := sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims(uuid.New()))

	if status, _ := serve(t, "Bearer "+token); status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for an alg=none token", status, http.StatusUnauthorized)
	}
}

// UserID must report false rather than a zero UUID when the request never went
// through the middleware, so a handler cannot mistake "nobody" for a user.
func TestUserIDWithoutAuthentication(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if id, ok := middleware.UserID(c); ok {
		t.Fatalf("UserID() = (%s, true), want (uuid.Nil, false)", id)
	}
}
