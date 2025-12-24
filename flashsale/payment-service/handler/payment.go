package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/flashsale/payment-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PaymentHandler struct {
	paymentRepo *repository.PaymentRepository
}

func NewPaymentHandler(paymentRepo *repository.PaymentRepository) *PaymentHandler {
	return &PaymentHandler{paymentRepo: paymentRepo}
}

type ProcessPaymentRequest struct {
	OrderID uuid.UUID `json:"order_id" binding:"required"`
	Amount  int       `json:"amount" binding:"required,min=1"`
	Method  string    `json:"method"`
}

type ProcessPaymentResponse struct {
	Success   bool      `json:"success"`
	PaymentID uuid.UUID `json:"payment_id,omitempty"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
}

// ProcessPayment simulates payment processing (internal endpoint)
func (h *PaymentHandler) ProcessPayment(c *gin.Context) {
	var req ProcessPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default to mock_success if no method specified
	if req.Method == "" {
		req.Method = "mock_success"
	}

	// Create payment record
	payment := &repository.Payment{
		ID:        uuid.New(),
		OrderID:   req.OrderID,
		Amount:    req.Amount,
		Method:    req.Method,
		Status:    repository.PaymentStatusPending,
		CreatedAt: time.Now(),
	}

	if err := h.paymentRepo.Create(payment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
		return
	}

	// Simulate payment processing based on method
	var finalStatus repository.PaymentStatus
	var message string

	switch req.Method {
	case "mock_success":
		time.Sleep(1 * time.Second) // Simulate processing delay
		finalStatus = repository.PaymentStatusSuccess
		message = "Payment successful"
	case "mock_fail":
		time.Sleep(1 * time.Second)
		finalStatus = repository.PaymentStatusFailed
		message = "Payment failed - card declined"
	case "mock_pending":
		finalStatus = repository.PaymentStatusPending
		message = "Payment pending - awaiting confirmation"
	default:
		time.Sleep(500 * time.Millisecond)
		finalStatus = repository.PaymentStatusSuccess
		message = "Payment successful"
	}

	// Update payment status
	h.paymentRepo.UpdateStatus(payment.ID, finalStatus)

	c.JSON(http.StatusOK, ProcessPaymentResponse{
		Success:   finalStatus == repository.PaymentStatusSuccess,
		PaymentID: payment.ID,
		Status:    string(finalStatus),
		Message:   message,
	})
}

// GetPayment returns a payment by ID
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment ID"})
		return
	}

	payment, err := h.paymentRepo.FindByID(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, payment)
}

// GetPaymentByOrder returns payment for an order
func (h *PaymentHandler) GetPaymentByOrder(c *gin.Context) {
	orderIDStr := c.Param("order_id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	payment, err := h.paymentRepo.FindByOrderID(orderID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found for this order"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, payment)
}
