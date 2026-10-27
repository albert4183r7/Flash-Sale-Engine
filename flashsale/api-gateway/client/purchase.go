// Package client talks to the purchase service over HTTP.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/common/response"
	"github.com/google/uuid"
)

// Purchase calls the purchase service.
type Purchase struct {
	baseURL       string
	internalToken string
	http          *http.Client
}

// NewPurchase creates a client for the purchase service at baseURL. The token
// identifies this gateway as a trusted caller.
func NewPurchase(baseURL, internalToken string) *Purchase {
	return &Purchase{
		baseURL:       baseURL,
		internalToken: internalToken,
		http:          &http.Client{Timeout: 10 * time.Second},
	}
}

type purchaseRequest struct {
	UserID    uuid.UUID `json:"user_id"`
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
}

type restoreStockRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
}

// Result is the purchase service's answer to a purchase attempt.
type Result struct {
	// Status is the HTTP status the purchase service returned, so the gateway
	// can pass a sold-out or duplicate outcome through unchanged instead of
	// flattening every refusal into one code.
	Status  int
	Success bool
	Message string
	Error   string
	OrderID uuid.UUID
}

type purchaseResponse struct {
	response.Body
	Data struct {
		OrderID uuid.UUID `json:"order_id"`
	} `json:"data"`
}

// Create asks the purchase service to reserve stock and queue an order.
func (p *Purchase) Create(ctx context.Context, userID, productID uuid.UUID, qty int) (Result, error) {
	var body purchaseResponse
	status, err := p.do(ctx, http.MethodPost, "/purchase", purchaseRequest{
		UserID:    userID,
		ProductID: productID,
		Qty:       qty,
	}, &body)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Status:  status,
		Success: body.Success,
		Message: body.Message,
		Error:   body.Error,
		OrderID: body.Data.OrderID,
	}, nil
}

// RestoreStock returns cancelled units to the in-memory stock counter.
func (p *Purchase) RestoreStock(ctx context.Context, productID uuid.UUID, qty int) error {
	status, err := p.do(ctx, http.MethodPost, "/internal/stock/restore", restoreStockRequest{
		ProductID: productID,
		Qty:       qty,
	}, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("purchase service returned %d restoring stock for product %s", status, productID)
	}
	return nil
}

type pendingOrderResponse struct {
	response.Body
	Data models.OrderEvent `json:"data"`
}

// PendingOrder returns an order the purchase service has accepted but the
// worker may not have stored yet. It reports false when no such order is held.
func (p *Purchase) PendingOrder(ctx context.Context, orderID uuid.UUID) (models.OrderEvent, bool, error) {
	var body pendingOrderResponse
	status, err := p.do(ctx, http.MethodGet, "/internal/orders/"+orderID.String()+"/pending", nil, &body)
	if err != nil {
		return models.OrderEvent{}, false, err
	}
	switch status {
	case http.StatusOK:
		return body.Data, true, nil
	case http.StatusNotFound:
		return models.OrderEvent{}, false, nil
	default:
		return models.OrderEvent{}, false, fmt.Errorf("purchase service returned %d reading pending order %s", status, orderID)
	}
}

// do sends a request with an optional JSON payload and decodes the JSON
// response into out, which may be nil when the body is not needed.
func (p *Purchase) do(ctx context.Context, method, path string, payload, out any) (int, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, fmt.Errorf("encode request for %s: %w", path, err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return 0, fmt.Errorf("build request for %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", p.internalToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("call purchase service %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if out != nil {
		// A non-JSON body (a proxy error page, for example) must be reported as
		// a failed call rather than decoded into a zero-valued success.
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response from %s (status %d): %w", path, resp.StatusCode, err)
		}
	}
	return resp.StatusCode, nil
}
