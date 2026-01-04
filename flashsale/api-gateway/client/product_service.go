package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ProductClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewProductClient(baseURL string) *ProductClient {
	return &ProductClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type Product struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	Stock       int       `json:"stock"`
}

type ProductsResponse struct {
	Products []Product `json:"products"`
}

func (c *ProductClient) ListProducts() (*ProductsResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/products")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ProductsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

func (c *ProductClient) GetProduct(id uuid.UUID) (*Product, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/products/%s", c.baseURL, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Product
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

type StockResponse struct {
	ProductID uuid.UUID `json:"product_id"`
	Stock     int       `json:"stock"`
}

func (c *ProductClient) GetStock(productID uuid.UUID) (*StockResponse, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/products/%s/stock", c.baseURL, productID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result StockResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

type StockRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
}

type StockModifyResponse struct {
	Success  bool   `json:"success"`
	NewStock int    `json:"new_stock,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (c *ProductClient) RestoreStock(productID uuid.UUID, qty int) error {
	req := StockRequest{ProductID: productID, Qty: qty}
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
