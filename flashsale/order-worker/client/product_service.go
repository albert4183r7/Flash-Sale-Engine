package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ProductServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewProductServiceClient(baseURL string) *ProductServiceClient {
	return &ProductServiceClient{
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

type StockSyncRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
}

type StockSyncResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (c *ProductServiceClient) GetProduct(productID uuid.UUID) (*Product, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/products/%s", c.baseURL, productID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product not found, status: %d", resp.StatusCode)
	}

	var product Product
	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		return nil, err
	}
	return &product, nil
}

func (c *ProductServiceClient) SyncStockToDB(productID uuid.UUID, qty int) error {
	req := StockSyncRequest{ProductID: productID, Qty: qty}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/internal/stock/sync-db", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return err
	}
	return nil
}
