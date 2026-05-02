// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

const (
	defaultOutputPath        = "public/alerts.json"
	defaultFilteredPath      = "public/alerts-filtered.json"
	defaultStatePath         = "public/alerts-state.json"
	defaultSourceHealthPath  = "public/source-health.json"
	defaultZoneBriefingsPath = "public/zone-briefings.json"
	defaultRegistryPath      = "registry/sources.db"
	defaultTimeoutMS         = 15000
	defaultIntervalMS        = 900000
	defaultMaxPerSource      = 40
	defaultMaxAgeDays        = 180
	defaultRemovedDays       = 14
	defaultMaxBodyBytes      = 2 * 1024 * 1024
	defaultCooldownHours     = 24
	defaultStaleDays         = 14
	defaultArchiveDays       = 90
	defaultMaxFreshnessHours = 72
	defaultRecentPerSource   = 20
	defaultHTMLScrapeHours   = 24
	defaultStopWordsPath     = "registry/stop_words.json"
)

type Config struct {
	RegistryPath                     string
	OutputPath                       string
	FilteredOutputPath               string
	StateOutputPath                  string
	SourceHealthOutputPath           string
	ZoneBriefingsOutputPath          string
	CountryBoundariesPath            string
	MaxPerSource                     int
	MaxAgeDays                       int
	RemovedRetentionDays             int
	AlertCooldownHours               int
	AlertStaleDays                   int
	AlertArchiveDays                 int
	MaxFreshnessHours                int
	RecentWindowPerSource            int
	HTMLScrapeIntervalHours          int
	IncidentRelevanceThreshold       float64
	MissingPersonRelevanceThreshold  float64
	FailOnCriticalSourceGap          bool
	CriticalSourcePrefixes           []string
	Watch                            bool
	IntervalMS                       int
	HTTPTimeoutMS                    int
	FetchTimeoutFastMS               int
	FetchWorkers                     int
	MaxResponseBodyBytes             int64
	UserAgent                        string
	WikimediaUserAgent               string
	TranslateEnabled                 bool
	BrowserEnabled                   bool
	BrowserTimeoutMS                 int
	BrowserWSURL                     string
	BrowserMaxConcurrency            int
	BrowserConnectRetries            int
	BrowserConnectRetryDelayMS       int
	DiscoverMode                     bool
	DiscoverBackground               bool
	DiscoverIntervalMS               int
	DiscoverOutputPath               string
	CandidateQueuePath               string
	SovereignSeedPath                string
	SearchDiscoveryEnabled           bool
	SearchDiscoveryMaxTargets        int
	SearchDiscoveryMaxURLsPerTarget  int
	DDGSearchEnabled                 bool
	DDGSearchMaxQueries              int
	DDGSearchDelayMS                 int
	DiscoverSocialEnabled            bool
	DiscoverSocialMaxTargets         int
	WikidataCachePath                string
	WikidataCacheTTLHours            int
	StructuredDiscoveryIntervalHours int
	VettingEnabled                   bool
	SourceVettingRequired            bool
	SourceMinQuality                 float64
	SourceMinOperationalRelevance    float64
	OfficialStatementsMinQuality     float64
	OfficialStatementsMinOperational float64
	VettingProvider                  string
	VettingBaseURL                   string
	VettingAPIKey                    string
	VettingModel                     string
	VettingTemperature               float64
	VettingTimeoutMS                 int
	VettingMaxSampleItems            int
	AlertLLMEnabled                  bool
	AlertLLMModel                    string
	AlertLLMMaxItemsPerSource        int
	AlarmRelevanceThreshold          float64
	CategoryDictionaryPath           string
	ReplacementQueuePath             string
	SourceDBPath                     string
	SourceDBInit                     bool
	SourceDBImportRegistry           bool
	SourceDBMergeRegistry            bool
	SourceDBExportRegistry           bool
	CuratedSeedPath                  string
	RegistrySeedPath                 string
	CursorsPath                      string
	GeoNamesPath                     string
	NominatimBaseURL                 string
	NominatimEnabled                 bool
	APIEnabled                       bool
	APIAddr                          string
	ACLEDUsername                    string
	ACLEDPassword                    string
	UCDPAccessToken                  string
	StopWordsPath                    string
	StopWords                        []string
	XFetchPauseMS                    int
	NoisePolicyPath                  string
	NoisePolicyBPath                 string
	NoisePolicyBPercent              int
	NoiseMetricsOutputPath           string
	MilitaryBasesEnabled             bool
	MilitaryBasesURL                 string
	MilitaryBasesOutputPath          string
	MilitaryBasesRefreshHours        int
	UCDPAPIVersion                   string
	ZoneBriefingRefreshHours         int
	ZoneBriefingACLEDEnabled         bool
	CollectorRole                    string
	CORSAllowedOrigins               []string
	APIBearerToken                   string
	ResetZoneBriefLLM                bool
	ResetAgentOps                    bool
	PacksDir                         string
	KafkaEnabled                     bool
	KafkaBrokers                     []string
	KafkaTopics                      []string
	KafkaGroupID                     string
	KafkaClientID                    string
	KafkaSecurityProtocol            string
	KafkaSASLMechanism               string
	KafkaUsername                    string
	KafkaPassword                    string
	KafkaTLSInsecureSkipVerify       bool
	KafkaTestOnStart                 bool
	KafkaMaxRecordBytes              int
	KafkaMaxPerCycle                 int
	KafkaPollTimeoutMS               int
	KafkaMapperPath                  string
	AgentOpsEnabled                  bool
	AgentOpsBrokers                  []string
	AgentOpsGroupName                string
	AgentOpsGroupID                  string
	AgentOpsClientID                 string
	AgentOpsTopicMode                string
	AgentOpsTopics                   []string
	AgentOpsSecurityProtocol         string
	AgentOpsSASLMechanism            string
	AgentOpsUsername                 string
	AgentOpsPassword                 string
	AgentOpsTLSInsecureSkipVerify    bool
	AgentOpsPolicyPath               string
	AgentOpsReplayEnabled            bool
	AgentOpsReplayPrefix             string
	AgentOpsRejectTopic              string
	AgentOpsOutputPath               string
	UIMode                           string
	Profile                          string
	UIPolicyPath                     string
}

func Default() Config {
	return Config{
		RegistryPath:                     defaultRegistryPath,
		OutputPath:                       defaultOutputPath,
		FilteredOutputPath:               defaultFilteredPath,
		StateOutputPath:                  defaultStatePath,
		SourceHealthOutputPath:           defaultSourceHealthPath,
		ZoneBriefingsOutputPath:          defaultZoneBriefingsPath,
		CountryBoundariesPath:            "registry/geo/countries-adm0.geojson",
		MaxPerSource:                     defaultMaxPerSource,
		MaxAgeDays:                       defaultMaxAgeDays,
		RemovedRetentionDays:             defaultRemovedDays,
		AlertCooldownHours:               defaultCooldownHours,
		AlertStaleDays:                   defaultStaleDays,
		AlertArchiveDays:                 defaultArchiveDays,
		MaxFreshnessHours:                defaultMaxFreshnessHours,
		RecentWindowPerSource:            defaultRecentPerSource,
		HTMLScrapeIntervalHours:          defaultHTMLScrapeHours,
		IncidentRelevanceThreshold:       0.42,
		MissingPersonRelevanceThreshold:  0,
		FailOnCriticalSourceGap:          false,
		CriticalSourcePrefixes:           []string{"cisa"},
		Watch:                            false,
		IntervalMS:                       defaultIntervalMS,
		HTTPTimeoutMS:                    defaultTimeoutMS,
		FetchTimeoutFastMS:               3000,
		FetchWorkers:                     12,
		MaxResponseBodyBytes:             defaultMaxBodyBytes,
		UserAgent:                        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		WikimediaUserAgent:               "kafSIEMBot/1.0 (https://www.scalytics.io; ops@scalytics.io) WDQS discovery",
		TranslateEnabled:                 true,
		BrowserEnabled:                   false,
		BrowserTimeoutMS:                 30000,
		BrowserWSURL:                     "ws://browser:3000",
		BrowserMaxConcurrency:            4,
		BrowserConnectRetries:            3,
		BrowserConnectRetryDelayMS:       1000,
		DiscoverMode:                     false,
		DiscoverBackground:               true,
		DiscoverIntervalMS:               defaultIntervalMS,
		DiscoverOutputPath:               "discover-results.json",
		CandidateQueuePath:               "registry/source_candidates.json",
		SovereignSeedPath:                "registry/sovereign_official_statements.seed.json",
		SearchDiscoveryEnabled:           false,
		SearchDiscoveryMaxTargets:        40,
		SearchDiscoveryMaxURLsPerTarget:  3,
		DDGSearchEnabled:                 true,
		DDGSearchMaxQueries:              40,
		DDGSearchDelayMS:                 5000,
		DiscoverSocialEnabled:            true,
		DiscoverSocialMaxTargets:         24,
		WikidataCachePath:                "registry/wikidata_cache",
		WikidataCacheTTLHours:            168,
		StructuredDiscoveryIntervalHours: 168,
		VettingEnabled:                   false,
		SourceVettingRequired:            true,
		SourceMinQuality:                 0.6,
		SourceMinOperationalRelevance:    0.6,
		OfficialStatementsMinQuality:     0.75,
		OfficialStatementsMinOperational: 0.7,
		VettingProvider:                  "openai-compatible",
		VettingBaseURL:                   "https://api.openai.com/v1",
		VettingModel:                     "gpt-4.1-mini",
		VettingTemperature:               0,
		VettingTimeoutMS:                 45000,
		VettingMaxSampleItems:            6,
		AlertLLMEnabled:                  false,
		AlertLLMModel:                    "gpt-4.1-mini",
		AlertLLMMaxItemsPerSource:        4,
		AlarmRelevanceThreshold:          0.72,
		CategoryDictionaryPath:           "registry/category_dictionary.json",
		ReplacementQueuePath:             "registry/source_dead_letter.json",
		SourceDBPath:                     "registry/sources.db",
		SourceDBInit:                     false,
		SourceDBImportRegistry:           false,
		SourceDBMergeRegistry:            false,
		SourceDBExportRegistry:           false,
		CuratedSeedPath:                  "registry/curated_agencies.seed.json",
		RegistrySeedPath:                 "registry/source_registry.json",
		CursorsPath:                      "public/cursors.json",
		GeoNamesPath:                     "registry/cities500.txt",
		NominatimBaseURL:                 "https://nominatim.openstreetmap.org",
		NominatimEnabled:                 true,
		APIEnabled:                       false,
		APIAddr:                          ":3001",
		UCDPAccessToken:                  "",
		StopWordsPath:                    defaultStopWordsPath,
		NoisePolicyPath:                  "registry/noise_policy.json",
		NoisePolicyBPath:                 "",
		NoisePolicyBPercent:              0,
		NoiseMetricsOutputPath:           "public/noise-metrics.json",
		MilitaryBasesEnabled:             true,
		MilitaryBasesURL:                 "https://services.arcgis.com/xOi1kZaI0eWDREZv/ArcGIS/rest/services/NTAD_Military_Bases/FeatureServer/0/query?where=1%3D1&outFields=*&outSR=4326&f=geojson",
		MilitaryBasesOutputPath:          "public/geo/military-bases.geojson",
		MilitaryBasesRefreshHours:        168,
		UCDPAPIVersion:                   "26.0.1",
		ZoneBriefingRefreshHours:         24,
		ZoneBriefingACLEDEnabled:         true,
		CollectorRole:                    "all",
		ResetAgentOps:                    false,
		PacksDir:                         "/packs",
		XFetchPauseMS:                    1250,
		KafkaEnabled:                     false,
		KafkaBrokers:                     nil,
		KafkaTopics:                      nil,
		KafkaGroupID:                     "kafsiem-kafka",
		KafkaClientID:                    "kafsiem-collector",
		KafkaSecurityProtocol:            "PLAINTEXT",
		KafkaSASLMechanism:               "PLAIN",
		KafkaUsername:                    "",
		KafkaPassword:                    "",
		KafkaTLSInsecureSkipVerify:       false,
		KafkaTestOnStart:                 true,
		KafkaMaxRecordBytes:              1 << 20,
		KafkaMaxPerCycle:                 500,
		KafkaPollTimeoutMS:               2000,
		KafkaMapperPath:                  "registry/kafka_mapper.json",
		AgentOpsEnabled:                  false,
		AgentOpsBrokers:                  nil,
		AgentOpsGroupName:                "",
		AgentOpsGroupID:                  "kafsiem-agentops",
		AgentOpsClientID:                 "kafsiem-agentops",
		AgentOpsTopicMode:                "auto",
		AgentOpsTopics:                   nil,
		AgentOpsSecurityProtocol:         "PLAINTEXT",
		AgentOpsSASLMechanism:            "PLAIN",
		AgentOpsUsername:                 "",
		AgentOpsPassword:                 "",
		AgentOpsTLSInsecureSkipVerify:    false,
		AgentOpsPolicyPath:               "/config/agentops_policy.yaml",
		AgentOpsReplayEnabled:            true,
		AgentOpsReplayPrefix:             "kafsiem-agentops-replay",
		AgentOpsRejectTopic:              "",
		AgentOpsOutputPath:               "public/agentops.db",
		UIMode:                           "OSINT",
		Profile:                          "osint-default",
		UIPolicyPath:                     "/config/ui_policy.yaml",
	}
}

func FromEnv() Config {
	cfg := Default()
	cfg.OutputPath = envString("OUTPUT_PATH", cfg.OutputPath)
	cfg.FilteredOutputPath = envString("FILTERED_OUTPUT_PATH", cfg.FilteredOutputPath)
	cfg.StateOutputPath = envString("STATE_OUTPUT_PATH", cfg.StateOutputPath)
	cfg.SourceHealthOutputPath = envString("SOURCE_HEALTH_OUTPUT_PATH", cfg.SourceHealthOutputPath)
	cfg.ZoneBriefingsOutputPath = envString("ZONE_BRIEFINGS_OUTPUT_PATH", cfg.ZoneBriefingsOutputPath)
	cfg.CountryBoundariesPath = envString("COUNTRY_BOUNDARIES_PATH", cfg.CountryBoundariesPath)
	cfg.RegistryPath = envString("SOURCE_REGISTRY_PATH", cfg.RegistryPath)
	cfg.MaxPerSource = envInt("MAX_PER_SOURCE", cfg.MaxPerSource)
	cfg.MaxAgeDays = envInt("MAX_AGE_DAYS", cfg.MaxAgeDays)
	cfg.RemovedRetentionDays = envInt("REMOVED_RETENTION_DAYS", cfg.RemovedRetentionDays)
	cfg.AlertCooldownHours = envInt("ALERT_COOLDOWN_HOURS", cfg.AlertCooldownHours)
	cfg.AlertStaleDays = envInt("ALERT_STALE_DAYS", cfg.AlertStaleDays)
	cfg.AlertArchiveDays = envInt("ALERT_ARCHIVE_DAYS", cfg.AlertArchiveDays)
	cfg.MaxFreshnessHours = envInt("MAX_FRESHNESS_HOURS", cfg.MaxFreshnessHours)
	cfg.RecentWindowPerSource = envInt("RECENT_WINDOW_PER_SOURCE", cfg.RecentWindowPerSource)
	cfg.HTMLScrapeIntervalHours = envInt("HTML_SCRAPE_INTERVAL_HOURS", cfg.HTMLScrapeIntervalHours)
	cfg.IncidentRelevanceThreshold = envFloat("INCIDENT_RELEVANCE_THRESHOLD", cfg.IncidentRelevanceThreshold)
	cfg.MissingPersonRelevanceThreshold = envFloat("MISSING_PERSON_RELEVANCE_THRESHOLD", cfg.MissingPersonRelevanceThreshold)
	cfg.FailOnCriticalSourceGap = envBool("FAIL_ON_CRITICAL_SOURCE_GAP", cfg.FailOnCriticalSourceGap)
	cfg.CriticalSourcePrefixes = envCSV("CRITICAL_SOURCE_PREFIXES", cfg.CriticalSourcePrefixes)
	cfg.Watch = envBool("WATCH", cfg.Watch)
	cfg.IntervalMS = envInt("INTERVAL_MS", cfg.IntervalMS)
	cfg.HTTPTimeoutMS = envInt("HTTP_TIMEOUT_MS", cfg.HTTPTimeoutMS)
	cfg.FetchTimeoutFastMS = envInt("FETCH_TIMEOUT_FAST_MS", cfg.FetchTimeoutFastMS)
	cfg.FetchWorkers = envInt("FETCH_WORKERS", cfg.FetchWorkers)
	cfg.MaxResponseBodyBytes = int64(envInt("MAX_RESPONSE_BODY_BYTES", int(cfg.MaxResponseBodyBytes)))
	cfg.UserAgent = envString("USER_AGENT", cfg.UserAgent)
	cfg.WikimediaUserAgent = envString("WIKIMEDIA_USER_AGENT", cfg.WikimediaUserAgent)
	cfg.TranslateEnabled = envBool("TRANSLATE_ENABLED", cfg.TranslateEnabled)
	cfg.BrowserEnabled = envBool("BROWSER_ENABLED", cfg.BrowserEnabled)
	cfg.BrowserTimeoutMS = envInt("BROWSER_TIMEOUT_MS", cfg.BrowserTimeoutMS)
	cfg.BrowserWSURL = envString("BROWSER_WS_URL", cfg.BrowserWSURL)
	cfg.BrowserMaxConcurrency = envInt("BROWSER_MAX_CONCURRENCY", cfg.BrowserMaxConcurrency)
	cfg.BrowserConnectRetries = envInt("BROWSER_CONNECT_RETRIES", cfg.BrowserConnectRetries)
	cfg.BrowserConnectRetryDelayMS = envInt("BROWSER_CONNECT_RETRY_DELAY_MS", cfg.BrowserConnectRetryDelayMS)
	cfg.DiscoverMode = envBool("DISCOVER_MODE", cfg.DiscoverMode)
	cfg.DiscoverBackground = envBool("DISCOVER_BACKGROUND", cfg.DiscoverBackground)
	cfg.DiscoverIntervalMS = envInt("DISCOVER_INTERVAL_MS", cfg.DiscoverIntervalMS)
	cfg.DiscoverOutputPath = envString("DISCOVER_OUTPUT_PATH", cfg.DiscoverOutputPath)
	cfg.CandidateQueuePath = envString("CANDIDATE_QUEUE_PATH", cfg.CandidateQueuePath)
	cfg.SovereignSeedPath = envString("SOVEREIGN_SEED_PATH", cfg.SovereignSeedPath)
	cfg.SearchDiscoveryEnabled = envBool("SEARCH_DISCOVERY_ENABLED", cfg.SearchDiscoveryEnabled)
	cfg.SearchDiscoveryMaxTargets = envInt("SEARCH_DISCOVERY_MAX_TARGETS", cfg.SearchDiscoveryMaxTargets)
	cfg.SearchDiscoveryMaxURLsPerTarget = envInt("SEARCH_DISCOVERY_MAX_URLS_PER_TARGET", cfg.SearchDiscoveryMaxURLsPerTarget)
	cfg.WikidataCachePath = envString("WIKIDATA_CACHE_PATH", cfg.WikidataCachePath)
	cfg.WikidataCacheTTLHours = envInt("WIKIDATA_CACHE_TTL_HOURS", cfg.WikidataCacheTTLHours)
	cfg.StructuredDiscoveryIntervalHours = envInt("STRUCTURED_DISCOVERY_INTERVAL_HOURS", cfg.StructuredDiscoveryIntervalHours)
	cfg.VettingEnabled = envBool("SOURCE_VETTING_ENABLED", cfg.VettingEnabled)
	cfg.SourceVettingRequired = envBool("SOURCE_VETTING_REQUIRED", cfg.SourceVettingRequired)
	cfg.SourceMinQuality = envFloat("SOURCE_MIN_QUALITY", cfg.SourceMinQuality)
	cfg.SourceMinOperationalRelevance = envFloat("SOURCE_MIN_OPERATIONAL_RELEVANCE", cfg.SourceMinOperationalRelevance)
	cfg.OfficialStatementsMinQuality = envFloat("OFFICIAL_STATEMENTS_MIN_QUALITY", cfg.OfficialStatementsMinQuality)
	cfg.OfficialStatementsMinOperational = envFloat("OFFICIAL_STATEMENTS_MIN_OPERATIONAL_RELEVANCE", cfg.OfficialStatementsMinOperational)
	cfg.VettingProvider = envString("SOURCE_VETTING_PROVIDER", cfg.VettingProvider)
	cfg.VettingBaseURL = envString("SOURCE_VETTING_BASE_URL", cfg.VettingBaseURL)
	cfg.VettingAPIKey = envString("SOURCE_VETTING_API_KEY", cfg.VettingAPIKey)
	cfg.VettingModel = envString("SOURCE_VETTING_MODEL", cfg.VettingModel)
	cfg.VettingTemperature = envFloat("SOURCE_VETTING_TEMPERATURE", cfg.VettingTemperature)
	cfg.VettingTimeoutMS = envInt("SOURCE_VETTING_TIMEOUT_MS", cfg.VettingTimeoutMS)
	cfg.VettingMaxSampleItems = envInt("SOURCE_VETTING_MAX_SAMPLE_ITEMS", cfg.VettingMaxSampleItems)
	cfg.AlertLLMEnabled = envBool("ALERT_LLM_ENABLED", cfg.AlertLLMEnabled)
	cfg.AlertLLMModel = envString("ALERT_LLM_MODEL", cfg.AlertLLMModel)
	cfg.AlertLLMMaxItemsPerSource = envInt("ALERT_LLM_MAX_ITEMS_PER_SOURCE", cfg.AlertLLMMaxItemsPerSource)
	cfg.AlarmRelevanceThreshold = envFloat("ALARM_RELEVANCE_THRESHOLD", cfg.AlarmRelevanceThreshold)
	cfg.CategoryDictionaryPath = envString("CATEGORY_DICTIONARY_PATH", cfg.CategoryDictionaryPath)
	cfg.ReplacementQueuePath = envString("REPLACEMENT_QUEUE_PATH", cfg.ReplacementQueuePath)
	cfg.SourceDBPath = envString("SOURCE_DB_PATH", cfg.SourceDBPath)
	cfg.SourceDBInit = envBool("SOURCE_DB_INIT", cfg.SourceDBInit)
	cfg.SourceDBImportRegistry = envBool("SOURCE_DB_IMPORT_REGISTRY", cfg.SourceDBImportRegistry)
	cfg.SourceDBMergeRegistry = envBool("SOURCE_DB_MERGE_REGISTRY", cfg.SourceDBMergeRegistry)
	cfg.SourceDBExportRegistry = envBool("SOURCE_DB_EXPORT_REGISTRY", cfg.SourceDBExportRegistry)
	cfg.CuratedSeedPath = envString("CURATED_SEED_PATH", cfg.CuratedSeedPath)
	cfg.RegistrySeedPath = envString("REGISTRY_SEED_PATH", cfg.RegistrySeedPath)
	cfg.CursorsPath = envString("CURSORS_PATH", cfg.CursorsPath)
	cfg.DDGSearchEnabled = envBool("DDG_SEARCH_ENABLED", cfg.DDGSearchEnabled)
	cfg.DDGSearchMaxQueries = envInt("DDG_SEARCH_MAX_QUERIES", cfg.DDGSearchMaxQueries)
	cfg.DDGSearchDelayMS = envInt("DDG_SEARCH_DELAY_MS", cfg.DDGSearchDelayMS)
	cfg.DiscoverSocialEnabled = envBool("DISCOVER_SOCIAL_ENABLED", cfg.DiscoverSocialEnabled)
	cfg.DiscoverSocialMaxTargets = envInt("DISCOVER_SOCIAL_MAX_TARGETS", cfg.DiscoverSocialMaxTargets)
	cfg.GeoNamesPath = envString("GEONAMES_PATH", cfg.GeoNamesPath)
	cfg.NominatimBaseURL = envString("NOMINATIM_BASE_URL", cfg.NominatimBaseURL)
	cfg.NominatimEnabled = envBool("NOMINATIM_ENABLED", cfg.NominatimEnabled)
	cfg.APIEnabled = envBool("API_ENABLED", cfg.APIEnabled)
	cfg.APIAddr = envString("API_ADDR", cfg.APIAddr)
	cfg.ACLEDUsername = envString("ACLED_USERNAME", cfg.ACLEDUsername)
	cfg.ACLEDPassword = envString("ACLED_PASSWORD", cfg.ACLEDPassword)
	cfg.UCDPAccessToken = envString("UCDP_ACCESS_TOKEN", cfg.UCDPAccessToken)
	cfg.StopWordsPath = envString("STOP_WORDS_PATH", cfg.StopWordsPath)
	cfg.StopWords = loadStopWords(cfg.StopWordsPath)
	if extra := envCSV("STOP_WORDS", nil); len(extra) > 0 {
		cfg.StopWords = append(cfg.StopWords, extra...)
	}
	cfg.XFetchPauseMS = envInt("X_FETCH_PAUSE_MS", cfg.XFetchPauseMS)
	cfg.NoisePolicyPath = envString("NOISE_POLICY_PATH", cfg.NoisePolicyPath)
	cfg.NoisePolicyBPath = envString("NOISE_POLICY_B_PATH", cfg.NoisePolicyBPath)
	cfg.NoisePolicyBPercent = envInt("NOISE_POLICY_B_PERCENT", cfg.NoisePolicyBPercent)
	cfg.NoiseMetricsOutputPath = envString("NOISE_METRICS_OUTPUT_PATH", cfg.NoiseMetricsOutputPath)
	cfg.MilitaryBasesEnabled = envBool("MILITARY_BASES_ENABLED", cfg.MilitaryBasesEnabled)
	cfg.MilitaryBasesURL = envString("MILITARY_BASES_URL", cfg.MilitaryBasesURL)
	cfg.MilitaryBasesOutputPath = envString("MILITARY_BASES_OUTPUT_PATH", cfg.MilitaryBasesOutputPath)
	cfg.MilitaryBasesRefreshHours = envInt("MILITARY_BASES_REFRESH_HOURS", cfg.MilitaryBasesRefreshHours)
	cfg.UCDPAPIVersion = envString("UCDP_API_VERSION", cfg.UCDPAPIVersion)
	cfg.ZoneBriefingRefreshHours = envInt("ZONE_BRIEFING_REFRESH_HOURS", cfg.ZoneBriefingRefreshHours)
	cfg.ZoneBriefingACLEDEnabled = envBool("ZONE_BRIEFING_ACLED_ENABLED", cfg.ZoneBriefingACLEDEnabled)
	cfg.CollectorRole = strings.ToLower(strings.TrimSpace(envString("COLLECTOR_ROLE", cfg.CollectorRole)))
	cfg.CORSAllowedOrigins = envCSV("CORS_ALLOWED_ORIGINS", cfg.CORSAllowedOrigins)
	cfg.APIBearerToken = envString("API_BEARER_TOKEN", cfg.APIBearerToken)
	cfg.ResetZoneBriefLLM = envBool("RESET_ZONE_BRIEF_LLM", cfg.ResetZoneBriefLLM)
	cfg.ResetAgentOps = envBool("RESET_AGENTOPS", cfg.ResetAgentOps)
	cfg.PacksDir = envString("KAFSIEM_PACKS_DIR", cfg.PacksDir)
	cfg.KafkaEnabled = envBool("KAFKA_ENABLED", cfg.KafkaEnabled)
	cfg.KafkaBrokers = envCSV("KAFKA_BROKERS", cfg.KafkaBrokers)
	cfg.KafkaTopics = envCSV("KAFKA_TOPICS", cfg.KafkaTopics)
	cfg.KafkaGroupID = envString("KAFKA_GROUP_ID", cfg.KafkaGroupID)
	cfg.KafkaClientID = envString("KAFKA_CLIENT_ID", cfg.KafkaClientID)
	cfg.KafkaSecurityProtocol = strings.ToUpper(strings.TrimSpace(envString("KAFKA_SECURITY_PROTOCOL", cfg.KafkaSecurityProtocol)))
	cfg.KafkaSASLMechanism = strings.ToUpper(strings.TrimSpace(envString("KAFKA_SASL_MECHANISM", cfg.KafkaSASLMechanism)))
	cfg.KafkaUsername = envString("KAFKA_USERNAME", cfg.KafkaUsername)
	cfg.KafkaPassword = envString("KAFKA_PASSWORD", cfg.KafkaPassword)
	cfg.KafkaTLSInsecureSkipVerify = envBool("KAFKA_TLS_INSECURE_SKIP_VERIFY", cfg.KafkaTLSInsecureSkipVerify)
	cfg.KafkaTestOnStart = envBool("KAFKA_TEST_ON_START", cfg.KafkaTestOnStart)
	cfg.KafkaMaxRecordBytes = envInt("KAFKA_MAX_RECORD_BYTES", cfg.KafkaMaxRecordBytes)
	cfg.KafkaMaxPerCycle = envInt("KAFKA_MAX_PER_CYCLE", cfg.KafkaMaxPerCycle)
	cfg.KafkaPollTimeoutMS = envInt("KAFKA_POLL_TIMEOUT_MS", cfg.KafkaPollTimeoutMS)
	cfg.KafkaMapperPath = envString("KAFKA_MAPPER_PATH", cfg.KafkaMapperPath)
	cfg.AgentOpsEnabled = envBool("AGENTOPS_ENABLED", cfg.AgentOpsEnabled)
	cfg.AgentOpsBrokers = envCSV("AGENTOPS_BROKERS", cfg.AgentOpsBrokers)
	cfg.AgentOpsGroupName = envString("AGENTOPS_GROUP_NAME", cfg.AgentOpsGroupName)
	cfg.AgentOpsGroupID = envString("AGENTOPS_GROUP_ID", cfg.AgentOpsGroupID)
	cfg.AgentOpsClientID = envString("AGENTOPS_CLIENT_ID", cfg.AgentOpsClientID)
	cfg.AgentOpsTopicMode = normalizeTopicMode(envString("AGENTOPS_TOPIC_MODE", cfg.AgentOpsTopicMode))
	cfg.AgentOpsTopics = envCSV("AGENTOPS_TOPICS", cfg.AgentOpsTopics)
	cfg.AgentOpsSecurityProtocol = strings.ToUpper(strings.TrimSpace(envString("AGENTOPS_SECURITY_PROTOCOL", cfg.AgentOpsSecurityProtocol)))
	cfg.AgentOpsSASLMechanism = strings.ToUpper(strings.TrimSpace(envString("AGENTOPS_SASL_MECHANISM", cfg.AgentOpsSASLMechanism)))
	cfg.AgentOpsUsername = envString("AGENTOPS_USERNAME", cfg.AgentOpsUsername)
	cfg.AgentOpsPassword = envString("AGENTOPS_PASSWORD", cfg.AgentOpsPassword)
	cfg.AgentOpsTLSInsecureSkipVerify = envBool("AGENTOPS_TLS_INSECURE_SKIP_VERIFY", cfg.AgentOpsTLSInsecureSkipVerify)
	cfg.AgentOpsPolicyPath = envString("AGENTOPS_POLICY_PATH", cfg.AgentOpsPolicyPath)
	cfg.AgentOpsReplayEnabled = envBool("AGENTOPS_REPLAY_ENABLED", cfg.AgentOpsReplayEnabled)
	cfg.AgentOpsReplayPrefix = envString("AGENTOPS_REPLAY_PREFIX", cfg.AgentOpsReplayPrefix)
	cfg.AgentOpsRejectTopic = envString("AGENTOPS_REJECT_TOPIC", defaultAgentOpsRejectTopic(cfg.AgentOpsGroupName))
	cfg.AgentOpsOutputPath = envString("AGENTOPS_OUTPUT_PATH", cfg.AgentOpsOutputPath)
	cfg.UIMode = normalizeUIMode(envString("UI_MODE", cfg.UIMode))
	cfg.Profile = normalizeProfile(envString("PROFILE", cfg.Profile))
	cfg.UIPolicyPath = envString("UI_POLICY_PATH", cfg.UIPolicyPath)
	return cfg
}

func defaultAgentOpsRejectTopic(groupName string) string {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return ""
	}
	return "group." + groupName + ".agentops.rejects"
}

func normalizeTopicMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return "auto"
	case "manual":
		return "manual"
	default:
		return "auto"
	}
}

func normalizeUIMode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "OSINT":
		return "OSINT"
	case "AGENTOPS", "HYBRID":
		return strings.ToUpper(strings.TrimSpace(raw))
	default:
		return "OSINT"
	}
}

func normalizeProfile(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "osint-default":
		return "osint-default"
	case "agentops-default", "hybrid-ops":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "osint-default"
	}
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

func loadStopWords(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		StopWords []string `json:"stop_words"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := make([]string, 0, len(doc.StopWords))
	for _, w := range doc.StopWords {
		w = strings.TrimSpace(w)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}
