package handler

import (
	"net/http"

	"github.com/flashsale/api-gateway/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrderHandler proxies order requests to order-service and product-service
type OrderHandler struct {
	orderClient   *client.OrderClient
	productClient *client.ProductClient
}

// NewOrderHandler creates a new OrderHandler
func NewOrderHandler(orderClient *client.OrderClient, productClient *client.ProductClient) *OrderHandler {
	return &OrderHandler{
		orderClient:   orderClient,
		productClient: productClient,
	}
}

// validateUserAccess validates that the JWT user_id matches the URL user_id
func validateUserAccess(c *gin.Context) (uuid.UUID, bool) {
	// Get user_id from URL param
	userIDParam := c.Param("user_id")
	urlUserID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "The user ID provided in the URL is not valid.",
			"error":   "INVALID_USER_ID",
		})
		return uuid.Nil, false
	}

	// Get user_id from JWT context
	jwtUserIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"data":    nil,
			"message": "You need to be logged in to access this resource.",
			"error":   "UNAUTHORIZED",
		})
		return uuid.Nil, false
	}

	jwtUserID, err := uuid.Parse(jwtUserIDStr.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"data":    nil,
			"message": "Invalid user session. Please log in again.",
			"error":   "INVALID_SESSION",
		})
		return uuid.Nil, false
	}

	// Validate that JWT user matches URL user (prevent unauthorized access)
	if urlUserID != jwtUserID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"data":    nil,
			"message": "You do not have permission to access this user's resources.",
			"error":   "FORBIDDEN",
		})
		return uuid.Nil, false
	}

	return urlUserID, true
}

// GetOrder returns a single order by ID
func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID, ok := validateUserAccess(c)
	if !ok {
		return
	}

	orderIDStr := c.Param("order_id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "The order ID provided is not valid.",
			"error":   "INVALID_ORDER_ID",
		})
		return
	}

	order, statusCode, err := h.orderClient.GetOrder(orderID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Unable to retrieve your order. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}

	if statusCode == http.StatusNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": "Order not found. It may have been deleted or the ID is incorrect.",
			"error":   "ORDER_NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"order_id":      order.ID,
			"user_id":       userID,
			"product_id":    order.ProductID,
			"product_name":  order.ProductName,
			"product_price": order.ProductPrice,
			"qty":           order.Qty,
			"notes":         order.Notes,
			"status":        order.Status,
			"created_at":    order.CreatedAt,
		},
		"message": "Order retrieved successfully.",
	})
}

// GetUserOrders returns all orders for the specified user
func (h *OrderHandler) GetUserOrders(c *gin.Context) {
	userID, ok := validateUserAccess(c)
	if !ok {
		return
	}

	orders, err := h.orderClient.GetOrdersByUser(userID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Unable to retrieve your orders. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}

	orderCount := 0
	if orders.Orders != nil {
		orderCount = len(orders.Orders)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    orders,
		"message": getOrdersMessage(orderCount),
	})
}

// CancelOrder cancels an order and restores stock
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	_, ok := validateUserAccess(c)
	if !ok {
		return
	}

	orderIDStr := c.Param("order_id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "The order ID provided is not valid.",
			"error":   "INVALID_ORDER_ID",
		})
		return
	}

	result, statusCode, err := h.orderClient.CancelOrder(orderID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Unable to cancel your order. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}

	if statusCode == http.StatusNotFound {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": "Order not found. It may have already been processed or cancelled.",
			"error":   "ORDER_NOT_FOUND",
		})
		return
	}

	if !result.Success {
		c.JSON(statusCode, gin.H{
			"success": false,
			"data":    nil,
			"message": result.Error,
			"error":   "CANCEL_FAILED",
		})
		return
	}

	// Restore stock in product-service
	err = h.productClient.RestoreStock(result.ProductID, result.Qty)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"order_id":     orderID,
				"product_name": result.ProductName,
			},
			"message": "Your order has been cancelled, but stock restoration is pending.",
			"warning": "STOCK_RESTORE_PENDING",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"order_id":     orderID,
			"product_name": result.ProductName,
		},
		"message": "Your order has been cancelled successfully.",
	})
}

// getOrdersMessage returns a user-friendly message based on order count
func getOrdersMessage(count int) string {
	if count == 0 {
		return "You don't have any orders yet."
	} else if count == 1 {
		return "You have 1 order."
	}
	return "Here are your orders."
}
