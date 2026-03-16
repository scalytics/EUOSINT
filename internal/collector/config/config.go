// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

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
	defaultRegistryPath     = "registry/sources.db"
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
	WikimediaUserAgent              string
	TranslateEnabled                bool
	BrowserEnabled                  bool
	BrowserTimeoutMS                int
	DiscoverMode                    bool
	DiscoverBackground              bool
	DiscoverIntervalMS              int
	DiscoverOutputPath              string
	CandidateQueuePath              string
	SearchDiscoveryEnabled          bool
	SearchDiscoveryMaxTargets       int
	SearchDiscoveryMaxURLsPerTarget int
	WikidataCachePath               string
	WikidataCacheTTLHours           int
	VettingEnabled                  bool
	VettingProvider                 string
	VettingBaseURL                  string
	VettingAPIKey                   string
	VettingModel                    string
	VettingTemperature              float64
	VettingMaxSampleItems           int
	AlertLLMEnabled                 bool
	AlertLLMModel                   string
	AlertLLMMaxItemsPerSource       int
	CategoryDictionaryPath          string
	ReplacementQueuePath            string
	SourceDBPath                    string
	SourceDBInit                    bool
	SourceDBImportRegistry          bool
	SourceDBMergeRegistry           bool
	SourceDBExportRegistry          bool
	CuratedSeedPath                 string
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
		UserAgent:                       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		WikimediaUserAgent:              "EUOSINTBot/1.0 (https://www.scalytics.io; ops@scalytics.io) WDQS discovery",
		TranslateEnabled:                true,
		BrowserEnabled:                  false,
		BrowserTimeoutMS:                30000,
		DiscoverMode:                    false,
		DiscoverBackground:              true,
		DiscoverIntervalMS:              defaultIntervalMS,
		DiscoverOutputPath:              "discover-results.json",
		CandidateQueuePath:              "registry/source_candidates.json",
		SearchDiscoveryEnabled:          false,
		SearchDiscoveryMaxTargets:       4,
		SearchDiscoveryMaxURLsPerTarget: 3,
		WikidataCachePath:               "registry/wikidata_cache",
		WikidataCacheTTLHours:           24,
		VettingEnabled:                  false,
		VettingProvider:                 "openai-compatible",
		VettingBaseURL:                  "https://api.openai.com/v1",
		VettingModel:                    "gpt-4.1-mini",
		VettingTemperature:              0,
		VettingMaxSampleItems:           6,
		AlertLLMEnabled:                 false,
		AlertLLMModel:                   "gpt-4.1-mini",
		AlertLLMMaxItemsPerSource:       4,
		CategoryDictionaryPath:          "registry/category_dictionary.json",
		ReplacementQueuePath:            "registry/source_dead_letter.json",
		SourceDBPath:                    "registry/sources.db",
		SourceDBInit:                    false,
		SourceDBImportRegistry:          false,
		SourceDBMergeRegistry:           false,
		SourceDBExportRegistry:          false,
		CuratedSeedPath:                 "registry/curated_agencies.seed.json",
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
	cfg.WikimediaUserAgent = envString("WIKIMEDIA_USER_AGENT", cfg.WikimediaUserAgent)
	cfg.TranslateEnabled = envBool("TRANSLATE_ENABLED", cfg.TranslateEnabled)
	cfg.BrowserEnabled = envBool("BROWSER_ENABLED", cfg.BrowserEnabled)
	cfg.BrowserTimeoutMS = envInt("BROWSER_TIMEOUT_MS", cfg.BrowserTimeoutMS)
	cfg.DiscoverMode = envBool("DISCOVER_MODE", cfg.DiscoverMode)
	cfg.DiscoverBackground = envBool("DISCOVER_BACKGROUND", cfg.DiscoverBackground)
	cfg.DiscoverIntervalMS = envInt("DISCOVER_INTERVAL_MS", cfg.DiscoverIntervalMS)
	cfg.DiscoverOutputPath = envString("DISCOVER_OUTPUT_PATH", cfg.DiscoverOutputPath)
	cfg.CandidateQueuePath = envString("CANDIDATE_QUEUE_PATH", cfg.CandidateQueuePath)
	cfg.SearchDiscoveryEnabled = envBool("SEARCH_DISCOVERY_ENABLED", cfg.SearchDiscoveryEnabled)
	cfg.SearchDiscoveryMaxTargets = envInt("SEARCH_DISCOVERY_MAX_TARGETS", cfg.SearchDiscoveryMaxTargets)
	cfg.SearchDiscoveryMaxURLsPerTarget = envInt("SEARCH_DISCOVERY_MAX_URLS_PER_TARGET", cfg.SearchDiscoveryMaxURLsPerTarget)
	cfg.WikidataCachePath = envString("WIKIDATA_CACHE_PATH", cfg.WikidataCachePath)
	cfg.WikidataCacheTTLHours = envInt("WIKIDATA_CACHE_TTL_HOURS", cfg.WikidataCacheTTLHours)
	cfg.VettingEnabled = envBool("SOURCE_VETTING_ENABLED", cfg.VettingEnabled)
	cfg.VettingProvider = envString("SOURCE_VETTING_PROVIDER", cfg.VettingProvider)
	cfg.VettingBaseURL = envString("SOURCE_VETTING_BASE_URL", cfg.VettingBaseURL)
	cfg.VettingAPIKey = envString("SOURCE_VETTING_API_KEY", cfg.VettingAPIKey)
	cfg.VettingModel = envString("SOURCE_VETTING_MODEL", cfg.VettingModel)
	cfg.VettingTemperature = envFloat("SOURCE_VETTING_TEMPERATURE", cfg.VettingTemperature)
	cfg.VettingMaxSampleItems = envInt("SOURCE_VETTING_MAX_SAMPLE_ITEMS", cfg.VettingMaxSampleItems)
	cfg.AlertLLMEnabled = envBool("ALERT_LLM_ENABLED", cfg.AlertLLMEnabled)
	cfg.AlertLLMModel = envString("ALERT_LLM_MODEL", cfg.AlertLLMModel)
	cfg.AlertLLMMaxItemsPerSource = envInt("ALERT_LLM_MAX_ITEMS_PER_SOURCE", cfg.AlertLLMMaxItemsPerSource)
	cfg.CategoryDictionaryPath = envString("CATEGORY_DICTIONARY_PATH", cfg.CategoryDictionaryPath)
	cfg.ReplacementQueuePath = envString("REPLACEMENT_QUEUE_PATH", cfg.ReplacementQueuePath)
	cfg.SourceDBPath = envString("SOURCE_DB_PATH", cfg.SourceDBPath)
	cfg.SourceDBInit = envBool("SOURCE_DB_INIT", cfg.SourceDBInit)
	cfg.SourceDBImportRegistry = envBool("SOURCE_DB_IMPORT_REGISTRY", cfg.SourceDBImportRegistry)
	cfg.SourceDBMergeRegistry = envBool("SOURCE_DB_MERGE_REGISTRY", cfg.SourceDBMergeRegistry)
	cfg.SourceDBExportRegistry = envBool("SOURCE_DB_EXPORT_REGISTRY", cfg.SourceDBExportRegistry)
	cfg.CuratedSeedPath = envString("CURATED_SEED_PATH", cfg.CuratedSeedPath)
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
