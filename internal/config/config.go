package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port          string
	Env           string
	LogLevel      string
	DatabaseURL   string
	EncryptionKey []byte
	WorkerCount   int
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("APP_ENV", "development"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		WorkerCount: getEnvInt("WORKER_COUNT", 4),
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("Load: DATABASE_URL is required")
	}
	cfg.DatabaseURL = dsn

	keyB64 := os.Getenv("ENCRYPTION_KEY")
	if keyB64 == "" {
		return nil, fmt.Errorf("Load: ENCRYPTION_KEY is required (base64-encoded 32 bytes)")
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("Load: decode ENCRYPTION_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("Load: ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	cfg.EncryptionKey = key

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
