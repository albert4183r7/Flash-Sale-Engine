package config

import (
	"log"
	"os"
)

type Config struct {
	Port               string
	JWTSecret          string
	PurchaseServiceURL string
	UserServiceURL     string
	ProductServiceURL  string
	OrderServiceURL    string
	RateLimitRequests  int
	RateLimitWindow    int
}

func Load() *Config {
	return &Config{
		Port:               getEnv("API_GATEWAY_PORT", "8080"),
		JWTSecret:          getEnv("JWT_SECRET", "your-secret-key"),
		PurchaseServiceURL: getEnv("PURCHASE_SERVICE_URL", "http://localhost:8081"),
		UserServiceURL:     getEnv("USER_SERVICE_URL", "http://localhost:8082"),
		ProductServiceURL:  getEnv("PRODUCT_SERVICE_URL", "http://localhost:8083"),
		OrderServiceURL:    getEnv("ORDER_SERVICE_URL", "http://localhost:8084"),
		RateLimitRequests:  120,
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