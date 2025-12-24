package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/product-service/cache"
	"github.com/flashsale/product-service/config"
	"github.com/flashsale/product-service/handler"
	"github.com/flashsale/product-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	log.Println("Starting Product Service...")
	cfg := config.Load()

	// 1. Connect to PostgreSQL (Products DB)
	db, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Retry loop for DB connection
	for i := 0; i < 5; i++ {
		if err := db.Ping(); err == nil {
			log.Println("Connected to PostgreSQL successfully")
			break
		}
		log.Printf("Waiting for database... (attempt %d/5)", i+1)
		time.Sleep(2 * time.Second)
	}

	// 2. Connect to Redis
	stockCache := cache.NewStockCache(cfg.RedisAddr, cfg.RedisPassword)
	defer stockCache.Close()

	ctx := context.Background()
	if err := stockCache.Ping(ctx); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
	} else {
		log.Println("Connected to Redis successfully")
	}

	// 3. Initialize Repository and Handlers
	productRepo := repository.NewProductRepository(db)
	productHandler := handler.NewProductHandler(productRepo, stockCache)
	healthHandler := handler.NewHealthHandler()

	// 4. Warmup: Load stock from DB to Redis
	products, err := productRepo.FindAll()
	if err != nil {
		log.Printf("Warning: Failed to fetch products for warmup: %v", err)
	} else {
		for _, p := range products {
			if err := stockCache.InitializeStock(ctx, p.ID, p.Stock); err == nil {
				log.Printf("Warmed up Product %s (%s) with Stock %d", p.Name, p.ID, p.Stock)
			}
		}
	}

	// 5. Setup Router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Health check
	r.GET("/health", healthHandler.Health)

	// Public product routes
	r.GET("/products", productHandler.ListProducts)
	r.GET("/products/:id", productHandler.GetProduct)
	r.GET("/products/:id/stock", productHandler.GetStock)

	// Internal routes (for other services)
	internal := r.Group("/internal")
	{
		internal.POST("/stock/decrement", productHandler.DecrementStock)
		internal.POST("/stock/restore", productHandler.RestoreStock)
		internal.POST("/stock/sync-db", productHandler.SyncStockToDB)
		internal.POST("/stock/warmup", productHandler.WarmupCache)
	}

	// 6. Start server
	go func() {
		log.Printf("Product Service listening on port %s", cfg.Port)
		if err := r.Run(":" + cfg.Port); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 7. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Product Service...")
}
