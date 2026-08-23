package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flashsale/purchase-service/middleware"
	"github.com/gin-gonic/gin"
)

const token = "the-internal-service-token"

func init() { gin.SetMode(gin.TestMode) }

func serve(header string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/internal", middleware.InternalAuth(token), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	if header != "" {
		req.Header.Set("X-Internal-Token", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestInternalAuthAcceptsCorrectToken(t *testing.T) {
	if got := serve(token).Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
}

// These routes let a caller order as any user and change stock at will, so
// anything other than the exact token must be refused.
func TestInternalAuthRejects(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing token", ""},
		{"wrong token", "not-the-token"},
		{"empty token", " "},
		{"token prefix", token[:len(token)-1]},
		{"token with suffix", token + "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serve(tt.header).Code; got != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", got, http.StatusUnauthorized)
			}
		})
	}
}
