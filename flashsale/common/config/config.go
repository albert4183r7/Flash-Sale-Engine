package config

import (
	"os"
)

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
		RabbitMQURL:        getEnv("RABBITMQ_URL", ""), // No default in prod
		PostgresURL:        getEnv("POSTGRES_URL", ""), // No default in prod
		JWTSecret:          getEnv("JWT_SECRET", ""),   // No default in prod
		PurchaseServiceURL: getEnv("PURCHASE_SERVICE_URL", "http://localhost:8081"),
	}
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		if defaultValue == "" {
			// In a real strict production app, you might want to panic here
			// log.Printf("Warning: Environment variable %s is not set", key)
			return ""
		}
		return defaultValue
	}
	return value
}