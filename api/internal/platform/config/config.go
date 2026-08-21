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

	OtelExporterEndpoint string
	OtelServiceName      string
	OtelSampleRatio      float64

	MetricsPort string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

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

		OtelExporterEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318/v1/traces"),
		OtelServiceName:      getEnv("OTEL_SERVICE_NAME", "order-api"),
		OtelSampleRatio:      getEnvFloat("OTEL_SAMPLE_RATIO", 1.0),

		MetricsPort: getEnv("METRICS_PORT", "9100"),
	}

	return &cfg, nil
}

func getEnvFloat(key string, fallback float64) float64 {
	valStr := os.Getenv(key)
	if valStr != "" {
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return fallback
		}
		return val
	}
	return fallback
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
