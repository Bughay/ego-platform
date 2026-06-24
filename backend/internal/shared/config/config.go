package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	JWT       JWTConfig
	Agent     AgentConfig
	Redis     RedisConfig
	RateLimit RateLimitConfig
}

// RateLimitConfig controls the per-IP fixed-window request limiter.
// Defaults are applied in LoadConfig so the limiter works with no .env entry.
type RateLimitConfig struct {
	Enabled   bool // default true
	Requests  int  // default 100 — max requests per window per IP
	WindowSec int  // default 60 — window length in seconds
}

// RedisConfig holds Redis connection settings.
// All fields default safely so local dev works without a .env entry.
type RedisConfig struct {
	Addr     string // default "localhost:6379"
	Password string // default "" (no auth)
	DB       int    // default 0
}

type ServerConfig struct {
	Port            string
	ReadTimeoutSec  int
	WriteTimeoutSec int
	LogFormat       string // "json" or "text" (default)
	LogLevel        string // "debug", "info" (default), "warn", "error"
}

type DatabaseConfig struct {
	DSN string // PostgreSQL Data Source Name
}

type JWTConfig struct {
	Secret            string
	AccessExpiryHours int
	RefreshExpiryDays int
}

type AgentConfig struct {
	DEEPSEEKAPIKEY string
	GROKAPIKEY     string
}

// Load reads environment variables and returns a populated Config struct.
// It fails fast if required variables are missing.
func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("config: JWT_SECRET environment variable is required")
	}

	dbDSN := os.Getenv("DATABASE_URL")
	if dbDSN == "" {
		return nil, fmt.Errorf("config: DATABASE_URL environment variable is required")
	}

	readTimeout, _ := strconv.Atoi(os.Getenv("SERVER_READ_TIMEOUT_SEC"))
	if readTimeout == 0 {
		readTimeout = 10
	}
	writeTimeout, _ := strconv.Atoi(os.Getenv("SERVER_WRITE_TIMEOUT_SEC"))
	if writeTimeout == 0 {
		writeTimeout = 30
	}
	jwtExpiry, _ := strconv.Atoi(os.Getenv("JWT_EXPIRY_HOURS"))
	if jwtExpiry == 0 {
		jwtExpiry = 1
	}
	// Refresh tokens have their own lifetime, configured independently of the
	// access token and expressed in days (matching the RefreshExpiryDays field).
	refreshExpiryDays, _ := strconv.Atoi(os.Getenv("JWT_REFRESH_EXPIRY_DAYS"))
	if refreshExpiryDays == 0 {
		refreshExpiryDays = 7 // a week
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	deepseekApiKey := os.Getenv("DEEPSEEKAPIKEY")
	grokApiKey := os.Getenv("GROKAPIKEY")

	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "text"
	}
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))

	// Rate limiting: enabled unless explicitly set to "false".
	rateLimitEnabled := os.Getenv("RATE_LIMIT_ENABLED") != "false"
	rateLimitRequests, _ := strconv.Atoi(os.Getenv("RATE_LIMIT_REQUESTS"))
	if rateLimitRequests == 0 {
		rateLimitRequests = 100
	}
	rateLimitWindow, _ := strconv.Atoi(os.Getenv("RATE_LIMIT_WINDOW_SEC"))
	if rateLimitWindow == 0 {
		rateLimitWindow = 60
	}

	return &Config{
		Server: ServerConfig{
			Port:            port,
			ReadTimeoutSec:  readTimeout,
			WriteTimeoutSec: writeTimeout,
			LogFormat:       logFormat,
			LogLevel:        logLevel,
		},
		Database: DatabaseConfig{DSN: dbDSN},
		JWT:      JWTConfig{Secret: secret, AccessExpiryHours: jwtExpiry, RefreshExpiryDays: refreshExpiryDays},
		Agent:    AgentConfig{DEEPSEEKAPIKEY: deepseekApiKey, GROKAPIKEY: grokApiKey},
		Redis: RedisConfig{
			Addr:     redisAddr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       redisDB,
		},
		RateLimit: RateLimitConfig{
			Enabled:   rateLimitEnabled,
			Requests:  rateLimitRequests,
			WindowSec: rateLimitWindow,
		},
	}, nil
}
