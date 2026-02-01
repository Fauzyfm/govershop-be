package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	// Server
	Port string
	Env  string

	// Database
	DatabaseURL string

	// Digiflazz
	DigiflazzUsername  string
	DigiflazzAPIKey    string
	DigiflazzDevKey    string
	DigiflazzWebhookIP string

	// Pakasir
	PakasirProject    string
	PakasirAPIKey     string
	PakasirWebhookURL string

	// Pricing
	DefaultMarkupPercent float64

	// Sync
	// Sync
	ProductSyncInterval int // in minutes

	// Admin Auth
	AdminUsername string
	AdminPassword string
	JWTSecret     string
}

// Global config instance
var AppConfig *Config

// Load loads configuration from environment variables
func Load() *Config {
	// Load .env file if exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	config := &Config{
		// Server
		Port: getEnv("PORT", "8080"),
		Env:  getEnv("ENV", "development"),

		// Database
		DatabaseURL: getEnv("DATABASE_URL", ""),

		// Digiflazz
		DigiflazzUsername:  getEnv("DIGIFLAZZ_USERNAME", ""),
		DigiflazzAPIKey:    getEnv("DIGIFLAZZ_API_KEY", ""),
		DigiflazzDevKey:    getEnv("DIGIFLAZZ_DEV_KEY", ""),
		DigiflazzWebhookIP: getEnv("DIGIFLAZZ_WEBHOOK_IP", "52.74.250.133"),

		// Pakasir
		PakasirProject:    getEnv("PAKASIR_PROJECT", ""),
		PakasirAPIKey:     getEnv("PAKASIR_API_KEY", ""),
		PakasirWebhookURL: getEnv("PAKASIR_WEBHOOK_URL", ""),

		// Pricing
		DefaultMarkupPercent: getEnvFloat("DEFAULT_MARKUP_PERCENT", 3.0),

		// Sync
		ProductSyncInterval: getEnvInt("PRODUCT_SYNC_INTERVAL", 30),

		// Admin Auth
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),
		JWTSecret:     getEnv("JWT_SECRET", "superdupersecretjwtkey"),
	}

	AppConfig = config
	return config
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// GetDigiflazzKey returns the appropriate key based on environment
func (c *Config) GetDigiflazzKey() string {
	if c.IsDevelopment() {
		return c.DigiflazzDevKey
	}
	return c.DigiflazzAPIKey
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}
