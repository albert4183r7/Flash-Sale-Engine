package config

import (
	"log"
	"os"
)

type Config struct {
	Port               string
	JWTSecret          string
	PurchaseServiceURL string
	PostgresURL        string
	RateLimitRequests  int
	RateLimitWindow    int
}

func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "8080"),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		PurchaseServiceURL: getEnv("PURCHASE_SERVICE_URL", "http://localhost:8081"),
		PostgresURL:        getEnv("POSTGRES_URL", ""),
		RateLimitRequests:  100,
		RateLimitWindow:    60,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	if defaultValue == "" {
		log.Printf("Warning: %s not set", key)
	}
	return defaultValue
}