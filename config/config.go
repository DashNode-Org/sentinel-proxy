package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	SentinelBackends         []string
	ProxyPort                int
	HealthCheckInterval      time.Duration
	IntegrityCheckInterval   time.Duration
	IntegrityCheckEpochs     int
	RequestTimeout           time.Duration
	LogLevel                 string
	SlotsPerEpoch            int
	ArchiverThresholdEpochs  int
	ExpectedValidators       int
	IntegrityScoreThreshold  int
}

func Load() *Config {
	return &Config{
		SentinelBackends:        parseStringSlice(getEnv("SENTINEL_BACKENDS", "")),
		ProxyPort:               parseInt(getEnv("PROXY_PORT", "8080")),
		HealthCheckInterval:     parseDurationMs(getEnv("HEALTH_CHECK_INTERVAL_MS", "30000")),
		IntegrityCheckInterval:  parseDurationMs(getEnv("INTEGRITY_CHECK_INTERVAL_MS", "60000")),
		IntegrityCheckEpochs:    parseInt(getEnv("INTEGRITY_CHECK_EPOCHS", "10")),
		RequestTimeout:          parseDurationMs(getEnv("REQUEST_TIMEOUT_MS", "30000")),
		LogLevel:                getEnv("LOG_LEVEL", "info"),
		SlotsPerEpoch:           parseInt(getEnv("SLOTS_PER_EPOCH", "32")),
		ArchiverThresholdEpochs: parseInt(getEnv("ARCHIVER_THRESHOLD_EPOCHS", "100")),
		ExpectedValidators:      parseInt(getEnv("EXPECTED_VALIDATORS", "24")),
		IntegrityScoreThreshold: parseInt(getEnv("INTEGRITY_SCORE_THRESHOLD", "95")),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseDurationMs(s string) time.Duration {
	ms, _ := strconv.Atoi(s)
	return time.Duration(ms) * time.Millisecond
}

func parseStringSlice(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
