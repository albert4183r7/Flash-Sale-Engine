package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flashsale/purchase-service/publisher"
	"github.com/flashsale/purchase-service/redis"
	"github.com/google/uuid"
)

// PurchaseResult represents the result of a purchase attempt
type PurchaseResult struct {
	Success bool
	OrderID uuid.UUID
	Message string
	Error   string
}

// PurchaseService handles purchase logic
type PurchaseService struct {
	redisClient    *redis.Client
	rabbitPublisher *publisher.RabbitMQ
	idempotencyTTL int
}

// NewPurchaseService creates a new PurchaseService
func NewPurchaseService(redisClient *redis.Client, rabbitPublisher *publisher.RabbitMQ, idempotencyTTL int) *PurchaseService {
	return &PurchaseService{
		redisClient:    redisClient,
		rabbitPublisher: rabbitPublisher,
		idempotencyTTL: idempotencyTTL,
	}
}

// ProcessPurchase handles the purchase logic
func (s *PurchaseService) ProcessPurchase(ctx context.Context, userID, productID, qty int) PurchaseResult {
	isFirstRequest, err := s.redisClient.SetIdempotencyKey(ctx, userID, productID, s.idempotencyTTL)
	if err != nil {
		return PurchaseResult{
			Success: false,
			Message: "Failed to check idempotency",
			Error:   err.Error(),
		}
	}

	if !isFirstRequest {
		return PurchaseResult{
			Success: false,
			Message: "Duplicate purchase request",
			Error:   fmt.Sprintf("You already have a pending purchase for product %d", productID),
		}
	}

	newStock, err := s.redisClient.DecrementStock(ctx, productID, qty)
	if err != nil {
		return PurchaseResult{
			Success: false,
			Message: "Failed to check stock",
			Error:   err.Error(),
		}
	}

	if newStock == -2 {
		return PurchaseResult{
			Success: false,
			Message: "Product not found",
			Error:   fmt.Sprintf("Product %d does not exist or has no stock initialized", productID),
		}
	}

	if newStock < 0 {
		return PurchaseResult{
			Success: false,
			Message: "Out of stock",
			Error:   fmt.Sprintf("Product %d is out of stock", productID),
		}
	}

	orderID := uuid.New()
	event := publisher.OrderEvent{
		OrderID:   orderID,
		UserID:    userID,
		ProductID: productID,
		Qty:       qty,
		Timestamp: time.Now(),
	}

	if err := s.rabbitPublisher.PublishOrderCreated(event); err != nil {
		return PurchaseResult{
			Success: false,
			Message: "Failed to process order",
			Error:   "Order could not be queued for processing",
		}
	}

	return PurchaseResult{
		Success: true,
		OrderID: orderID,
		Message: "Purchase accepted and queued for processing",
	}
}

// InitializeProducts sets up initial stock for products
func (s *PurchaseService) InitializeProducts(ctx context.Context) error {
	products := map[int]int{
		1: 100,
		2: 50,
		3: 200,
		4: 75,
		5: 150,
	}

	for productID, stock := range products {
		if err := s.redisClient.InitializeStock(ctx, productID, stock); err != nil {
			return fmt.Errorf("failed to initialize stock for product %d: %w", productID, err)
		}
	}

	return nil
}
