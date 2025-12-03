package router

import (
	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/handler"
	"github.com/flashsale/api-gateway/middleware"
	"github.com/gin-gonic/gin"
)

// Setup creates and configures the Gin router
func Setup(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(middleware.Logger())

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	r.Use(rateLimiter.RateLimit())

	healthHandler := handler.NewHealthHandler()
	r.GET("/health", healthHandler.Health)

	authHandler := handler.NewAuthHandler(cfg.JWTSecret)
	auth := r.Group("/auth")
	{
		auth.POST("/login", authHandler.Login)
	}

	purchaseClient := client.NewPurchaseClient(cfg.PurchaseServiceURL)
	purchaseHandler := handler.NewPurchaseHandler(purchaseClient)
	orderHandler := handler.NewOrderHandler()

	protected := r.Group("/")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		protected.POST("/purchase", purchaseHandler.Purchase)
		protected.GET("/orders/:id", orderHandler.GetOrder)
	}

	return r
}
