package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/api-gateway/service"
	"github.com/flashsale/common/models"
	"github.com/google/uuid"
)

// fakeOrders is an in-memory OrderStore that enforces the same ownership rule
// as the real repository.
type fakeOrders struct {
	orders map[uuid.UUID]models.Order
	err    error
}

func newFakeOrders() *fakeOrders {
	return &fakeOrders{orders: map[uuid.UUID]models.Order{}}
}

func (f *fakeOrders) add(userID uuid.UUID, status models.OrderStatus, qty int) models.Order {
	o := models.Order{
		ID:        uuid.New(),
		UserID:    userID,
		ProductID: uuid.New(),
		Qty:       qty,
		Status:    status,
		CreatedAt: time.Now(),
	}
	f.orders[o.ID] = o
	return o
}

func (f *fakeOrders) FindByIDForUser(_ context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	if f.err != nil {
		return models.Order{}, f.err
	}
	o, ok := f.orders[orderID]
	if !ok || o.UserID != userID {
		return models.Order{}, repository.ErrOrderNotFound
	}
	return o, nil
}

func (f *fakeOrders) CancelForUser(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	o, err := f.FindByIDForUser(ctx, orderID, userID)
	if err != nil {
		return models.Order{}, err
	}
	if !o.Status.ConsumedStock() {
		return models.Order{}, repository.ErrOrderNotCancellable
	}
	o.Status = models.OrderStatusCancelled
	f.orders[orderID] = o
	return o, nil
}

// fakeStock records the units handed back to the in-memory counter.
type fakeStock struct {
	restored map[uuid.UUID]int
	err      error
}

func newFakeStock() *fakeStock { return &fakeStock{restored: map[uuid.UUID]int{}} }

func (f *fakeStock) RestoreStock(_ context.Context, productID uuid.UUID, qty int) error {
	if f.err != nil {
		return f.err
	}
	f.restored[productID] += qty
	return nil
}

func TestGetOrderReturnsOwnOrder(t *testing.T) {
	owner := uuid.New()
	orders := newFakeOrders()
	want := orders.add(owner, models.OrderStatusSuccess, 2)
	svc := service.NewOrder(orders, newFakeStock())

	got, err := svc.Get(context.Background(), want.ID, owner)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("order id = %s, want %s", got.ID, want.ID)
	}
}

// Reading somebody else's order must be indistinguishable from the order not
// existing, so the API cannot be used to probe for other buyers' orders.
func TestGetOrderHidesOtherUsersOrders(t *testing.T) {
	owner, stranger := uuid.New(), uuid.New()
	orders := newFakeOrders()
	order := orders.add(owner, models.OrderStatusSuccess, 2)
	svc := service.NewOrder(orders, newFakeStock())

	_, err := svc.Get(context.Background(), order.ID, stranger)
	if !errors.Is(err, service.ErrOrderNotFound) {
		t.Fatalf("Get() error = %v, want ErrOrderNotFound", err)
	}
}

func TestCancelOrderRestoresStock(t *testing.T) {
	owner := uuid.New()
	orders := newFakeOrders()
	order := orders.add(owner, models.OrderStatusSuccess, 3)
	stock := newFakeStock()
	svc := service.NewOrder(orders, stock)

	cancelled, err := svc.Cancel(context.Background(), order.ID, owner)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != models.OrderStatusCancelled {
		t.Errorf("status = %s, want %s", cancelled.Status, models.OrderStatusCancelled)
	}
	if got := stock.restored[order.ProductID]; got != 3 {
		t.Errorf("restored %d units, want 3", got)
	}
}

// Cancelling another buyer's order is the vulnerability this guards: it must be
// refused, and it must not return any stock.
func TestCancelOrderRefusesOtherUsersOrders(t *testing.T) {
	owner, attacker := uuid.New(), uuid.New()
	orders := newFakeOrders()
	order := orders.add(owner, models.OrderStatusSuccess, 3)
	stock := newFakeStock()
	svc := service.NewOrder(orders, stock)

	_, err := svc.Cancel(context.Background(), order.ID, attacker)
	if !errors.Is(err, service.ErrOrderNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrOrderNotFound", err)
	}
	if len(stock.restored) != 0 {
		t.Error("stock was restored for a cancellation that should have been refused")
	}
	if orders.orders[order.ID].Status != models.OrderStatusSuccess {
		t.Error("order status was changed by a caller who does not own it")
	}
}

// Cancelling twice must not return the stock twice, which would let the sale
// oversell.
func TestCancelOrderIsNotRepeatable(t *testing.T) {
	owner := uuid.New()
	orders := newFakeOrders()
	order := orders.add(owner, models.OrderStatusSuccess, 4)
	stock := newFakeStock()
	svc := service.NewOrder(orders, stock)

	if _, err := svc.Cancel(context.Background(), order.ID, owner); err != nil {
		t.Fatalf("first Cancel() error = %v", err)
	}

	_, err := svc.Cancel(context.Background(), order.ID, owner)
	if !errors.Is(err, service.ErrOrderNotCancellable) {
		t.Fatalf("second Cancel() error = %v, want ErrOrderNotCancellable", err)
	}
	if got := stock.restored[order.ProductID]; got != 4 {
		t.Fatalf("restored %d units after two cancellations, want 4", got)
	}
}

// The durable cancellation has already committed by the time the counter is
// credited, so a Redis failure must not report the cancellation as failed.
// Under-counting stock is the safe direction: it under-sells rather than
// overselling.
func TestCancelOrderSucceedsWhenStockCacheFails(t *testing.T) {
	owner := uuid.New()
	orders := newFakeOrders()
	order := orders.add(owner, models.OrderStatusSuccess, 2)
	stock := newFakeStock()
	stock.err = errors.New("connection refused")
	svc := service.NewOrder(orders, stock)

	cancelled, err := svc.Cancel(context.Background(), order.ID, owner)
	if err != nil {
		t.Fatalf("Cancel() error = %v, want success despite the stock cache failing", err)
	}
	if cancelled.Status != models.OrderStatusCancelled {
		t.Errorf("status = %s, want %s", cancelled.Status, models.OrderStatusCancelled)
	}
}
