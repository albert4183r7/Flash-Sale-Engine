package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type PaymentServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPaymentServiceClient(baseURL string) *PaymentServiceClient {
	return &PaymentServiceClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second}, // Longer timeout for payment processing
	}
}

type ProcessPaymentRequest struct {
	OrderID uuid.UUID `json:"order_id"`
	Amount  int       `json:"amount"`
	Method  string    `json:"method"`
}

type ProcessPaymentResponse struct {
	Success   bool      `json:"success"`
	PaymentID uuid.UUID `json:"payment_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
}

func (c *PaymentServiceClient) ProcessPayment(orderID uuid.UUID, amount int, method string) (*ProcessPaymentResponse, error) {
	req := ProcessPaymentRequest{
		OrderID: orderID,
		Amount:  amount,
		Method:  method,
	}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/internal/process", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ProcessPaymentResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}
