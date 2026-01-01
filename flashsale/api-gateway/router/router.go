package router

import (
	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/handler"
	"github.com/flashsale/api-gateway/middleware"
	"github.com/gin-gonic/gin"
)

func Setup(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	r.Use(rateLimiter.RateLimit())

	// Health check (outside versioning for monitoring tools)
	healthHandler := handler.NewHealthHandler()
	r.GET("/health", healthHandler.Health)

	// Initialize service clients
	userClient := client.NewUserClient(cfg.UserServiceURL)
	productClient := client.NewProductClient(cfg.ProductServiceURL)
	orderClient := client.NewOrderClient(cfg.OrderServiceURL)
	purchaseClient := client.NewPurchaseClient(cfg.PurchaseServiceURL)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(userClient)
	productHandler := handler.NewProductHandler(productClient)
	purchaseHandler := handler.NewPurchaseHandler(purchaseClient)
	orderHandler := handler.NewOrderHandler(orderClient, productClient)

	// ===========================================
	// API v1 Routes
	// ===========================================
	v1 := r.Group("/api/v1")

	// Auth routes (public)
	auth := v1.Group("/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
	}

	// Product routes (public)
	products := v1.Group("/products")
	{
		products.GET("", productHandler.ListProducts)
		products.GET("/:id", productHandler.GetProduct)
		products.GET("/:id/stock", productHandler.GetStock)
	}

	// User routes (protected - requires JWT)
	users := v1.Group("/users")
	users.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		// Orders under user resource (RESTful pattern)
		users.POST("/:user_id/orders", purchaseHandler.CreateOrder)
		users.GET("/:user_id/orders", orderHandler.GetUserOrders)
		users.GET("/:user_id/orders/:order_id", orderHandler.GetOrder)
		users.DELETE("/:user_id/orders/:order_id", orderHandler.CancelOrder)
	}

	return r
}
