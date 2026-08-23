// Package router wires the API gateway's HTTP routes.
package router

import (
	"database/sql"

	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/handler"
	"github.com/flashsale/api-gateway/middleware"
	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/api-gateway/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Setup builds the gateway's HTTP engine. The returned RateLimiter must be
// stopped when the server shuts down.
func Setup(cfg *config.Config, db *sql.DB, cache *redis.Client) (*gin.Engine, *middleware.RateLimiter) {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery(), middleware.Logger())

	limiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	r.Use(limiter.RateLimit())

	users := repository.NewUserRepository(db)
	orders := repository.NewOrderRepository(db)
	purchaseClient := client.NewPurchase(cfg.PurchaseServiceURL, cfg.InternalToken)

	authService := service.NewAuth(users, cache, cfg.JWTSecret, cfg.TokenTTL, cfg.CredentialCacheTTL)
	orderService := service.NewOrder(orders, stockRestorer{purchaseClient})

	health := handler.NewHealthHandler(func(c *gin.Context) error {
		return db.PingContext(c.Request.Context())
	})
	r.GET("/health", health.Health)

	authHandler := handler.NewAuthHandler(authService)
	auth := r.Group("/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
	}

	purchaseHandler := handler.NewPurchaseHandler(purchaseClient)
	orderHandler := handler.NewOrderHandler(orderService)

	protected := r.Group("/", middleware.JWTAuth(cfg.JWTSecret))
	{
		protected.POST("/purchase", purchaseHandler.Purchase)
		protected.GET("/orders/:id", orderHandler.GetOrder)
		protected.DELETE("/orders/:id", orderHandler.CancelOrder)
	}

	return r, limiter
}

// stockRestorer adapts the purchase service client to the order service's
// narrow StockRestorer dependency.
type stockRestorer struct{ *client.Purchase }
