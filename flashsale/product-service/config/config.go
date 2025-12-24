package config

import "os"

type Config struct {
	Port          string
	PostgresURL   string
	RedisAddr     string
	RedisPassword string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PRODUCT_SERVICE_PORT", "8083"),
		PostgresURL:   os.Getenv("PRODUCT_POSTGRES_URL"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
