package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/flashsale/order-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type OrderHandler struct {
	orderRepo *repository.OrderRepository
}

func NewOrderHandler(orderRepo *repository.OrderRepository) *OrderHandler {
	return &OrderHandler{orderRepo: orderRepo}
}

// GetOrder returns an order by ID
func (h *OrderHandler) GetOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	order, err := h.orderRepo.FindByID(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, order)
}

// GetOrdersByUser returns all orders for a user
func (h *OrderHandler) GetOrdersByUser(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	orders, err := h.orderRepo.FindByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"orders": orders})
}

type CreateOrderRequest struct {
	OrderID      uuid.UUID `json:"order_id" binding:"required"`
	UserID       uuid.UUID `json:"user_id" binding:"required"`
	ProductID    uuid.UUID `json:"product_id" binding:"required"`
	ProductName  string    `json:"product_name" binding:"required"`
	ProductPrice int       `json:"product_price" binding:"required"`
	Qty          int       `json:"qty" binding:"required,min=1"`
	Notes        string    `json:"notes"`
}

// CreateOrder creates a new order (called by order-worker)
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order := &repository.Order{
		ID:           req.OrderID,
		UserID:       req.UserID,
		ProductID:    req.ProductID,
		ProductName:  req.ProductName,
		ProductPrice: req.ProductPrice,
		Qty:          req.Qty,
		Notes:        req.Notes,
		Status:       repository.OrderStatusPending,
		CreatedAt:    time.Now(),
	}

	if err := h.orderRepo.Create(order); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":  true,
		"order_id": order.ID,
	})
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateOrderStatus updates the status of an order
func (h *OrderHandler) UpdateOrderStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := repository.OrderStatus(req.Status)
	if err := h.orderRepo.UpdateStatus(id, status); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// CancelOrder cancels an order and returns order details for stock restoration
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	order, err := h.orderRepo.Cancel(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel order"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"order_id":     order.ID,
		"product_id":   order.ProductID,
		"product_name": order.ProductName,
		"qty":          order.Qty,
		"message":      "Order cancelled",
	})
}
