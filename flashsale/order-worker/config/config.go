package config

import "os"

// Config holds Order Worker configuration
type Config struct {
	RabbitMQURL string
	PostgresURL string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		RabbitMQURL: getEnv("RABBITMQ_URL", ""),
		PostgresURL: getEnv("POSTGRES_URL", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
