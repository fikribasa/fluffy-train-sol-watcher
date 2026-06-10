package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	FlareSolverrURL     string
	FlareSolverrTimeout int
	PollInterval        time.Duration
	SolThreshold        float64
	DBPath              string
	TelegramToken       string
	TelegramChatID      string
	TelegramThreadId    string
	ErrorCooldown       time.Duration
}

func Load() (*Config, error) {
	// Load .env if present; ignore error if file doesn't exist
	_ = godotenv.Load()

	cfg := &Config{}

	cfg.FlareSolverrURL = envOrDefault("FLARESOLVERR_URL", "http://127.0.0.1:8191/v1")

	timeoutStr := envOrDefault("FLARESOLVERR_TIMEOUT", "60000")
	timeout, err := strconv.Atoi(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid FLARESOLVERR_TIMEOUT %q: %w", timeoutStr, err)
	}
	cfg.FlareSolverrTimeout = timeout

	pollStr := envOrDefault("POLL_INTERVAL", "2m")
	d, err := time.ParseDuration(pollStr)
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", pollStr, err)
	}
	cfg.PollInterval = d

	threshStr := envOrDefault("SOL_THRESHOLD", "3.0")
	thresh, err := strconv.ParseFloat(threshStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid SOL_THRESHOLD %q: %w", threshStr, err)
	}
	cfg.SolThreshold = thresh

	cfg.DBPath = envOrDefault("DB_PATH", "./watcher.db")

	cfg.TelegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cfg.TelegramChatID = os.Getenv("TELEGRAM_CHAT_ID")
	cfg.TelegramThreadId = os.Getenv("TELEGRAM_THREAD_ID")

	cooldownStr := envOrDefault("ERROR_COOLDOWN", "10m")
	cooldown, err := time.ParseDuration(cooldownStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ERROR_COOLDOWN %q: %w", cooldownStr, err)
	}
	cfg.ErrorCooldown = cooldown

	return cfg, nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "WARN: env %s is not set\n", key)
	}
	return v
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
