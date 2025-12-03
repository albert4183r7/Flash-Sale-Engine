package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type PurchaseClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPurchaseClient(baseURL string) *PurchaseClient {
	return &PurchaseClient{
		baseURL: baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type PurchaseRequest struct {
	UserID    int `json:"user_id"`
	ProductID int `json:"product_id"`
	Qty       int `json:"qty"`
}

type PurchaseResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	OrderID string `json:"order_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (c *PurchaseClient) Purchase(userID, productID, qty int) (*PurchaseResponse, error) {
	req := PurchaseRequest{UserID: userID, ProductID: productID, Qty: qty}
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
func (c *PurchaseClient) RestoreStock(productID, qty int) error {
	req := map[string]int{"product_id": productID, "qty": qty}
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