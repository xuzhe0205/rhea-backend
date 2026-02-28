package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GeminiAPIKey string
	GeminiModel  string
	// later: OpenAIAPIKey, OpenAIModel...
	DBDSN string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// 在生产环境（如 Docker 运行）中，没有 .env 是正常的，所以通常只打 log 不 return err
		fmt.Println("Info: .env file not found, using system env variables")
	}
	dbPass := os.Getenv("DB_PASSWORD")
	if dbPass == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required in environment")
	}
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		dbPass,
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_SSLMODE"),
	)

	cfg := &Config{
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		GeminiModel:  getenvDefault("GEMINI_MODEL", "gemini-2.5-flash"),
		DBDSN:        dsn,
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
