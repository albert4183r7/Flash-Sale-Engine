package config

import (
	"log"
	"os"
)

type Config struct {
	Port               	string
	JWTSecret          	string
	PurchaseServiceURL 	string
	PostgresURL        	string
	RedisAddr			string
	RedisPassword		string
	RateLimitRequests  	int
	RateLimitWindow    	int
}

func Load() *Config {
	return &Config{
		Port:               getEnv("API_GATEWAY_PORT", ""),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		PurchaseServiceURL: getEnv("PURCHASE_SERVICE_URL", ""),
		PostgresURL:        getEnv("POSTGRES_URL", ""),
		RedisAddr:          getEnv("REDIS_ADDR", ""),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
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