package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/user-service/cache"
	"github.com/flashsale/user-service/config"
	"github.com/flashsale/user-service/handler"
	"github.com/flashsale/user-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	log.Println("Starting User Service...")
	cfg := config.Load()

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

	userCache := cache.NewUserCache(cfg.RedisAddr, cfg.RedisPassword)
	defer userCache.Close()

	if err := userCache.Ping(context.Background()); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
	} else {
		log.Println("Connected to Redis successfully")
	}

	userRepo := repository.NewUserRepository(db)
	authHandler := handler.NewAuthHandler(userRepo, userCache, cfg.JWTSecret, cfg.TokenExpiry)
	healthHandler := handler.NewHealthHandler()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Health check
	r.GET("/health", healthHandler.Health)

	// Auth routes
	r.POST("/signup", authHandler.Signup)
	r.POST("/login", authHandler.Login)

	// Internal routes (for other services)
	internal := r.Group("/internal")
	{
		internal.GET("/users/:id", authHandler.GetUser)
		internal.POST("/validate-token", authHandler.ValidateToken)
	}

	go func() {
		log.Printf("User Service listening on port %s", cfg.Port)
		if err := r.Run(":" + cfg.Port); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down User Service...")
}
