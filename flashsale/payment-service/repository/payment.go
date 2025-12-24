package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "PENDING"
	PaymentStatusSuccess PaymentStatus = "SUCCESS"
	PaymentStatusFailed  PaymentStatus = "FAILED"
)

type Payment struct {
	ID        uuid.UUID     `json:"id"`
	OrderID   uuid.UUID     `json:"order_id"`
	Amount    int           `json:"amount"`
	Method    string        `json:"method"`
	Status    PaymentStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
}

type PaymentRepository struct {
	db *sql.DB
}

func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(payment *Payment) error {
	_, err := r.db.Exec(
		`INSERT INTO payments (id, order_id, amount, method, status, created_at) 
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		payment.ID, payment.OrderID, payment.Amount, payment.Method, payment.Status, payment.CreatedAt,
	)
	return err
}

func (r *PaymentRepository) FindByID(id uuid.UUID) (*Payment, error) {
	var payment Payment
	err := r.db.QueryRow(
		`SELECT id, order_id, amount, method, status, created_at FROM payments WHERE id = $1`,
		id,
	).Scan(&payment.ID, &payment.OrderID, &payment.Amount, &payment.Method, &payment.Status, &payment.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *PaymentRepository) FindByOrderID(orderID uuid.UUID) (*Payment, error) {
	var payment Payment
	err := r.db.QueryRow(
		`SELECT id, order_id, amount, method, status, created_at FROM payments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`,
		orderID,
	).Scan(&payment.ID, &payment.OrderID, &payment.Amount, &payment.Method, &payment.Status, &payment.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *PaymentRepository) UpdateStatus(id uuid.UUID, status PaymentStatus) error {
	_, err := r.db.Exec(
		`UPDATE payments SET status = $1 WHERE id = $2`,
		status, id,
	)
	return err
}
