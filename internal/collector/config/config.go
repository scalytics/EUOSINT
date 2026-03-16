package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultOutputPath       = "public/alerts.json"
	defaultFilteredPath     = "public/alerts-filtered.json"
	defaultStatePath        = "public/alerts-state.json"
	defaultSourceHealthPath = "public/source-health.json"
	defaultRegistryPath     = "registry/source_registry.json"
	defaultTimeoutMS        = 15000
	defaultIntervalMS       = 900000
	defaultMaxPerSource     = 20
	defaultMaxAgeDays       = 180
	defaultRemovedDays      = 14
	defaultMaxBodyBytes     = 2 * 1024 * 1024
)

type Config struct {
	RegistryPath                    string
	OutputPath                      string
	FilteredOutputPath              string
	StateOutputPath                 string
	SourceHealthOutputPath          string
	MaxPerSource                    int
	MaxAgeDays                      int
	RemovedRetentionDays            int
	IncidentRelevanceThreshold      float64
	MissingPersonRelevanceThreshold float64
	FailOnCriticalSourceGap         bool
	CriticalSourcePrefixes          []string
	Watch                           bool
	IntervalMS                      int
	HTTPTimeoutMS                   int
	MaxResponseBodyBytes            int64
	UserAgent                       string
}

func Default() Config {
	return Config{
		RegistryPath:                    defaultRegistryPath,
		OutputPath:                      defaultOutputPath,
		FilteredOutputPath:              defaultFilteredPath,
		StateOutputPath:                 defaultStatePath,
		SourceHealthOutputPath:          defaultSourceHealthPath,
		MaxPerSource:                    defaultMaxPerSource,
		MaxAgeDays:                      defaultMaxAgeDays,
		RemovedRetentionDays:            defaultRemovedDays,
		IncidentRelevanceThreshold:      0.42,
		MissingPersonRelevanceThreshold: 0,
		FailOnCriticalSourceGap:         false,
		CriticalSourcePrefixes:          []string{"cisa"},
		Watch:                           false,
		IntervalMS:                      defaultIntervalMS,
		HTTPTimeoutMS:                   defaultTimeoutMS,
		MaxResponseBodyBytes:            defaultMaxBodyBytes,
		UserAgent:                       "euosint-bot/1.0",
	}
}

func FromEnv() Config {
	cfg := Default()
	cfg.OutputPath = envString("OUTPUT_PATH", cfg.OutputPath)
	cfg.FilteredOutputPath = envString("FILTERED_OUTPUT_PATH", cfg.FilteredOutputPath)
	cfg.StateOutputPath = envString("STATE_OUTPUT_PATH", cfg.StateOutputPath)
	cfg.SourceHealthOutputPath = envString("SOURCE_HEALTH_OUTPUT_PATH", cfg.SourceHealthOutputPath)
	cfg.RegistryPath = envString("SOURCE_REGISTRY_PATH", cfg.RegistryPath)
	cfg.MaxPerSource = envInt("MAX_PER_SOURCE", cfg.MaxPerSource)
	cfg.MaxAgeDays = envInt("MAX_AGE_DAYS", cfg.MaxAgeDays)
	cfg.RemovedRetentionDays = envInt("REMOVED_RETENTION_DAYS", cfg.RemovedRetentionDays)
	cfg.IncidentRelevanceThreshold = envFloat("INCIDENT_RELEVANCE_THRESHOLD", cfg.IncidentRelevanceThreshold)
	cfg.MissingPersonRelevanceThreshold = envFloat("MISSING_PERSON_RELEVANCE_THRESHOLD", cfg.MissingPersonRelevanceThreshold)
	cfg.FailOnCriticalSourceGap = envBool("FAIL_ON_CRITICAL_SOURCE_GAP", cfg.FailOnCriticalSourceGap)
	cfg.CriticalSourcePrefixes = envCSV("CRITICAL_SOURCE_PREFIXES", cfg.CriticalSourcePrefixes)
	cfg.Watch = envBool("WATCH", cfg.Watch)
	cfg.IntervalMS = envInt("INTERVAL_MS", cfg.IntervalMS)
	cfg.HTTPTimeoutMS = envInt("HTTP_TIMEOUT_MS", cfg.HTTPTimeoutMS)
	cfg.MaxResponseBodyBytes = int64(envInt("MAX_RESPONSE_BODY_BYTES", int(cfg.MaxResponseBodyBytes)))
	cfg.UserAgent = envString("USER_AGENT", cfg.UserAgent)
	return cfg
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func envCSV(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
