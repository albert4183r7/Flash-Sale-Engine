package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type OrderServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewOrderServiceClient(baseURL string) *OrderServiceClient {
	return &OrderServiceClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type CreateOrderRequest struct {
	OrderID      uuid.UUID `json:"order_id"`
	UserID       uuid.UUID `json:"user_id"`
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	ProductPrice int       `json:"product_price"`
	Qty          int       `json:"qty"`
	Notes        string    `json:"notes,omitempty"`
}

type CreateOrderResponse struct {
	Success bool      `json:"success"`
	OrderID uuid.UUID `json:"order_id"`
	Error   string    `json:"error,omitempty"`
}

func (c *OrderServiceClient) CreateOrder(orderID, userID, productID uuid.UUID, productName string, productPrice, qty int, notes string) (*CreateOrderResponse, error) {
	req := CreateOrderRequest{
		OrderID:      orderID,
		UserID:       userID,
		ProductID:    productID,
		ProductName:  productName,
		ProductPrice: productPrice,
		Qty:          qty,
		Notes:        notes,
	}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/internal/orders", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CreateOrderResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

func (c *OrderServiceClient) UpdateOrderStatus(orderID uuid.UUID, status string) error {
	req := UpdateStatusRequest{Status: status}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/internal/orders/%s/status", c.baseURL, orderID), bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update order status, status code: %d", resp.StatusCode)
	}
	return nil
}
