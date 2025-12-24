package config

import "os"

type Config struct {
	Port        string
	PostgresURL string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("ORDER_SERVICE_PORT", "8084"),
		PostgresURL: os.Getenv("ORDER_POSTGRES_URL"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
