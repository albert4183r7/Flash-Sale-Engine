package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ProductClient calls product-service for product information
type ProductClient struct {
	baseURL    string
	httpClient *http.Client
}

// Product represents a product from product-service
type Product struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	Stock       int       `json:"stock"`
}

// NewProductClient creates a new ProductClient
func NewProductClient(baseURL string) *ProductClient {
	return &ProductClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetProduct fetches a product by ID
func (c *ProductClient) GetProduct(productID uuid.UUID) (*Product, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/products/%s", c.baseURL, productID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product not found")
	}

	var product Product
	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		return nil, err
	}
	return &product, nil
}
