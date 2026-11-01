package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
	// Встроенная база часовых поясов: на Windows и в минимальных образах системной базы может не быть,
	// а SCHOOL_TIMEZONE (например, Europe/Moscow) должен загружаться везде одинаково.
	_ "time/tzdata"
)

type Config struct {
	Environment         string
	NetworkProfile      string
	HTTPAddr            string
	DatabaseURL         string
	SessionCookieSecure bool
	SessionTTL          time.Duration
	SchoolTimezone      string
	FileStoragePath     string
	MaxUploadBytes      int64
	StaticDir           string
	// TrustProxyHeaders разрешает брать адрес клиента из X-Real-IP. Включать только когда сервер
	// недоступен напрямую и все запросы приходят через Nginx, который перезаписывает этот заголовок.
	TrustProxyHeaders bool
	// TLSCertFile — путь к сертификату Nginx (только публичная часть), чтобы показывать срок действия на странице «Система».
	TLSCertFile string
	Argon2      Argon2Config
	// PasswordHashConcurrency — сколько хешей паролей считается одновременно; каждый занимает Argon2.MemoryKiB памяти,
	// поэтому произведение должно с запасом укладываться в лимит памяти процесса (по умолчанию 2 × 64 МиБ).
	PasswordHashConcurrency int
}

type Argon2Config struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	KeyLength   uint32
}

func Load() (Config, error) {
	cfg := Config{
		Environment:         env("APP_ENV", "development"),
		NetworkProfile:      env("NETWORK_PROFILE", "local-http"),
		HTTPAddr:            env("HTTP_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		SessionCookieSecure: envBool("SESSION_COOKIE_SECURE", false),
		SessionTTL:          envDuration("SESSION_TTL", 168*time.Hour),
		SchoolTimezone:      env("SCHOOL_TIMEZONE", "Europe/Moscow"),
		FileStoragePath:     env("FILE_STORAGE_PATH", "./data/files"),
		MaxUploadBytes:      envInt64("MAX_UPLOAD_BYTES", 10<<20),
		StaticDir:           os.Getenv("STATIC_DIR"),
		TrustProxyHeaders:   envBool("TRUST_PROXY_HEADERS", false),
		TLSCertFile:         os.Getenv("TLS_CERT_FILE"),
		Argon2: Argon2Config{
			MemoryKiB:   uint32(envInt64("ARGON2_MEMORY_KIB", 64*1024)),
			Iterations:  uint32(envInt64("ARGON2_TIME", 3)),
			Parallelism: uint8(envInt64("ARGON2_PARALLELISM", 2)),
			KeyLength:   uint32(envInt64("ARGON2_KEY_LENGTH", 32)),
		},
		PasswordHashConcurrency: int(envInt64("PASSWORD_HASH_CONCURRENCY", 2)),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.NetworkProfile != "local-http" && cfg.NetworkProfile != "https" {
		return Config{}, fmt.Errorf("NETWORK_PROFILE must be local-http or https")
	}
	if cfg.NetworkProfile == "https" && !cfg.SessionCookieSecure {
		return Config{}, fmt.Errorf("https profile requires secure session cookies")
	}
	if cfg.Environment == "production" && cfg.NetworkProfile != "local-http" && !cfg.SessionCookieSecure {
		return Config{}, fmt.Errorf("public production cannot use insecure session cookies")
	}
	if cfg.SessionTTL < 15*time.Minute || cfg.SessionTTL > 30*24*time.Hour {
		return Config{}, fmt.Errorf("SESSION_TTL must be between 15m and 720h")
	}
	if cfg.MaxUploadBytes < 1 || cfg.MaxUploadBytes > 50<<20 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES must be between 1 and 52428800")
	}
	if _, err := time.LoadLocation(cfg.SchoolTimezone); err != nil {
		return Config{}, fmt.Errorf("invalid SCHOOL_TIMEZONE: %w", err)
	}
	if cfg.PasswordHashConcurrency < 1 || cfg.PasswordHashConcurrency > 16 {
		return Config{}, fmt.Errorf("PASSWORD_HASH_CONCURRENCY must be between 1 and 16")
	}
	if cfg.Argon2.MemoryKiB < 19*1024 || cfg.Argon2.Iterations < 1 || cfg.Argon2.Parallelism < 1 || cfg.Argon2.KeyLength < 16 {
		return Config{}, fmt.Errorf("Argon2 parameters are below safe minimums")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
