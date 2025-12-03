package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PurchaseClient is a client for the Purchase Service
type PurchaseClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPurchaseClient creates a new Purchase Service client
func NewPurchaseClient(baseURL string) *PurchaseClient {
	return &PurchaseClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// PurchaseRequest represents the purchase request to the service
type PurchaseRequest struct {
	UserID    int `json:"user_id"`
	ProductID int `json:"product_id"`
	Qty       int `json:"qty"`
}

// PurchaseResponse represents the response from Purchase Service
type PurchaseResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	OrderID string `json:"order_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Purchase sends a purchase request to the Purchase Service
func (c *PurchaseClient) Purchase(userID, productID, qty int) (*PurchaseResponse, error) {
	req := PurchaseRequest{
		UserID:    userID,
		ProductID: productID,
		Qty:       qty,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/purchase",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result PurchaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
