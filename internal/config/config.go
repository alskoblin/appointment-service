package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type BookingConfig struct {
	AppPort       string
	DBHost        string
	DBPort        string
	DBName        string
	DBUser        string
	DBPassword    string
	DBSSLMode     string
	LogLevel      string
	KafkaBrokers  []string
	KafkaTopic    string
	JWTSecret     string
	JWTTTLMinutes int
}

type BillingConfig struct {
	AppPort            string
	DBHost             string
	DBPort             string
	DBName             string
	DBUser             string
	DBPassword         string
	DBSSLMode          string
	LogLevel           string
	KafkaBrokers       []string
	KafkaTopic         string
	KafkaGroupID       string
	DefaultAmountCents int64
	Currency           string
	PaymentsEnabled    bool
}

type NotificationConfig struct {
	DBHost             string
	DBPort             string
	DBName             string
	DBUser             string
	DBPassword         string
	DBSSLMode          string
	LogLevel           string
	KafkaBrokers       []string
	KafkaTopic         string
	KafkaGroupID       string
	TelegramBotToken   string
	TelegramAPIBaseURL string
}

type CalendarSyncConfig struct {
	DBHost                    string
	DBPort                    string
	DBName                    string
	DBUser                    string
	DBPassword                string
	DBSSLMode                 string
	LogLevel                  string
	KafkaBrokers              []string
	KafkaTopic                string
	KafkaGroupID              string
	GoogleCalendarID          string
	GoogleServiceAccountEmail string
	GoogleServiceAccountKey   string
	GoogleTokenURI            string
	GoogleAPIBaseURL          string
	GoogleAccessToken         string
}

func LoadBooking() (BookingConfig, error) {
	jwtTTLMinute, err := parseIntEnv("BOOKING_JWT_TTL_MINUTES", 720)
	if err != nil {
		return BookingConfig{}, err
	}

	cfg := BookingConfig{
		AppPort:       getEnv("BOOKING_APP_PORT", getEnv("APP_PORT", "8080")),
		DBHost:        getEnv("BOOKING_DB_HOST", getEnv("DB_HOST", "localhost")),
		DBPort:        getEnv("BOOKING_DB_PORT", getEnv("DB_PORT", "5432")),
		DBName:        getEnv("BOOKING_DB_NAME", getEnv("DB_NAME", "booking_db")),
		DBUser:        getEnv("BOOKING_DB_USER", getEnv("DB_USER", "postgres")),
		DBPassword:    getEnv("BOOKING_DB_PASSWORD", getEnv("DB_PASSWORD", "postgres")),
		DBSSLMode:     getEnv("BOOKING_DB_SSLMODE", getEnv("DB_SSLMODE", "disable")),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		KafkaBrokers:  splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:    getEnv("KAFKA_APPOINTMENTS_TOPIC", "appointments.events.v1"),
		JWTSecret:     getEnv("BOOKING_JWT_SECRET", getEnv("JWT_SECRET", "dev-secret-change-me")),
		JWTTTLMinutes: jwtTTLMinute,
	}
	if err := validateDB(cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser); err != nil {
		return BookingConfig{}, err
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return BookingConfig{}, fmt.Errorf("BOOKING_JWT_SECRET is required")
	}
	if cfg.JWTTTLMinutes <= 0 {
		return BookingConfig{}, fmt.Errorf("BOOKING_JWT_TTL_MINUTES must be positive")
	}
	return cfg, nil
}

func LoadNotification() (NotificationConfig, error) {
	cfg := NotificationConfig{
		DBHost:             getEnv("NOTIFICATION_DB_HOST", getEnv("DB_HOST", "localhost")),
		DBPort:             getEnv("NOTIFICATION_DB_PORT", getEnv("DB_PORT", "5432")),
		DBName:             getEnv("NOTIFICATION_DB_NAME", "notification_db"),
		DBUser:             getEnv("NOTIFICATION_DB_USER", getEnv("DB_USER", "postgres")),
		DBPassword:         getEnv("NOTIFICATION_DB_PASSWORD", getEnv("DB_PASSWORD", "postgres")),
		DBSSLMode:          getEnv("NOTIFICATION_DB_SSLMODE", getEnv("DB_SSLMODE", "disable")),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		KafkaBrokers:       splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:         getEnv("KAFKA_APPOINTMENTS_TOPIC", "appointments.events.v1"),
		KafkaGroupID:       getEnv("KAFKA_NOTIFICATION_GROUP", "notification-service-v1"),
		TelegramBotToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramAPIBaseURL: getEnv("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
	}
	if err := validateDB(cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser); err != nil {
		return NotificationConfig{}, err
	}
	if len(cfg.KafkaBrokers) == 0 {
		return NotificationConfig{}, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.KafkaGroupID == "" {
		return NotificationConfig{}, fmt.Errorf("KAFKA_NOTIFICATION_GROUP is required")
	}
	return cfg, nil
}

func LoadBilling() (BillingConfig, error) {
	amount, err := parseInt64Env("BILLING_DEFAULT_AMOUNT_CENTS", 5000)
	if err != nil {
		return BillingConfig{}, err
	}

	cfg := BillingConfig{
		AppPort:            getEnv("BILLING_APP_PORT", "8081"),
		DBHost:             getEnv("BILLING_DB_HOST", getEnv("DB_HOST", "localhost")),
		DBPort:             getEnv("BILLING_DB_PORT", getEnv("DB_PORT", "5432")),
		DBName:             getEnv("BILLING_DB_NAME", "billing_db"),
		DBUser:             getEnv("BILLING_DB_USER", getEnv("DB_USER", "postgres")),
		DBPassword:         getEnv("BILLING_DB_PASSWORD", getEnv("DB_PASSWORD", "postgres")),
		DBSSLMode:          getEnv("BILLING_DB_SSLMODE", getEnv("DB_SSLMODE", "disable")),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		KafkaBrokers:       splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:         getEnv("KAFKA_APPOINTMENTS_TOPIC", "appointments.events.v1"),
		KafkaGroupID:       getEnv("KAFKA_BILLING_GROUP", "billing-service-v1"),
		DefaultAmountCents: amount,
		Currency:           getEnv("BILLING_CURRENCY", "RUB"),
		PaymentsEnabled:    getEnvBool("BILLING_PAYMENTS_ENABLED", true),
	}
	if err := validateDB(cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser); err != nil {
		return BillingConfig{}, err
	}
	if len(cfg.KafkaBrokers) == 0 {
		return BillingConfig{}, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.KafkaGroupID == "" {
		return BillingConfig{}, fmt.Errorf("KAFKA_BILLING_GROUP is required")
	}
	if cfg.DefaultAmountCents <= 0 {
		return BillingConfig{}, fmt.Errorf("BILLING_DEFAULT_AMOUNT_CENTS must be positive")
	}
	if strings.TrimSpace(cfg.Currency) == "" {
		return BillingConfig{}, fmt.Errorf("BILLING_CURRENCY is required")
	}
	return cfg, nil
}

func LoadCalendarSync() (CalendarSyncConfig, error) {
	cfg := CalendarSyncConfig{
		DBHost:                    getEnv("CALENDAR_DB_HOST", getEnv("DB_HOST", "localhost")),
		DBPort:                    getEnv("CALENDAR_DB_PORT", getEnv("DB_PORT", "5432")),
		DBName:                    getEnv("CALENDAR_DB_NAME", "calendar_db"),
		DBUser:                    getEnv("CALENDAR_DB_USER", getEnv("DB_USER", "postgres")),
		DBPassword:                getEnv("CALENDAR_DB_PASSWORD", getEnv("DB_PASSWORD", "postgres")),
		DBSSLMode:                 getEnv("CALENDAR_DB_SSLMODE", getEnv("DB_SSLMODE", "disable")),
		LogLevel:                  getEnv("LOG_LEVEL", "info"),
		KafkaBrokers:              splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:                getEnv("KAFKA_APPOINTMENTS_TOPIC", "appointments.events.v1"),
		KafkaGroupID:              getEnv("KAFKA_CALENDAR_GROUP", "calendar-sync-service-v1"),
		GoogleCalendarID:          os.Getenv("GOOGLE_CALENDAR_ID"),
		GoogleServiceAccountEmail: os.Getenv("GOOGLE_SERVICE_ACCOUNT_EMAIL"),
		GoogleServiceAccountKey:   os.Getenv("GOOGLE_SERVICE_ACCOUNT_PRIVATE_KEY"),
		GoogleTokenURI:            getEnv("GOOGLE_TOKEN_URI", "https://oauth2.googleapis.com/token"),
		GoogleAPIBaseURL:          getEnv("GOOGLE_CALENDAR_API_BASE_URL", "https://www.googleapis.com/calendar/v3"),
		GoogleAccessToken:         os.Getenv("GOOGLE_CALENDAR_ACCESS_TOKEN"),
	}
	if err := validateDB(cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser); err != nil {
		return CalendarSyncConfig{}, err
	}
	if len(cfg.KafkaBrokers) == 0 {
		return CalendarSyncConfig{}, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.KafkaGroupID == "" {
		return CalendarSyncConfig{}, fmt.Errorf("KAFKA_CALENDAR_GROUP is required")
	}
	if cfg.GoogleCalendarID == "" {
		return CalendarSyncConfig{}, fmt.Errorf("GOOGLE_CALENDAR_ID is required")
	}
	if cfg.GoogleAccessToken == "" {
		if cfg.GoogleServiceAccountEmail == "" || cfg.GoogleServiceAccountKey == "" {
			return CalendarSyncConfig{}, fmt.Errorf("either GOOGLE_CALENDAR_ACCESS_TOKEN or both GOOGLE_SERVICE_ACCOUNT_EMAIL and GOOGLE_SERVICE_ACCOUNT_PRIVATE_KEY are required")
		}
	}
	return cfg, nil
}

func (c BookingConfig) DatabaseURL() string {
	return databaseURL(c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func (c NotificationConfig) DatabaseURL() string {
	return databaseURL(c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func (c BillingConfig) DatabaseURL() string {
	return databaseURL(c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func (c CalendarSyncConfig) DatabaseURL() string {
	return databaseURL(c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func databaseURL(user, password, host, port, name, sslmode string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, name, sslmode)
}

func validateDB(host, port, name, user string) error {
	if host == "" || port == "" || name == "" || user == "" {
		return fmt.Errorf("database configuration is incomplete")
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func parseInt64Env(key string, fallback int64) (int64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be int64: %w", key, err)
	}
	return value, nil
}

func parseIntEnv(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be int: %w", key, err)
	}
	return value, nil
}

func getEnvBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return parsed
}
