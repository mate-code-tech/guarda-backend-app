package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBURL        string
	GeminiAPIKey string
	Port         string
}

func Load() *Config {
	godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		DBURL:        os.Getenv("DB_URL"),
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		Port:         port,
	}
}
