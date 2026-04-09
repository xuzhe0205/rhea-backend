package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GeminiEmbeddingModel string
	GeminiAPIKey         string
	GeminiAPIKeyFree     string
	ModelPro             string
	ModelFlash           string
	ModelLite            string
	DBDSN                string
	R2AccessKeyID        string
	R2SecretAccessKey    string
	R2AccountID          string
	R2Bucket             string
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
		GeminiEmbeddingModel: getenvDefault("GEMINI_EMBEDDING_TEXT_ONLY", "gemini-embedding-001"),
		GeminiAPIKey:         os.Getenv("GEMINI_API_KEY"),
		GeminiAPIKeyFree:     os.Getenv("GEMINI_API_KEY_FREE"),
		// 从环境变量读取，如果没有则赋予默认值
		ModelPro:   getenvDefault("GEMINI_MODEL_PRO", "gemini-2.5-pro"),
		ModelFlash: getenvDefault("GEMINI_MODEL_FLASH", "gemini-3-flash-preview"),
		ModelLite:  getenvDefault("GEMINI_MODEL_LITE", "gemini-3.1-flash-lite-preview"),
		DBDSN:      dbDSN,
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		R2Bucket:          getenvDefault("R2_BUCKET", "rhea-uploads"),
	}

	if cfg.GeminiAPIKey == "" || cfg.GeminiAPIKeyFree == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY and GEMINI_API_KEY_FREE are required")
	}

	return cfg, nil
}

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
