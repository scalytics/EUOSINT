package app

import (
	"context"
	"flag"
	"io"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/run"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
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
	fs.Float64Var(&cfg.IncidentRelevanceThreshold, "incident-threshold", cfg.IncidentRelevanceThreshold, "Default relevance threshold for active alerts")
	fs.Float64Var(&cfg.MissingPersonRelevanceThreshold, "missing-person-threshold", cfg.MissingPersonRelevanceThreshold, "Relevance threshold for missing person alerts")
	fs.BoolVar(&cfg.FailOnCriticalSourceGap, "fail-on-critical-source-gap", cfg.FailOnCriticalSourceGap, "Fail the run when critical sources fetch zero records")
	fs.BoolVar(&cfg.TranslateEnabled, "translate", cfg.TranslateEnabled, "Translate non-Latin RSS titles and summaries to English")

	if err := fs.Parse(args); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return run.New(stdout, stderr).Run(ctx, cfg)
}
