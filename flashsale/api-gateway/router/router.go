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

	// Health check
	healthHandler := handler.NewHealthHandler()
	r.GET("/health", healthHandler.Health)

	// Initialize service clients
	userClient := client.NewUserClient(cfg.UserServiceURL)
	productClient := client.NewProductClient(cfg.ProductServiceURL)
	orderClient := client.NewOrderClient(cfg.OrderServiceURL)
	purchaseClient := client.NewPurchaseClient(cfg.PurchaseServiceURL)

	// Auth routes (proxied to user-service)
	authHandler := handler.NewAuthHandler(userClient)
	auth := r.Group("/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
	}

	// Public product routes (proxied to product-service)
	productHandler := handler.NewProductHandler(productClient)
	r.GET("/products", productHandler.ListProducts)
	r.GET("/products/:id", productHandler.GetProduct)
	r.GET("/products/:id/stock", productHandler.GetStock)

	// Protected routes
	purchaseHandler := handler.NewPurchaseHandler(purchaseClient)
	orderHandler := handler.NewOrderHandler(orderClient, productClient)

	protected := r.Group("/")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		protected.POST("/purchase", purchaseHandler.Purchase)
		protected.GET("/orders/:id", orderHandler.GetOrder)
		protected.GET("/my-orders", orderHandler.GetMyOrders)
		protected.DELETE("/orders/:id", orderHandler.CancelOrder)
	}

	return r
}