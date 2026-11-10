package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port           string
	AppEnv         string
	DBURL          string
	JWTSecret      string
	JWTExpiryHours int
	CORSOrigins    []string
}

// Load reads configuration from .env files (if present) and environment variables.
func Load() *Config {
	loadDotEnv(".env", "backend/.env", "../.env")

	expiryHours := 24
	if raw := strings.TrimSpace(os.Getenv("JWT_EXPIRY_HOURS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			expiryHours = parsed
		}
	}

	rawOrigins := getEnv("CORS_ORIGINS", "http://localhost:4200")
	var origins []string
	for _, o := range strings.Split(rawOrigins, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		AppEnv:         getEnv("APP_ENV", "development"),
		DBURL:          getEnv("DB_URL", "postgres://postgres:postgres@localhost:5432/craftbridge?sslmode=disable"),
		JWTSecret:      getEnv("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		JWTExpiryHours: expiryHours,
		CORSOrigins:    origins,
	}
}

func loadDotEnv(paths ...string) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if key, val, ok := strings.Cut(line, "="); ok {
				k := strings.TrimSpace(key)
				v := strings.Trim(strings.TrimSpace(val), `"'`)
				if k != "" && os.Getenv(k) == "" {
					_ = os.Setenv(k, v)
				}
			}
		}
		_ = file.Close()
		break
	}
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
