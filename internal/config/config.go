package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GeminiAPIKey string
	GeminiModel  string
	DBDSN        string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Info: .env file not found, using system env variables")
	}

	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbPass := os.Getenv("DB_PASSWORD")
		if dbPass == "" {
			return nil, fmt.Errorf("DB_PASSWORD is required when DB_DSN is not set")
		}

		dbHost := getenvDefault("DB_HOST", "localhost")
		dbUser := getenvDefault("DB_USER", "postgres")
		dbName := getenvDefault("DB_NAME", "postgres")
		dbPort := getenvDefault("DB_PORT", "5432")
		dbSSLMode := getenvDefault("DB_SSLMODE", "disable")

		dbDSN = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
			dbHost,
			dbUser,
			dbPass,
			dbName,
			dbPort,
			dbSSLMode,
		)
	}

	cfg := &Config{
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		GeminiModel:  getenvDefault("GEMINI_MODEL", "gemini-2.5-flash"),
		DBDSN:        dbDSN,
	}

	if cfg.GeminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required")
	}

	return cfg, nil
}

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
