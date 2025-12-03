package config

import "os"

// Config holds API Gateway configuration
type Config struct {
	Port               string
	JWTSecret          string
	PurchaseServiceURL string
	RateLimitRequests  int
	RateLimitWindow    int
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "8080"),
		JWTSecret:          getEnv("JWT_SECRET", "your-super-secret-key-change-in-production"),
		PurchaseServiceURL: getEnv("PURCHASE_SERVICE_URL", "http://localhost:8081"),
		RateLimitRequests:  100,
		RateLimitWindow:    60,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
