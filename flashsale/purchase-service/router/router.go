// Package router wires the purchase service's HTTP routes.
package router

import (
	"context"

	"github.com/flashsale/purchase-service/config"
	"github.com/flashsale/purchase-service/handler"
	"github.com/flashsale/purchase-service/middleware"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/service"
	"github.com/flashsale/purchase-service/warmup"
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
	orderHandler := handler.NewOrderHandler(stock)
	stockHandler := handler.NewStockHandler(stock, func(ctx context.Context) (warmup.Result, error) {
		return warmup.LoadStock(ctx, cfg.PostgresURL, stock)
	})

	// Every route below trusts its caller to have authenticated the buyer, so
	// all of them sit behind the shared internal token.
	internal := r.Group("/", middleware.InternalAuth(cfg.InternalToken))
	{
		internal.POST("/purchase", purchaseHandler.Purchase)
		internal.GET("/stock/:id", stockHandler.GetStock)
		internal.POST("/internal/stock/restore", stockHandler.RestoreStock)
		internal.POST("/internal/stock/load", stockHandler.LoadStock)
		internal.GET("/internal/orders/:id/pending", orderHandler.GetPending)
	}

	return r
}
