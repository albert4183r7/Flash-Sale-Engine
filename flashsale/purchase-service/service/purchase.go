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
func (s *PurchaseService) ProcessPurchase(ctx context.Context, userID, productID uuid.UUID, qty int) PurchaseResult {
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
			Error:   fmt.Sprintf("You already have a pending purchase for product %s", productID.String()),
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
			Error:   fmt.Sprintf("Product %s does not exist or has no stock initialized", productID.String()),
		}
	}

	if newStock < 0 {
		return PurchaseResult{
			Success: false,
			Message: "Out of stock",
			Error:   fmt.Sprintf("Product %s is out of stock", productID.String()),
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
	products := map[string]int{
		"11111111-1111-1111-1111-111111111111": 100, // iPhone 15 Pro
		"22222222-2222-2222-2222-222222222222": 50,  // MacBook Air M3
		"33333333-3333-3333-3333-333333333333": 200, // AirPods Pro
		"44444444-4444-4444-4444-444444444444": 75,  // iPad Pro
		"55555555-5555-5555-5555-555555555555": 150, // Apple Watch
	}

	for idStr, stock := range products {
		uid, err := uuid.Parse(idStr)
		if err != nil {
			return fmt.Errorf("failed to parse product UUID %s: %w", idStr, err)
		}

		if err := s.redisClient.InitializeStock(ctx, uid, stock); err != nil {
			return fmt.Errorf("failed to initialize stock for product %s: %w", idStr, err)
		}
	}

	return nil
}
