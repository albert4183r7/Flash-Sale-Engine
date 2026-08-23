package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flashsale/api-gateway/client"
	"github.com/google/uuid"
)

const internalToken = "internal-token-for-tests"

func TestCreateSendsInternalToken(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Internal-Token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "queued",
			"data":    map[string]any{"order_id": uuid.New().String()},
		})
	}))
	defer srv.Close()

	c := client.NewPurchase(srv.URL, internalToken)
	if _, err := c.Create(context.Background(), uuid.New(), uuid.New(), 1); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Without this header the purchase service rejects the call, so the gateway
	// must always send it.
	if gotToken != internalToken {
		t.Errorf("X-Internal-Token = %q, want %q", gotToken, internalToken)
	}
}

// The gateway relays the purchase service's status, so a sold-out product stays
// a 409 instead of collapsing into a generic failure.
func TestCreatePreservesUpstreamStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"conflict", http.StatusConflict},
		{"not found", http.StatusNotFound},
		{"service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				json.NewEncoder(w).Encode(map[string]any{
					"success": false,
					"message": "refused",
					"error":   "detail",
				})
			}))
			defer srv.Close()

			c := client.NewPurchase(srv.URL, internalToken)
			result, err := c.Create(context.Background(), uuid.New(), uuid.New(), 1)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if result.Status != tt.status {
				t.Errorf("Status = %d, want %d", result.Status, tt.status)
			}
			if result.Success {
				t.Error("Success = true for a refused purchase")
			}
		})
	}
}

// A proxy error page must be reported as a failed call, not decoded into a
// zero-valued success.
func TestCreateRejectsNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	c := client.NewPurchase(srv.URL, internalToken)
	if _, err := c.Create(context.Background(), uuid.New(), uuid.New(), 1); err == nil {
		t.Fatal("Create() = nil error for a non-JSON response, want an error")
	}
}

func TestCreatePropagatesContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := client.NewPurchase(srv.URL, internalToken)
	if _, err := c.Create(ctx, uuid.New(), uuid.New(), 1); err == nil {
		t.Fatal("Create() with a cancelled context = nil, want an error")
	}
}

func TestRestoreStockReportsUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := client.NewPurchase(srv.URL, internalToken)
	if err := c.RestoreStock(context.Background(), uuid.New(), 2); err == nil {
		t.Fatal("RestoreStock() = nil for a rejected call, want an error")
	}
}

func TestRestoreStockSucceeds(t *testing.T) {
	var body struct {
		ProductID uuid.UUID `json:"product_id"`
		Qty       int       `json:"qty"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	productID := uuid.New()
	c := client.NewPurchase(srv.URL, internalToken)
	if err := c.RestoreStock(context.Background(), productID, 3); err != nil {
		t.Fatalf("RestoreStock() error = %v", err)
	}
	if body.ProductID != productID || body.Qty != 3 {
		t.Errorf("request body = %+v, want product %s qty 3", body, productID)
	}
}
