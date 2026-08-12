package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	JwtSecret   string
	Port        string
	GoEnv       string
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

	cfg := Config{
		DatabaseURL: databaseUrl,
		JwtSecret:   jwtSecret,
		Port:        getEnv("PORT", "8080"),
		GoEnv:       getEnv("GO_ENV", "development"),
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
