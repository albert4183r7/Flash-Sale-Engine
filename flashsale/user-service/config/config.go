package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	PostgresURL   string
	RedisAddr     string
	RedisPassword string
	JWTSecret     string
	TokenExpiry   int // hours
}

func Load() *Config {
	tokenExpiry, _ := strconv.Atoi(os.Getenv("TOKEN_EXPIRY_HOURS"))
	if tokenExpiry == 0 {
		tokenExpiry = 24
	}

	return &Config{
		Port:          getEnv("USER_SERVICE_PORT", "8082"),
		PostgresURL:   os.Getenv("USER_POSTGRES_URL"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		JWTSecret:     getEnv("JWT_SECRET", "your-secret-key"),
		TokenExpiry:   tokenExpiry,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
