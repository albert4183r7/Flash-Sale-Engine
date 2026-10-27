// Package router wires the purchase service's HTTP routes.
package router

import (
	"github.com/flashsale/purchase-service/config"
	"github.com/flashsale/purchase-service/handler"
	"github.com/flashsale/purchase-service/middleware"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
)

// Setup builds the purchase service's HTTP engine.
func Setup(cfg *config.Config, stock *redis.Client, purchases *service.Purchase) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	health := handler.NewHealthHandler(func(c *gin.Context) error {
		return stock.Ping(c.Request.Context())
	})
	r.GET("/health", health.Health)

	purchaseHandler := handler.NewPurchaseHandler(purchases)
	stockHandler := handler.NewStockHandler(stock)

	// Every route below trusts its caller to have authenticated the buyer, so
	// all of them sit behind the shared internal token.
	internal := r.Group("/", middleware.InternalAuth(cfg.InternalToken))
	{
		internal.POST("/purchase", purchaseHandler.Purchase)
		internal.GET("/stock/:id", stockHandler.GetStock)
		internal.POST("/internal/stock/restore", stockHandler.RestoreStock)
	}

	return r
}
