package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flashsale/purchase-service/client"
	"github.com/flashsale/purchase-service/publisher"
	"github.com/flashsale/purchase-service/redis"
	"github.com/google/uuid"
)

// PurchaseResult represents the result of a purchase attempt
type PurchaseResult struct {
	Success     bool
	OrderID     uuid.UUID
	ProductID   uuid.UUID
	ProductName string
	Qty         int
	Message     string    // User-friendly message for display
	ErrorCode   string    // Technical error code for debugging
}

// PurchaseService handles purchase logic
type PurchaseService struct {
	redisClient     *redis.Client
	rabbitPublisher *publisher.RabbitMQ
	productClient   *client.ProductClient
	idempotencyTTL  int
}

// NewPurchaseService creates a new PurchaseService
func NewPurchaseService(redisClient *redis.Client, rabbitPublisher *publisher.RabbitMQ, productClient *client.ProductClient, idempotencyTTL int) *PurchaseService {
	return &PurchaseService{
		redisClient:     redisClient,
		rabbitPublisher: rabbitPublisher,
		productClient:   productClient,
		idempotencyTTL:  idempotencyTTL,
	}
}

// ProcessPurchase handles the purchase logic
func (s *PurchaseService) ProcessPurchase(ctx context.Context, userID, productID uuid.UUID, qty int, notes, paymentMethod string) PurchaseResult {
	// Fetch product info first for better error messages
	product, err := s.productClient.GetProduct(productID)
	productName := "this product"
	if err == nil && product != nil {
		productName = product.Name
	}

	// Check idempotency
	isFirstRequest, err := s.redisClient.SetIdempotencyKey(ctx, userID, productID, s.idempotencyTTL)
	if err != nil {
		return PurchaseResult{
			Success:   false,
			ProductID: productID,
			Message:   "We couldn't process your request. Please try again.",
			ErrorCode: "IDEMPOTENCY_CHECK_FAILED",
		}
	}

	if !isFirstRequest {
		return PurchaseResult{
			Success:     false,
			ProductID:   productID,
			ProductName: productName,
			Message:     fmt.Sprintf("You already have a pending order for %s. Please wait for it to complete.", productName),
			ErrorCode:   "DUPLICATE_PURCHASE",
		}
	}

	// Check and decrement stock
	newStock, err := s.redisClient.DecrementStock(ctx, productID, qty)
	if err != nil {
		return PurchaseResult{
			Success:   false,
			ProductID: productID,
			Message:   "Unable to check stock availability. Please try again.",
			ErrorCode: "STOCK_CHECK_FAILED",
		}
	}

	if newStock == -2 {
		return PurchaseResult{
			Success:     false,
			ProductID:   productID,
			ProductName: productName,
			Message:     fmt.Sprintf("%s is not available for purchase.", productName),
			ErrorCode:   "PRODUCT_NOT_FOUND",
		}
	}

	if newStock < 0 {
		return PurchaseResult{
			Success:     false,
			ProductID:   productID,
			ProductName: productName,
			Message:     fmt.Sprintf("Sorry, %s is currently out of stock.", productName),
			ErrorCode:   "OUT_OF_STOCK",
		}
	}

	// Publish order event
	orderID := uuid.New()
	event := publisher.OrderEvent{
		OrderID:       orderID,
		UserID:        userID,
		ProductID:     productID,
		Qty:           qty,
		Notes:         notes,
		PaymentMethod: paymentMethod,
		Timestamp:     time.Now(),
	}

	if err := s.rabbitPublisher.PublishOrderCreated(event); err != nil {
		return PurchaseResult{
			Success:   false,
			ProductID: productID,
			Message:   "Your order couldn't be processed. Please try again.",
			ErrorCode: "ORDER_QUEUE_FAILED",
		}
	}

	return PurchaseResult{
		Success:     true,
		OrderID:     orderID,
		ProductID:   productID,
		ProductName: productName,
		Qty:         qty,
		Message:     fmt.Sprintf("Your order for %s has been placed and is being processed.", productName),
	}
}
