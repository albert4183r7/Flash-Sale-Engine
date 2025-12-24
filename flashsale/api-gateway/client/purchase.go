package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// PurchaseClient handles HTTP calls to purchase-service
type PurchaseClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPurchaseClient creates a new PurchaseClient
func NewPurchaseClient(baseURL string) *PurchaseClient {
	return &PurchaseClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// PurchaseRequest represents a purchase request to purchase-service
type PurchaseRequest struct {
	UserID        uuid.UUID `json:"user_id"`
	ProductID     uuid.UUID `json:"product_id"`
	Qty           int       `json:"qty"`
	Notes         string    `json:"notes,omitempty"`
	PaymentMethod string    `json:"payment_method,omitempty"`
}

// PurchaseData contains the data portion of the response
type PurchaseData struct {
	OrderID     uuid.UUID `json:"order_id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Qty         int       `json:"qty"`
	Status      string    `json:"status"`
}

// PurchaseResponse represents the response from purchase-service
type PurchaseResponse struct {
	Success bool         `json:"success"`
	Data    PurchaseData `json:"data"`
	Message string       `json:"message"`
	Error   string       `json:"error,omitempty"`
}

// Purchase sends a purchase request to purchase-service
func (c *PurchaseClient) Purchase(userID, productID uuid.UUID, qty int, notes, paymentMethod string) (*PurchaseResponse, error) {
	req := PurchaseRequest{
		UserID:        userID,
		ProductID:     productID,
		Qty:           qty,
		Notes:         notes,
		PaymentMethod: paymentMethod,
	}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/purchase", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result PurchaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RestoreStock calls the internal endpoint to increment Redis stock
func (c *PurchaseClient) RestoreStock(productID uuid.UUID, qty int) error {
	req := map[string]interface{}{"product_id": productID, "qty": qty}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/internal/stock/restore", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to restore stock, status: %d", resp.StatusCode)
	}
	return nil
}