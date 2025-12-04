package config

import "os"

// Config holds Purchase Service configuration
type Config struct {
	Port          string
	RedisAddr     string
	RedisPassword string
	RabbitMQURL   string
	IdempotencyTTL int
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Port:          getEnv("PURCHASE_SERVICE_PORT", ""),
		RedisAddr:     getEnv("REDIS_ADDR", ""),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RabbitMQURL:   getEnv("RABBITMQ_URL", ""),
		IdempotencyTTL: 120,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
