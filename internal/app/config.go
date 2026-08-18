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

	// TrashDays is how long a trashed asset is retained before the automatic
	// cleanup job permanently deletes it. Mirrors Immich's trashDays setting.
	TrashDays int

	// PreviewSize is the longest-edge resolution (px) used when generating a
	// "preview" render on demand from the original. Mirrors Immich's generated
	// preview image size. Override with IMMICH_PREVIEW_SIZE.
	PreviewSize int

	OCRProvider string
	OCRBaseURL  string
	OCRAPIKey   string
	OCRModel    string
	OCRPrompt   string
	OCRDetail   string
	OCRTimeout  int
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// trashDaysFromEnv reads IMMICH_TRASH_DAYS (defaults to 30) using the same
// int-parse fallback pattern as the rest of LoadConfig.
func trashDaysFromEnv() int {
	if v := os.Getenv("IMMICH_TRASH_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 30
}

// previewSizeFromEnv reads IMMICH_PREVIEW_SIZE (defaults to 2560) — the
// longest-edge resolution of on-demand preview renders.
func previewSizeFromEnv() int {
	if v := os.Getenv("IMMICH_PREVIEW_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 2560
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
		CompatVersion:  getEnv("IMMICH_COMPAT_VERSION", "3.1.0"),
		TrashDays:      trashDaysFromEnv(),
		PreviewSize:    previewSizeFromEnv(),
		OCRProvider:    getEnv("IMMICH_OCR_PROVIDER", "none"),
		OCRBaseURL:     getEnv("IMMICH_OCR_BASE_URL", "https://api.openai.com/v1"),
		OCRAPIKey:      getEnv("IMMICH_OCR_API_KEY", ""),
		OCRModel:       getEnv("IMMICH_OCR_MODEL", "gpt-4.1-mini"),
		OCRPrompt:      getEnv("IMMICH_OCR_PROMPT", ""),
		OCRDetail:      getEnv("IMMICH_OCR_DETAIL", "high"),
		OCRTimeout:     envInt("IMMICH_OCR_TIMEOUT_SECONDS", 120),
	}
	maj, min, pat := 3, 1, 0
	if n, err := fmt.Sscanf(cfg.CompatVersion, "%d.%d.%d", &maj, &min, &pat); n >= 1 && err == nil {
		cfg.CompatMajor, cfg.CompatMinor, cfg.CompatPatch = maj, min, pat
	} else {
		cfg.CompatMajor, cfg.CompatMinor, cfg.CompatPatch = 1, 130, 0
	}
	return cfg
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
