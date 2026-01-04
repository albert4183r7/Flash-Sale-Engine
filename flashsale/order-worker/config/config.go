package config

import "os"

// Config holds Order Worker configuration
type Config struct {
	RabbitMQURL       string
	OrderServiceURL   string
	ProductServiceURL string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		OrderServiceURL:   getEnv("ORDER_SERVICE_URL", "http://localhost:8084"),
		ProductServiceURL: getEnv("PRODUCT_SERVICE_URL", "http://localhost:8083"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
