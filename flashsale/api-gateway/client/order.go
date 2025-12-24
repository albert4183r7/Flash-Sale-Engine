package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type OrderClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewOrderClient(baseURL string) *OrderClient {
	return &OrderClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type Order struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type OrdersResponse struct {
	Orders []Order `json:"orders"`
}

func (c *OrderClient) GetOrder(orderID uuid.UUID) (*Order, int, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/orders/%s", c.baseURL, orderID))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var result Order
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, resp.StatusCode, nil
}

func (c *OrderClient) GetOrdersByUser(userID uuid.UUID) (*OrdersResponse, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/users/%s/orders", c.baseURL, userID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result OrdersResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

type CancelOrderResponse struct {
	Success     bool      `json:"success"`
	OrderID     uuid.UUID `json:"order_id"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Qty         int       `json:"qty"`
	Message     string    `json:"message"`
	Error       string    `json:"error,omitempty"`
}

func (c *OrderClient) CancelOrder(orderID uuid.UUID) (*CancelOrderResponse, int, error) {
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/internal/orders/%s", c.baseURL, orderID), nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var result CancelOrderResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, resp.StatusCode, nil
}
