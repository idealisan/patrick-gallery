package app

import (
	"fmt"
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

	// CompatVersion is the Immich server version this Go port advertises to
	// official clients (mobile/web). It MUST match the version the client
	// expects or the app will refuse to connect. Default tracks a recent
	// stable Immich release; override with IMMICH_COMPAT_VERSION if your
	// client requires a specific version.
	CompatVersion string
	CompatMajor   int
	CompatMinor   int
	CompatPatch   int
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func LoadConfig() *Config {
	port, _ := strconv.Atoi(getEnv("IMMICH_PORT", "8081"))
	cfg := &Config{
		Host:           getEnv("IMMICH_HOST", "0.0.0.0"),
		Port:           port,
		DBPath:         getEnv("IMMICH_DB", "immich.db"),
		ResourceDir:    getEnv("IMMICH_RESOURCE", "resources"),
		JWTSecret:      getEnv("IMMICH_JWT_SECRET", "immich-dev-secret-change-me"),
		APIKeySalt:     getEnv("IMMICH_API_KEY_SALT", "immich-dev-api-salt"),
		LoginRequired:  getEnv("IMMICH_LOGIN_REQUIRED", "true") == "true",
		ExternalDomain: getEnv("IMMICH_EXTERNAL_DOMAIN", ""),
		CompatVersion:  getEnv("IMMICH_COMPAT_VERSION", "1.130.0"),
	}
	maj, min, pat := 1, 130, 0
	if n, err := fmt.Sscanf(cfg.CompatVersion, "%d.%d.%d", &maj, &min, &pat); n >= 1 && err == nil {
		cfg.CompatMajor, cfg.CompatMinor, cfg.CompatPatch = maj, min, pat
	} else {
		cfg.CompatMajor, cfg.CompatMinor, cfg.CompatPatch = 1, 130, 0
	}
	return cfg
}
