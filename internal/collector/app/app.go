// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/scalytics/euosint/internal/collector/api"
	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/discover"
	"github.com/scalytics/euosint/internal/collector/run"
	"github.com/scalytics/euosint/internal/sourcedb"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	stdout = newTimestampWriter(stdout)
	stderr = newTimestampWriter(stderr)

	fs := flag.NewFlagSet("euosint-collector", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := config.FromEnv()
	fs.StringVar(&cfg.RegistryPath, "registry", cfg.RegistryPath, "Path to the collector source registry JSON file")
	fs.StringVar(&cfg.OutputPath, "output", cfg.OutputPath, "Path for active alerts output JSON file")
	fs.StringVar(&cfg.FilteredOutputPath, "filtered-output", cfg.FilteredOutputPath, "Path for filtered alerts output JSON file")
	fs.StringVar(&cfg.StateOutputPath, "state-output", cfg.StateOutputPath, "Path for collector state JSON file")
	fs.StringVar(&cfg.SourceHealthOutputPath, "source-health-output", cfg.SourceHealthOutputPath, "Path for source health JSON file")
	fs.BoolVar(&cfg.Watch, "watch", cfg.Watch, "Run continuously with the configured interval")
	fs.IntVar(&cfg.IntervalMS, "interval-ms", cfg.IntervalMS, "Polling interval in milliseconds when watch mode is enabled")
	fs.IntVar(&cfg.MaxPerSource, "max-per-source", cfg.MaxPerSource, "Maximum items retained per source fetch")
	fs.IntVar(&cfg.MaxAgeDays, "max-age-days", cfg.MaxAgeDays, "Maximum item age in days")
	fs.IntVar(&cfg.RemovedRetentionDays, "removed-retention-days", cfg.RemovedRetentionDays, "Retention in days for removed alerts")
	fs.IntVar(&cfg.AlertCooldownHours, "alert-cooldown-hours", cfg.AlertCooldownHours, "Hours to keep missing alerts in cooldown before stale")
	fs.IntVar(&cfg.AlertStaleDays, "alert-stale-days", cfg.AlertStaleDays, "Days to keep missing alerts in stale before archive")
	fs.IntVar(&cfg.AlertArchiveDays, "alert-archive-days", cfg.AlertArchiveDays, "Days to keep archived alerts in state history")
	fs.IntVar(&cfg.RecentWindowPerSource, "recent-window-per-source", cfg.RecentWindowPerSource, "Rolling max items per non-HTML source fetch")
	fs.IntVar(&cfg.HTMLScrapeIntervalHours, "html-scrape-interval-hours", cfg.HTMLScrapeIntervalHours, "Minimum hours between successful HTML full scrapes")
	fs.Float64Var(&cfg.IncidentRelevanceThreshold, "incident-threshold", cfg.IncidentRelevanceThreshold, "Default relevance threshold for active alerts")
	fs.Float64Var(&cfg.MissingPersonRelevanceThreshold, "missing-person-threshold", cfg.MissingPersonRelevanceThreshold, "Relevance threshold for missing person alerts")
	fs.BoolVar(&cfg.FailOnCriticalSourceGap, "fail-on-critical-source-gap", cfg.FailOnCriticalSourceGap, "Fail the run when critical sources fetch zero records")
	fs.BoolVar(&cfg.TranslateEnabled, "translate", cfg.TranslateEnabled, "Translate non-Latin RSS titles and summaries to English")
	fs.BoolVar(&cfg.BrowserEnabled, "browser", cfg.BrowserEnabled, "Enable headless Chrome fetching for browser-mode sources")
	fs.IntVar(&cfg.BrowserTimeoutMS, "browser-timeout-ms", cfg.BrowserTimeoutMS, "Timeout in milliseconds for headless Chrome page loads")
	fs.BoolVar(&cfg.DiscoverMode, "discover", cfg.DiscoverMode, "Run source discovery instead of collection")
	fs.BoolVar(&cfg.DiscoverBackground, "discover-background", cfg.DiscoverBackground, "Run source discovery in the background while the collector is watching feeds")
	fs.IntVar(&cfg.DiscoverIntervalMS, "discover-interval-ms", cfg.DiscoverIntervalMS, "Background discovery interval in milliseconds")
	fs.StringVar(&cfg.DiscoverOutputPath, "discover-output", cfg.DiscoverOutputPath, "Path for discovery results JSON file")
	fs.StringVar(&cfg.CandidateQueuePath, "candidate-queue", cfg.CandidateQueuePath, "Path for the crawler candidate intake JSON file")
	fs.StringVar(&cfg.SovereignSeedPath, "sovereign-seed", cfg.SovereignSeedPath, "Path for curated sovereign official-statements candidate seeds")
	fs.BoolVar(&cfg.SearchDiscoveryEnabled, "search-discovery", cfg.SearchDiscoveryEnabled, "Use the configured OpenAI-compatible model as a token-safe candidate URL discovery accelerator")
	fs.IntVar(&cfg.SearchDiscoveryMaxTargets, "search-discovery-max-targets", cfg.SearchDiscoveryMaxTargets, "Maximum number of search-discovery targets per run")
	fs.IntVar(&cfg.SearchDiscoveryMaxURLsPerTarget, "search-discovery-max-urls", cfg.SearchDiscoveryMaxURLsPerTarget, "Maximum number of URLs requested from the model per search target")
	fs.StringVar(&cfg.WikimediaUserAgent, "wikimedia-user-agent", cfg.WikimediaUserAgent, "Identifying bot User-Agent for Wikimedia/Wikidata requests")
	fs.StringVar(&cfg.WikidataCachePath, "wikidata-cache-path", cfg.WikidataCachePath, "Directory for cached Wikidata discovery responses")
	fs.IntVar(&cfg.WikidataCacheTTLHours, "wikidata-cache-ttl-hours", cfg.WikidataCacheTTLHours, "How long to reuse cached Wikidata discovery responses")
	fs.IntVar(&cfg.StructuredDiscoveryIntervalHours, "structured-discovery-interval-hours", cfg.StructuredDiscoveryIntervalHours, "Minimum hours between FIRST/Wikidata structured discovery seeding runs")
	fs.BoolVar(&cfg.VettingEnabled, "source-vetting", cfg.VettingEnabled, "Enable LLM-assisted source vetting and promotion for discovered candidates")
	fs.BoolVar(&cfg.SourceVettingRequired, "source-vetting-required", cfg.SourceVettingRequired, "Require source vetting as a hard promotion gate during discovery")
	fs.Float64Var(&cfg.SourceMinQuality, "source-min-quality", cfg.SourceMinQuality, "Minimum source quality score required for auto-promotion")
	fs.Float64Var(&cfg.SourceMinOperationalRelevance, "source-min-operational-relevance", cfg.SourceMinOperationalRelevance, "Minimum operational relevance score required for auto-promotion")
	fs.Float64Var(&cfg.OfficialStatementsMinQuality, "official-statements-min-quality", cfg.OfficialStatementsMinQuality, "Minimum source quality score required for sovereign official-statement source promotion")
	fs.Float64Var(&cfg.OfficialStatementsMinOperational, "official-statements-min-operational-relevance", cfg.OfficialStatementsMinOperational, "Minimum operational relevance score required for sovereign official-statement source promotion")
	fs.StringVar(&cfg.VettingProvider, "source-vetting-provider", cfg.VettingProvider, "LLM provider label for docs/logging (openai, mistral, xai, claude, gemini, vllm, ollama)")
	fs.StringVar(&cfg.VettingBaseURL, "source-vetting-base-url", cfg.VettingBaseURL, "OpenAI-compatible base URL for source vetting")
	fs.StringVar(&cfg.VettingAPIKey, "source-vetting-api-key", cfg.VettingAPIKey, "API key for the source vetting endpoint")
	fs.StringVar(&cfg.VettingModel, "source-vetting-model", cfg.VettingModel, "Model name for the source vetting endpoint")
	fs.Float64Var(&cfg.VettingTemperature, "source-vetting-temperature", cfg.VettingTemperature, "Temperature for source vetting requests")
	fs.IntVar(&cfg.VettingMaxSampleItems, "source-vetting-max-samples", cfg.VettingMaxSampleItems, "Maximum sample items fetched per discovered source for vetting")
	fs.BoolVar(&cfg.AlertLLMEnabled, "alert-llm", cfg.AlertLLMEnabled, "Enable LLM alert translation and yes/no category gating")
	fs.StringVar(&cfg.AlertLLMModel, "alert-llm-model", cfg.AlertLLMModel, "Model name for LLM alert translation/gating")
	fs.IntVar(&cfg.AlertLLMMaxItemsPerSource, "alert-llm-max-items", cfg.AlertLLMMaxItemsPerSource, "Maximum number of alert items per source sent to the alert LLM in one collector pass")
	fs.Float64Var(&cfg.AlarmRelevanceThreshold, "alarm-threshold", cfg.AlarmRelevanceThreshold, "Minimum relevance score for an alert to be classified into alarm signal lane")
	fs.StringVar(&cfg.ReplacementQueuePath, "replacement-queue", cfg.ReplacementQueuePath, "Path for the dead-source DLQ JSON file")
	fs.StringVar(&cfg.SourceDBPath, "source-db", cfg.SourceDBPath, "Path to the SQLite source database")
	fs.BoolVar(&cfg.SourceDBInit, "source-db-init", cfg.SourceDBInit, "Initialize the SQLite source database schema")
	fs.BoolVar(&cfg.SourceDBImportRegistry, "source-db-import-registry", cfg.SourceDBImportRegistry, "Import the JSON registry into the SQLite source database")
	fs.BoolVar(&cfg.SourceDBMergeRegistry, "source-db-merge-registry", cfg.SourceDBMergeRegistry, "Merge a JSON registry or curated seed into the SQLite source database")
	fs.BoolVar(&cfg.SourceDBExportRegistry, "source-db-export-registry", cfg.SourceDBExportRegistry, "Export the SQLite source database back into the JSON registry")
	fs.StringVar(&cfg.CuratedSeedPath, "curated-seed", cfg.CuratedSeedPath, "Path to the curated agency seed JSON file")
	fs.StringVar(&cfg.RegistrySeedPath, "registry-seed", cfg.RegistrySeedPath, "Path to the baked-in JSON registry for live merge on each cycle")
	fs.BoolVar(&cfg.APIEnabled, "api", cfg.APIEnabled, "Start the search API server alongside the collector")
	fs.StringVar(&cfg.APIAddr, "api-addr", cfg.APIAddr, "Listen address for the search API server")
	fs.StringVar(&cfg.UCDPAccessToken, "ucdp-access-token", cfg.UCDPAccessToken, "UCDP API access token (x-ucdp-access-token)")
	fs.BoolVar(&cfg.MilitaryBasesEnabled, "military-bases-enabled", cfg.MilitaryBasesEnabled, "Enable periodic refresh of the public military-bases GeoJSON layer")
	fs.StringVar(&cfg.MilitaryBasesURL, "military-bases-url", cfg.MilitaryBasesURL, "Source URL for military-bases GeoJSON")
	fs.StringVar(&cfg.MilitaryBasesOutputPath, "military-bases-output", cfg.MilitaryBasesOutputPath, "Output path for military-bases GeoJSON")
	fs.IntVar(&cfg.MilitaryBasesRefreshHours, "military-bases-refresh-hours", cfg.MilitaryBasesRefreshHours, "Refresh cadence in hours for military-bases GeoJSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if cfg.DiscoverMode {
		return discover.Run(ctx, cfg, stdout, stderr)
	}
	if cfg.SourceDBInit || cfg.SourceDBImportRegistry || cfg.SourceDBMergeRegistry || cfg.SourceDBExportRegistry {
		db, err := sourcedb.Open(cfg.SourceDBPath)
		if err != nil {
			return err
		}
		defer db.Close()

		if cfg.SourceDBInit {
			if err := db.Init(ctx); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Initialized source DB schema -> %s\n", cfg.SourceDBPath)
		}
		if cfg.SourceDBImportRegistry {
			if err := db.ImportRegistry(ctx, cfg.RegistryPath); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Imported registry JSON into source DB -> %s\n", cfg.SourceDBPath)
		}
		if cfg.SourceDBMergeRegistry {
			if err := db.MergeRegistry(ctx, cfg.CuratedSeedPath); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Merged curated seed into source DB -> %s\n", cfg.SourceDBPath)
		}
		if cfg.SourceDBExportRegistry {
			if err := db.ExportRegistry(ctx, cfg.RegistryPath); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Exported source DB registry -> %s\n", cfg.RegistryPath)
		}
		return nil
	}

	if cfg.APIEnabled {
		apiDB, err := sourcedb.Open(cfg.RegistryPath)
		if err != nil {
			return fmt.Errorf("open DB for API: %w", err)
		}
		defer apiDB.Close()

		srv := api.New(apiDB, cfg.APIAddr, stderr)
		if err := srv.Start(); err != nil {
			return err
		}
		defer srv.Stop(ctx)
		fmt.Fprintf(stdout, "Search API listening on %s\n", cfg.APIAddr)
	}

	return run.New(stdout, stderr).Run(ctx, cfg)
}
