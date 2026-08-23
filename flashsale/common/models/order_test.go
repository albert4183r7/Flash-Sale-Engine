package models_test

import (
	"testing"
	"time"

	"github.com/flashsale/common/models"
	"github.com/google/uuid"
)

func TestOrderStatusValid(t *testing.T) {
	tests := []struct {
		status models.OrderStatus
		want   bool
	}{
		{models.OrderStatusPending, true},
		{models.OrderStatusSuccess, true},
		{models.OrderStatusFailed, true},
		{models.OrderStatusCancelled, true},
		{models.OrderStatus("SHIPPED"), false},
		{models.OrderStatus("pending"), false}, // statuses are stored upper case
		{models.OrderStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Cancelling an order returns stock, so only the states that are actually
// holding stock may be cancelled. Getting this wrong would credit stock for an
// order that never consumed any.
func TestOrderStatusConsumedStock(t *testing.T) {
	tests := []struct {
		status models.OrderStatus
		want   bool
	}{
		{models.OrderStatusPending, true},
		{models.OrderStatusSuccess, true},
		{models.OrderStatusFailed, false},
		{models.OrderStatusCancelled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.ConsumedStock(); got != tt.want {
				t.Errorf("ConsumedStock() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderEventValidate(t *testing.T) {
	valid := models.OrderEvent{
		OrderID:   uuid.New(),
		UserID:    uuid.New(),
		ProductID: uuid.New(),
		Qty:       1,
		Timestamp: time.Now(),
	}

	tests := []struct {
		name    string
		mutate  func(*models.OrderEvent)
		wantErr bool
	}{
		{"valid", func(*models.OrderEvent) {}, false},
		{"missing order id", func(e *models.OrderEvent) { e.OrderID = uuid.Nil }, true},
		{"missing user id", func(e *models.OrderEvent) { e.UserID = uuid.Nil }, true},
		{"missing product id", func(e *models.OrderEvent) { e.ProductID = uuid.Nil }, true},
		{"zero qty", func(e *models.OrderEvent) { e.Qty = 0 }, true},
		{"negative qty", func(e *models.OrderEvent) { e.Qty = -3 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.mutate(&event)

			err := event.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}
