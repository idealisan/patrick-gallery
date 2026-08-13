package app

import (
	"os"
	"strconv"
)

// Config holds the runtime configuration for the Go Immich port.
// Values come from environment variables with sensible dev defaults so the
// server runs out-of-the-box (`go run .`).
type Config struct {
	Host           string
	Port           int
	DBPath         string
	ResourceDir    string // root dir for original + thumbnail + encoded files
	JWTSecret      string
	APIKeySalt     string
	LoginRequired  bool
	ExternalDomain string
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func LoadConfig() *Config {
	port, _ := strconv.Atoi(getEnv("IMMICH_PORT", "8081"))
	return &Config{
		Host:           getEnv("IMMICH_HOST", "0.0.0.0"),
		Port:           port,
		DBPath:         getEnv("IMMICH_DB", "immich.db"),
		ResourceDir:    getEnv("IMMICH_RESOURCE", "resources"),
		JWTSecret:      getEnv("IMMICH_JWT_SECRET", "immich-dev-secret-change-me"),
		APIKeySalt:     getEnv("IMMICH_API_KEY_SALT", "immich-dev-api-salt"),
		LoginRequired:  getEnv("IMMICH_LOGIN_REQUIRED", "true") == "true",
		ExternalDomain: getEnv("IMMICH_EXTERNAL_DOMAIN", ""),
	}
}
