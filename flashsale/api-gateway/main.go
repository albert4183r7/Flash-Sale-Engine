package main

import (
	"context"
	"database/sql"
	"log"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Load Environment Variables
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	log.Println("Starting API Gateway...")
	cfg := config.Load()

	// 2. Connect to Database
	db, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping DB: %v", err)
	}

	// 3. Connect to Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0, // Use default DB
	})
	defer redisClient.Close()
	
	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
	} else {
		log.Println("Connected to Redis successfully")
	}

	// 4. Setup Router (Pass Redis Client)
	r := router.Setup(cfg, db, redisClient)

	log.Printf("API Gateway listening on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}