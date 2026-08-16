package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	JwtSecret   string
	Port        string
	GoEnv       string
	RedisHost   string
	RedisPort   string
	JwtExpiry   time.Duration

	LoginRateLimit  int
	LoginRateWindow time.Duration
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	databaseUrl := os.Getenv("DATABASE_URL")
	if databaseUrl == "" {
		return nil, fmt.Errorf("missing DATABASE_URL")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("missing JWT_SECRET")
	}

	jwtExpiry, err := getEnvTimeDuration("JWT_EXPIRY")
	if err != nil {
		return nil, fmt.Errorf("JWT_EXPIRY is invalid format")
	}

	cfg := Config{
		DatabaseURL: databaseUrl,
		JwtSecret:   jwtSecret,
		Port:        getEnv("PORT", "8080"),
		GoEnv:       getEnv("GO_ENV", "development"),
		RedisHost:   getEnv("REDIS_HOST", "localhost"),
		RedisPort:   getEnv("REDIS_PORT", "6379"),
		JwtExpiry:   jwtExpiry,

		LoginRateLimit:  getEnvInt("LOGIN_RATE_LIMIT", 5),
		LoginRateWindow: getEnvDuration("LOGIN_RATE_WINDOW", 15*time.Minute),
	}

	return &cfg, nil
}

func getEnv(key, fallback string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return val
}

func getEnvInt(key string, fallback int) int {
	val, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return val
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	val, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return val
}

func getEnvTimeDuration(key string) (time.Duration, error) {
	val := os.Getenv(key)
	result, err := time.ParseDuration(val)
	if err != nil {
		return 0, err
	}
	return result, nil
}
