package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port                            string
	AppEnv                          string
	DBURL                           string
	JWTSecret                       string
	JWTExpiryHours                  int
	CORSOrigins                     []string
	PayHereMerchantID               string
	PayHereMerchantSecret           string
	PayHereCheckoutURL              string
	PayHereAppID                    string
	PayHereAppSecret                string
	PayHereAPIBaseURL               string
	PaymentReturnURL                string
	PaymentCancelURL                string
	PaymentNotifyURL                string
	PaymentReconcileAfterMinutes    int
	PaymentAttemptTTLHours          int
	PaymentReconcileIntervalSeconds int
	RefundWindowDays                int
}

// Load reads configuration from .env files (if present) and environment variables.
func Load() *Config {
	loadDotEnv(".env", "backend/.env", "../.env")

	expiryHours := getPositiveIntEnv("JWT_EXPIRY_HOURS", 24)
	reconcileAfterMinutes := getPositiveIntEnv("PAYMENT_RECONCILE_AFTER_MINUTES", 30)
	attemptTTLHours := getPositiveIntEnv("PAYMENT_ATTEMPT_TTL_HOURS", 24)
	reconcileIntervalSeconds := getNonNegativeIntEnv("PAYMENT_RECONCILE_INTERVAL_SECONDS", 300)
	refundWindowDays := getPositiveIntEnv("REFUND_WINDOW_DAYS", 30)

	rawOrigins := getEnv("CORS_ORIGINS", "http://localhost:4200")
	var origins []string
	for _, o := range strings.Split(rawOrigins, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	return &Config{
		Port:                            getEnv("PORT", "8080"),
		AppEnv:                          getEnv("APP_ENV", "development"),
		DBURL:                           getEnv("DB_URL", "postgres://postgres:postgres@localhost:5432/craftbridge?sslmode=disable"),
		JWTSecret:                       getEnv("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		JWTExpiryHours:                  expiryHours,
		CORSOrigins:                     origins,
		PayHereMerchantID:               getEnv("PAYHERE_MERCHANT_ID", ""),
		PayHereMerchantSecret:           getEnv("PAYHERE_MERCHANT_SECRET", ""),
		PayHereCheckoutURL:              getEnv("PAYHERE_CHECKOUT_URL", "https://sandbox.payhere.lk/pay/checkout"),
		PayHereAppID:                    getEnv("PAYHERE_APP_ID", ""),
		PayHereAppSecret:                getEnv("PAYHERE_APP_SECRET", ""),
		PayHereAPIBaseURL:               getEnv("PAYHERE_API_BASE_URL", "https://sandbox.payhere.lk"),
		PaymentReturnURL:                getEnv("PAYMENT_RETURN_URL", ""),
		PaymentCancelURL:                getEnv("PAYMENT_CANCEL_URL", ""),
		PaymentNotifyURL:                getEnv("PAYMENT_NOTIFY_URL", ""),
		PaymentReconcileAfterMinutes:    reconcileAfterMinutes,
		PaymentAttemptTTLHours:          attemptTTLHours,
		PaymentReconcileIntervalSeconds: reconcileIntervalSeconds,
		RefundWindowDays:                refundWindowDays,
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

func getPositiveIntEnv(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func getNonNegativeIntEnv(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return fallback
}
