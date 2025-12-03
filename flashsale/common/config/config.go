package config

import "os"

// Config holds all configuration for the services
type Config struct {
	// Server
	Port string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// RabbitMQ
	RabbitMQURL string

	// PostgreSQL
	PostgresURL string

	// JWT
	JWTSecret string

	// Service URLs
	PurchaseServiceURL string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		Port:               getEnv("PORT", "8080"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		RedisDB:            0,
		RabbitMQURL:        getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		PostgresURL:        getEnv("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable"),
		JWTSecret:          getEnv("JWT_SECRET", "your-super-secret-key-change-in-production"),
		PurchaseServiceURL: getEnv("PURCHASE_SERVICE_URL", "http://localhost:8081"),
	}
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
