package router

import (
	"database/sql"

	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/handler"
	"github.com/flashsale/api-gateway/middleware"
	"github.com/gin-gonic/gin"
)

func Setup(cfg *config.Config, db *sql.DB) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	r.Use(rateLimiter.RateLimit())

	healthHandler := handler.NewHealthHandler()
	r.GET("/health", healthHandler.Health)

	// Inject DB into AuthHandler
	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	auth := r.Group("/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
	}

	purchaseClient := client.NewPurchaseClient(cfg.PurchaseServiceURL)
	purchaseHandler := handler.NewPurchaseHandler(purchaseClient)
	orderHandler := handler.NewOrderHandler(db, purchaseClient) // Inject DB

	protected := r.Group("/")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	{
		protected.POST("/purchase", purchaseHandler.Purchase)
		protected.GET("/orders/:id", orderHandler.GetOrder)
		protected.DELETE("/orders/:id", orderHandler.CancelOrder)
	}

	return r
}