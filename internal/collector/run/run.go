// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/dictionary"
	"github.com/scalytics/euosint/internal/collector/discover"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/normalize"
	"github.com/scalytics/euosint/internal/collector/output"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/collector/registry"
	"github.com/scalytics/euosint/internal/collector/state"
	"github.com/scalytics/euosint/internal/collector/translate"
	"github.com/scalytics/euosint/internal/collector/trends"
	"github.com/scalytics/euosint/internal/collector/vet"
	"github.com/scalytics/euosint/internal/sourcedb"
)

type Runner struct {
	stdout         io.Writer
	stderr         io.Writer
	clientFactory  func(config.Config) *fetch.Client
	browserFactory func(config.Config) (*fetch.BrowserClient, error)
}

func New(stdout io.Writer, stderr io.Writer) Runner {
	return Runner{
		stdout:        stdout,
		stderr:        stderr,
		clientFactory: fetch.New,
		browserFactory: func(cfg config.Config) (*fetch.BrowserClient, error) {
			return fetch.NewBrowser(cfg.BrowserTimeoutMS)
		},
	}
}

func (r Runner) Run(ctx context.Context, cfg config.Config) error {
	if cfg.Watch {
		return r.watch(ctx, cfg)
	}
	return r.runOnce(ctx, cfg)
}

func (r Runner) watch(ctx context.Context, cfg config.Config) error {
	ticker := time.NewTicker(time.Duration(cfg.IntervalMS) * time.Millisecond)
	defer ticker.Stop()
	discoveryStarted := false

	for {
		if err := r.runOnce(ctx, cfg); err != nil {
			fmt.Fprintf(r.stderr, "collector run failed: %v\n", err)
		}
		if cfg.DiscoverBackground && !discoveryStarted {
			go r.runDiscoveryLoop(ctx, cfg)
			discoveryStarted = true
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r Runner) runOnce(ctx context.Context, cfg config.Config) error {
	// Live-merge the baked-in JSON registry into SQLite every cycle.
	// This picks up new sources and syncs rejected status without restart.
	if cfg.RegistrySeedPath != "" && isSQLitePath(cfg.RegistryPath) {
		if err := r.mergeRegistry(ctx, cfg); err != nil {
			fmt.Fprintf(r.stderr, "WARN registry merge: %v\n", err)
		}
	}

	sources, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		return err
	}
	sources = prioritizeSources(sources)
	client := r.clientFactory(cfg)

	var browser *fetch.BrowserClient
	if cfg.BrowserEnabled && r.browserFactory != nil {
		b, err := r.browserFactory(cfg)
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN browser init failed (falling back to stealth): %v\n", err)
		} else {
			browser = b
			defer browser.Close()
		}
	}

	now := time.Now().UTC()
	geocoder := r.initGeocoder(ctx, cfg)
	nctx := normalize.Context{Config: cfg, Now: now, Geocoder: geocoder}
	categoryDictionary, err := dictionary.Load(cfg.CategoryDictionaryPath)
	if err != nil {
		fmt.Fprintf(r.stderr, "WARN category dictionary load failed (falling back to legacy filters): %v\n", err)
	}

	cursors := state.ReadCursors(cfg.CursorsPath)
	dlq := state.ReadDLQ(cfg.ReplacementQueuePath)

	// Load previous alerts early so progress snapshots include them.
	// This prevents the dashboard from going blank during a sweep.
	previousAlerts, err := loadPreviousAlerts(ctx, cfg)
	if err != nil {
		fmt.Fprintf(r.stderr, "WARN previous alerts load failed: %v\n", err)
	}

	// Split sources into fast (RSS/JSON — parallel) and slow (browser/HTML — sequential).
	var fastSources, slowSources []model.RegistrySource
	for _, s := range sources {
		if needsBrowser(s) {
			slowSources = append(slowSources, s)
		} else {
			fastSources = append(fastSources, s)
		}
	}

	alerts := []model.Alert{normalize.StaticInterpolEntry(now)}
	sourceHealth := make([]model.SourceHealthEntry, 0, len(sources))
	var mu sync.Mutex
	completed := 0

	// Fast parallel pass — RSS/JSON feeds with short timeout.
	fastCfg := cfg
	fastCfg.HTTPTimeoutMS = cfg.FetchTimeoutFastMS
	if fastCfg.HTTPTimeoutMS <= 0 {
		fastCfg.HTTPTimeoutMS = 3000
	}
	fastClient := r.clientFactory(fastCfg)
	workers := cfg.FetchWorkers
	if workers <= 0 {
		workers = 12
	}
	if workers > len(fastSources) && len(fastSources) > 0 {
		workers = len(fastSources)
	}

	if len(fastSources) > 0 {
		work := make(chan model.RegistrySource, len(fastSources))
		for _, s := range fastSources {
			work <- s
		}
		close(work)

		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for source := range work {
					if ctx.Err() != nil {
						return
					}
					if dlq.ShouldSkip(source.Source.SourceID, now) {
						mu.Lock()
						sourceHealth = append(sourceHealth, model.SourceHealthEntry{
							SourceID:      source.Source.SourceID,
							AuthorityName: source.Source.AuthorityName,
							Type:          source.Type,
							FeedURL:       source.FeedURL,
							Status:        "skipped",
							Error:         "dead letter queue",
							ErrorClass:    "dlq",
						})
						completed++
						mu.Unlock()
						continue
					}
					batch, entry := r.fetchOneSource(ctx, fastClient, nil, nctx, source, categoryDictionary, cursors)
					mu.Lock()
					sourceHealth = append(sourceHealth, entry)
					alerts = append(alerts, batch...)
					completed++
					if entry.Status == "error" && entry.DiscoveryAction == "dead_letter" {
						dlq.Add(buildDLQEntry(entry, source))
					} else if entry.Status == "error" && dlq.DueForRetry(source.Source.SourceID, now) {
						dlq.UpdateAttempt(source.Source.SourceID, now)
					} else if entry.Status != "error" {
						dlq.Remove(source.Source.SourceID)
					}
					if completed%25 == 0 || completed == 1 {
						r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth)
					}
					mu.Unlock()
					if entry.Status == "error" && entry.DiscoveryAction == "dead_letter" {
						fmt.Fprintf(r.stderr, "WARN %s: %s (added to DLQ)\n", source.Source.AuthorityName, entry.Error)
					} else if entry.Status == "error" {
						fmt.Fprintf(r.stderr, "WARN %s: %s\n", source.Source.AuthorityName, entry.Error)
					}
				}
			}()
		}
		wg.Wait()
		// Snapshot after fast pass completes.
		r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth)
	}

	// Slow sequential pass — browser/HTML sources with full timeout.
	for _, source := range slowSources {
		if ctx.Err() != nil {
			break
		}
		if dlq.ShouldSkip(source.Source.SourceID, now) {
			sourceHealth = append(sourceHealth, model.SourceHealthEntry{
				SourceID:      source.Source.SourceID,
				AuthorityName: source.Source.AuthorityName,
				Type:          source.Type,
				FeedURL:       source.FeedURL,
				Status:        "skipped",
				Error:         "dead letter queue",
				ErrorClass:    "dlq",
			})
			completed++
			continue
		}
		batch, entry := r.fetchOneSourceSlow(ctx, client, browser, nctx, source, categoryDictionary, cursors)
		sourceHealth = append(sourceHealth, entry)
		alerts = append(alerts, batch...)
		completed++
		if entry.Status == "error" && entry.DiscoveryAction == "dead_letter" {
			dlq.Add(buildDLQEntry(entry, source))
			fmt.Fprintf(r.stderr, "WARN %s: %s (added to DLQ)\n", source.Source.AuthorityName, entry.Error)
		} else if entry.Status == "error" && dlq.DueForRetry(source.Source.SourceID, now) {
			dlq.UpdateAttempt(source.Source.SourceID, now)
			fmt.Fprintf(r.stderr, "WARN %s: %s (still dead, retry in 7d)\n", source.Source.AuthorityName, entry.Error)
		} else if entry.Status == "error" {
			fmt.Fprintf(r.stderr, "WARN %s: %s\n", source.Source.AuthorityName, entry.Error)
		} else {
			dlq.Remove(source.Source.SourceID)
		}
		if completed%25 == 0 {
			r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth)
		}
	}

	if err := state.WriteCursors(cfg.CursorsPath, cursors); err != nil {
		fmt.Fprintf(r.stderr, "WARN failed to save cursors: %v\n", err)
	}
	if err := dlq.Write(cfg.ReplacementQueuePath); err != nil {
		fmt.Fprintf(r.stderr, "WARN failed to save DLQ: %v\n", err)
	}
	if dlq.Len() > 0 {
		fmt.Fprintf(r.stderr, "DLQ: %d dead sources skipped (retry every 7d)\n", dlq.Len())
	}

	deduped, duplicateAudit := normalize.Deduplicate(alerts)
	active, filtered := normalize.FilterActive(cfg, deduped)
	populateSourceHealth(sourceHealth, active, filtered)
	if err := assertCriticalSourceCoverage(cfg, sourceHealth); err != nil {
		return err
	}

	// Purge stale alerts from sources that no longer exist or were rejected.
	// Include source IDs from both the registry and the current fetch batch
	// (covers synthetic alerts like the Interpol hub static entry).
	activeSourceIDs := map[string]struct{}{}
	for _, s := range sources {
		activeSourceIDs[s.Source.SourceID] = struct{}{}
	}
	for _, a := range alerts {
		activeSourceIDs[a.SourceID] = struct{}{}
	}
	previousAlerts = purgeOrphanAlerts(previousAlerts, activeSourceIDs)

	accumulateSources := map[string]bool{}
	for _, s := range sources {
		if s.Accumulate {
			accumulateSources[s.Source.SourceID] = true
		}
	}
	currentActive, currentFiltered, fullState := state.Reconcile(cfg, active, filtered, previousAlerts, now, accumulateSources)
	replacementQueue := dlq.Entries()
	if err := deactivateReplacementSources(ctx, cfg.RegistryPath, replacementQueue); err != nil {
		return err
	}
	if err := saveAlertState(ctx, cfg, fullState); err != nil {
		return err
	}
	// ── BM25 corpus relevance scoring ─────────────────────────────────────
	if boosted, err := r.applyCorpusScores(ctx, cfg, currentActive); err != nil {
		fmt.Fprintf(r.stderr, "WARN corpus scoring: %v\n", err)
	} else {
		currentActive = boosted
	}

	// ── Trend detection: record term frequencies and emit discovery hints ──
	if spikes, err := r.recordTrendsAndDetectSpikes(ctx, cfg, currentActive, now); err != nil {
		fmt.Fprintf(r.stderr, "WARN trend detection: %v\n", err)
	} else if len(spikes) > 0 {
		fmt.Fprintf(r.stderr, "Trend spikes detected: %d terms trending\n", len(spikes))
		hints := trends.BuildHints(spikes)
		if len(hints) > 0 {
			if err := r.queueTrendHints(cfg, hints); err != nil {
				fmt.Fprintf(r.stderr, "WARN trend hint queueing: %v\n", err)
			} else {
				fmt.Fprintf(r.stderr, "Queued %d discovery hints from trend spikes\n", len(hints))
			}
		}
	}

	if err := output.Write(cfg, currentActive, currentFiltered, fullState, sourceHealth, duplicateAudit, replacementQueue); err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdout, "Wrote %d active alerts -> %s (%d filtered in %s)\n", len(currentActive), cfg.OutputPath, len(currentFiltered), cfg.FilteredOutputPath)
	return err
}

func (r Runner) writeProgressSnapshot(cfg config.Config, freshAlerts []model.Alert, previousAlerts []model.Alert, sourceHealth []model.SourceHealthEntry) {
	// Merge fresh alerts with previous state so the dashboard never goes
	// blank during a sweep. Previous alerts that aren't in the fresh batch
	// are carried forward as-is.
	freshByID := make(map[string]struct{}, len(freshAlerts))
	for _, a := range freshAlerts {
		freshByID[a.AlertID] = struct{}{}
	}
	merged := make([]model.Alert, 0, len(freshAlerts)+len(previousAlerts))
	merged = append(merged, freshAlerts...)
	for _, prev := range previousAlerts {
		if _, ok := freshByID[prev.AlertID]; !ok {
			merged = append(merged, prev)
		}
	}
	deduped, duplicateAudit := normalize.Deduplicate(merged)
	active, filtered := normalize.FilterActive(cfg, deduped)
	if err := output.Write(cfg, active, filtered, active, sourceHealth, duplicateAudit, nil); err != nil {
		fmt.Fprintf(r.stderr, "WARN progress snapshot write failed: %v\n", err)
		return
	}
	fmt.Fprintf(r.stdout, "Progress snapshot: %d active alerts (%d fresh + %d previous) after %d sources\n", len(active), len(freshAlerts), len(previousAlerts), len(sourceHealth))
}

func prioritizeSources(sources []model.RegistrySource) []model.RegistrySource {
	if len(sources) < 2 {
		return sources
	}
	out := make([]model.RegistrySource, len(sources))
	copy(out, sources)
	sort.SliceStable(out, func(i, j int) bool {
		a := out[i]
		b := out[j]

		// Explicit preferred_source_rank wins first (lower rank = higher priority).
		aRanked := a.PreferredRank > 0
		bRanked := b.PreferredRank > 0
		if aRanked != bRanked {
			return aRanked
		}
		if aRanked && a.PreferredRank != b.PreferredRank {
			return a.PreferredRank < b.PreferredRank
		}

		// Curated/high-value agencies should be fetched early so startup reaches
		// high-signal alerts before sweeping long-tail candidate feeds.
		if a.Source.IsHighValue != b.Source.IsHighValue {
			return a.Source.IsHighValue
		}
		if a.Source.IsCurated != b.Source.IsCurated {
			return a.Source.IsCurated
		}
		if a.Source.OperationalRelevance != b.Source.OperationalRelevance {
			return a.Source.OperationalRelevance > b.Source.OperationalRelevance
		}
		if a.SourceQuality != b.SourceQuality {
			return a.SourceQuality > b.SourceQuality
		}

		// Stable tiebreakers keep output deterministic.
		aStatus := strings.ToLower(strings.TrimSpace(a.PromotionStatus))
		bStatus := strings.ToLower(strings.TrimSpace(b.PromotionStatus))
		if statusPriority(aStatus) != statusPriority(bStatus) {
			return statusPriority(aStatus) > statusPriority(bStatus)
		}
		if typePriority(a.Type) != typePriority(b.Type) {
			return typePriority(a.Type) > typePriority(b.Type)
		}
		return a.Source.SourceID < b.Source.SourceID
	})
	return out
}

// needsBrowser returns true for source types that require a headless browser
// or need the full HTTP timeout (HTML scraping, Interpol API with WAF, etc.).
func needsBrowser(s model.RegistrySource) bool {
	switch s.Type {
	case "html-list", "telegram":
		return true
	case "interpol-red-json", "interpol-yellow-json":
		return true // Akamai WAF, needs special headers + browser fallback
	}
	return s.FetchMode == "browser"
}

// fetchOneSource fetches a single source with retry logic, returning the
// batch of alerts and a health entry. When customFetcher is nil, the
// defaultClient is used directly.
func (r Runner) fetchOneSource(ctx context.Context, defaultClient *fetch.Client, customFetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store, cursors state.Cursors) ([]model.Alert, model.SourceHealthEntry) {
	startedAt := time.Now().UTC()

	var fetcher fetch.Fetcher
	if customFetcher != nil {
		fetcher = customFetcher
	} else {
		fetcher = defaultClient
	}

	batch, err := r.fetchSource(ctx, fetcher, nil, nctx, source, categoryDictionary, cursors)

	// Retry once for transient errors after a short backoff.
	if err != nil {
		errClass, _, _ := classifySourceError(err)
		if (errClass == "timeout" || errClass == "eof" || errClass == "transient") && ctx.Err() == nil {
			fmt.Fprintf(r.stderr, "RETRY %s (transient %s): %v\n", source.Source.AuthorityName, errClass, err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
			if ctx.Err() == nil {
				batch, err = r.fetchSource(ctx, fetcher, nil, nctx, source, categoryDictionary, cursors)
			}
		}
	}

	entry := model.SourceHealthEntry{
		SourceID:      source.Source.SourceID,
		AuthorityName: source.Source.AuthorityName,
		Type:          source.Type,
		FeedURL:       source.FeedURL,
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
		entry.ErrorClass, entry.NeedsReplacement, entry.DiscoveryAction = classifySourceError(err)
		return nil, entry
	}
	entry.Status = "ok"
	entry.FetchedCount = len(batch)
	return batch, entry
}

// fetchOneSourceSlow fetches a source that needs the browser or full timeout.
// Unlike fetchOneSource, this passes the browser instance through for
// Interpol and browser-mode sources.
func (r Runner) fetchOneSourceSlow(ctx context.Context, client *fetch.Client, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store, cursors state.Cursors) ([]model.Alert, model.SourceHealthEntry) {
	startedAt := time.Now().UTC()
	fetcher := fetch.FetcherFor(source.FetchMode, client, browser)

	batch, err := r.fetchSource(ctx, fetcher, browser, nctx, source, categoryDictionary, cursors)

	if err != nil {
		errClass, _, _ := classifySourceError(err)
		if (errClass == "timeout" || errClass == "eof" || errClass == "transient") && ctx.Err() == nil {
			fmt.Fprintf(r.stderr, "RETRY %s (transient %s): %v\n", source.Source.AuthorityName, errClass, err)
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
			}
			if ctx.Err() == nil {
				batch, err = r.fetchSource(ctx, fetcher, browser, nctx, source, categoryDictionary, cursors)
			}
		}
	}

	entry := model.SourceHealthEntry{
		SourceID:      source.Source.SourceID,
		AuthorityName: source.Source.AuthorityName,
		Type:          source.Type,
		FeedURL:       source.FeedURL,
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
		entry.ErrorClass, entry.NeedsReplacement, entry.DiscoveryAction = classifySourceError(err)
		return nil, entry
	}
	entry.Status = "ok"
	entry.FetchedCount = len(batch)
	return batch, entry
}

func statusPriority(status string) int {
	switch status {
	case "active":
		return 3
	case "promoted":
		return 2
	case "candidate":
		return 1
	default:
		return 0
	}
}

func typePriority(kind string) int {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "kev-json":
		return 5
	case "interpol-red-json", "interpol-yellow-json", "fbi-wanted-json", "travelwarning-json", "travelwarning-atom":
		return 4
	case "rss":
		return 3
	case "html-list", "telegram":
		return 2
	default:
		return 1
	}
}

// purgeOrphanAlerts removes alerts whose source_id is no longer in the
// active registry. This cleans up zombie alerts from rejected or removed
// sources that would otherwise persist in the state file indefinitely.
func purgeOrphanAlerts(alerts []model.Alert, activeSourceIDs map[string]struct{}) []model.Alert {
	out := make([]model.Alert, 0, len(alerts))
	for _, a := range alerts {
		if _, ok := activeSourceIDs[a.SourceID]; ok {
			out = append(out, a)
		}
	}
	return out
}

func (r Runner) fetchSource(ctx context.Context, fetcher fetch.Fetcher, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store, cursors state.Cursors) ([]model.Alert, error) {
	switch source.Type {
	case "rss":
		return r.fetchRSS(ctx, fetcher, nctx, source)
	case "html-list":
		return r.fetchHTML(ctx, fetcher, nctx, source, categoryDictionary)
	case "kev-json":
		return r.fetchKEV(ctx, fetcher, nctx, source)
	case "interpol-red-json", "interpol-yellow-json":
		return r.fetchInterpol(ctx, fetcher, browser, nctx, source, cursors)
	case "fbi-wanted-json":
		return r.fetchFBIWanted(ctx, fetcher, nctx, source)
	case "travelwarning-json":
		return r.fetchTravelWarningJSON(ctx, fetcher, nctx, source)
	case "travelwarning-atom":
		return r.fetchTravelWarningAtom(ctx, fetcher, nctx, source)
	case "telegram":
		return r.fetchTelegram(ctx, fetcher, nctx, source)
	default:
		return nil, fmt.Errorf("unsupported source type %s", source.Type)
	}
}

func (r Runner) fetchRSS(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetchWithFallback(ctx, fetcher, source, "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
	if err != nil {
		return nil, err
	}
	items := parse.ParseFeed(string(body))
	if nctx.Config.TranslateEnabled {
		// translate.Batch requires the stealth HTTP client (not a browser).
		translateClient := r.clientFactory(nctx.Config)
		if translated, err := translate.Batch(ctx, translateClient, items); err == nil {
			items = translated
		} else {
			fmt.Fprintf(r.stderr, "WARN %s: translate batch failed: %v\n", source.Source.AuthorityName, err)
		}
	}
	items = filterFeedKeywords(items, source.IncludeKeywords, source.ExcludeKeywords)
	limit := perSourceLimit(nctx.Config, source)
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Link) == "" {
			continue
		}
		alert := normalize.RSSItem(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	out = downgradeNonActionable(out, source)
	return out, nil
}

func (r Runner) fetchHTML(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store) ([]model.Alert, error) {
	body, finalURL, err := fetchWithFallbackURL(ctx, fetcher, source, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if err != nil {
		return nil, err
	}
	items := parse.ParseHTMLAnchors(string(body), finalURL)
	items = filterKeywords(items, source.IncludeKeywords, source.ExcludeKeywords)
	items = filterCategoryItems(items, source, categoryDictionary)
	limit := perSourceLimit(nctx.Config, source)
	if nctx.Config.AlertLLMEnabled {
		if llmLimit := nctx.Config.AlertLLMMaxItemsPerSource; llmLimit > 0 && llmLimit < limit {
			limit = llmLimit
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]model.Alert, 0, len(items))
	if nctx.Config.AlertLLMEnabled {
		alertLLM := vet.NewClient(config.Config{
			HTTPTimeoutMS:      nctx.Config.HTTPTimeoutMS,
			VettingBaseURL:     nctx.Config.VettingBaseURL,
			VettingAPIKey:      nctx.Config.VettingAPIKey,
			VettingProvider:    nctx.Config.VettingProvider,
			VettingModel:       nctx.Config.AlertLLMModel,
			VettingTemperature: 0,
		})
		classified, err := translate.BatchLLM(ctx, nctx.Config, alertLLM, source.Category, items)
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN %s: alert llm failed: %v\n", source.Source.AuthorityName, err)
		} else {
			for _, classifiedItem := range classified {
				meta := source
				meta.Category = firstNonEmpty(classifiedItem.Category, source.Category)
				alert := normalize.HTMLItem(nctx, meta, classifiedItem.Item)
				if alert != nil {
					out = append(out, *alert)
				}
			}
			out = downgradeNonActionable(out, source)
			return out, nil
		}
	}
	for _, item := range items {
		alert := normalize.HTMLItem(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	out = downgradeNonActionable(out, source)
	return out, nil
}

func (r Runner) fetchTelegram(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, true, "text/html,application/xhtml+xml,*/*;q=0.8")
	if err != nil {
		return nil, err
	}
	channel := extractTelegramChannel(source.FeedURL)
	items := parse.ParseTelegram(string(body), channel)
	items = filterKeywords(items, source.IncludeKeywords, source.ExcludeKeywords)
	limit := perSourceLimit(nctx.Config, source)
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]model.Alert, 0, len(items))
	for _, item := range items {
		alert := normalize.HTMLItem(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	out = downgradeNonActionable(out, source)
	return out, nil
}

func extractTelegramChannel(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/s/")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	return path
}

func (r Runner) fetchKEV(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Vulnerabilities []struct {
			CVEID                   string `json:"cveID"`
			CVEIDAlt                string `json:"cveId"`
			CVE                     string `json:"cve"`
			VulnerabilityName       string `json:"vulnerabilityName"`
			ShortDescription        string `json:"shortDescription"`
			DateAdded               string `json:"dateAdded"`
			KnownRansomwareCampaign bool   `json:"knownRansomwareCampaign"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := []model.Alert{}
	for _, vuln := range doc.Vulnerabilities {
		if len(out) == limit {
			break
		}
		cveID := firstNonEmpty(vuln.CVEID, vuln.CVEIDAlt, vuln.CVE)
		alert := normalize.KEVAlert(nctx, source, cveID, vuln.VulnerabilityName, vuln.ShortDescription, vuln.DateAdded, vuln.KnownRansomwareCampaign)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchInterpol(ctx context.Context, fetcher fetch.Fetcher, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource, cursors state.Cursors) ([]model.Alert, error) {
	limit := perSourceLimit(nctx.Config, source)
	pageSize := 160
	var allNotices []model.Alert
	sid := source.Source.SourceID

	// Interpol's API sits behind Akamai WAF and requires XHR-style headers
	// with Referer/Origin pointing to the Interpol website.
	interpolHeaders := map[string]string{
		"Referer":          "https://www.interpol.int/How-we-work/Notices/View-Notices",
		"Origin":           "https://www.interpol.int",
		"Sec-Fetch-Dest":   "empty",
		"Sec-Fetch-Mode":   "cors",
		"Sec-Fetch-Site":   "same-site",
		"X-Requested-With": "XMLHttpRequest",
	}

	clientFetcher, isClient := fetcher.(*fetch.Client)

	fetchPage := func(page int) ([]model.Alert, error) {
		pageURL := buildInterpolPageURL(source.FeedURL, page, pageSize)
		var body []byte
		var err error
		if isClient {
			body, err = clientFetcher.TextWithHeaders(ctx, pageURL, source.FollowRedirects, "application/json", interpolHeaders)
		} else {
			body, err = fetcher.Text(ctx, pageURL, source.FollowRedirects, "application/json")
		}
		if err != nil {
			return nil, err
		}
		return parseInterpolNotices(nctx, source, body)
	}

	// Always fetch page 1 first to pick up new notices.
	batch, err := fetchPage(1)
	if err != nil {
		if browser != nil {
			fmt.Fprintf(r.stderr, "WARN %s: stealth fetch failed, trying browser fallback: %v\n", source.Source.AuthorityName, err)
			bBody, bErr := fetchInterpolViaBrowser(ctx, browser, source)
			if bErr == nil && len(bBody) > 0 {
				return parseInterpolNotices(nctx, source, bBody)
			}
		}
		return nil, err
	}
	allNotices = append(allNotices, batch...)
	lastPageFetched := 1

	// Resume from cursor to backfill older pages.
	resumePage := cursors[sid]
	if resumePage < 2 {
		resumePage = 2
	}
	for page := resumePage; len(allNotices) < limit; page++ {
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			cursors[sid] = page
			return allNotices, nil
		}
		batch, err := fetchPage(page)
		if err != nil {
			break
		}
		allNotices = append(allNotices, batch...)
		lastPageFetched = page
		if len(batch) < pageSize {
			// Reached the end — wrap cursor back to 2 for next run.
			lastPageFetched = 1
			break
		}
	}

	// Advance cursor for next run.
	cursors[sid] = lastPageFetched + 1

	// Deduplicate by AlertID — Interpol pages shift as new notices are
	// added, so page 1 and page 2 can overlap.
	seen := make(map[string]struct{}, len(allNotices))
	deduped := make([]model.Alert, 0, len(allNotices))
	for _, a := range allNotices {
		if _, ok := seen[a.AlertID]; ok {
			continue
		}
		seen[a.AlertID] = struct{}{}
		deduped = append(deduped, a)
	}
	allNotices = deduped

	if len(allNotices) > limit {
		allNotices = allNotices[:limit]
	}
	return allNotices, nil
}

func buildInterpolPageURL(baseURL string, page int, pageSize int) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	q := u.Query()
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("resultPerPage", fmt.Sprintf("%d", pageSize))
	u.RawQuery = q.Encode()
	return u.String()
}

func parseInterpolNotices(nctx normalize.Context, source model.RegistrySource, body []byte) ([]model.Alert, error) {
	var doc struct {
		Embedded struct {
			Notices []struct {
				EntityID               string   `json:"entity_id"`
				Forename               string   `json:"forename"`
				Name                   string   `json:"name"`
				PlaceOfBirth           string   `json:"place_of_birth"`
				IssuingEntity          string   `json:"issuing_entity"`
				Nationalities          []string `json:"nationalities"`
				CountriesLikelyToVisit []string `json:"countries_likely_to_be_visited"`
				Links                  struct {
					Self struct {
						Href string `json:"href"`
					} `json:"self"`
				} `json:"_links"`
			} `json:"notices"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := []model.Alert{}
	for _, notice := range doc.Embedded.Notices {
		titlePrefix := "INTERPOL Red Notice"
		if source.Type == "interpol-yellow-json" {
			titlePrefix = "INTERPOL Yellow Notice"
		}
		label := strings.TrimSpace(strings.TrimSpace(notice.Forename) + " " + strings.TrimSpace(notice.Name))
		title := titlePrefix
		if label != "" {
			title = titlePrefix + ": " + label
		}
		link := interpolWebURL(source.Type, notice.EntityID, notice.Links.Self.Href)
		countryCode := ""
		if len(notice.Nationalities) > 0 {
			countryCode = notice.Nationalities[0]
		} else if len(notice.CountriesLikelyToVisit) > 0 {
			countryCode = notice.CountriesLikelyToVisit[0]
		}
		noticeID := extractInterpolNoticeID(notice.EntityID, link)
		summary := strings.TrimSpace(notice.IssuingEntity + " " + notice.PlaceOfBirth)
		tags := append([]string{}, notice.Nationalities...)
		tags = append(tags, notice.CountriesLikelyToVisit...)
		alert := normalize.InterpolAlert(nctx, source, noticeID, title, link, countryCode, summary, tags)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func fetchInterpolViaBrowser(ctx context.Context, browser *fetch.BrowserClient, source model.RegistrySource) ([]byte, error) {
	pageURL, matchURL := interpolBrowserURLs(source.Type)
	if pageURL == "" || matchURL == "" {
		return nil, fmt.Errorf("no browser fallback for %s", source.Type)
	}
	bodies, err := browser.CaptureJSONResponses(ctx, pageURL, matchURL)
	if err != nil {
		return nil, err
	}
	for _, body := range bodies {
		if len(body) > 0 {
			return body, nil
		}
	}
	return nil, fmt.Errorf("no interpol browser JSON bodies captured")
}

func interpolBrowserURLs(sourceType string) (pageURL string, matchURL string) {
	switch sourceType {
	case "interpol-red-json":
		return "https://www.interpol.int/How-we-work/Notices/Red-Notices/View-Red-Notices", "/notices/v1/red"
	case "interpol-yellow-json":
		return "https://www.interpol.int/How-we-work/Notices/Yellow-Notices/View-Yellow-Notices", "/notices/v1/yellow"
	default:
		return "", ""
	}
}

func extractInterpolNoticeID(entityID string, link string) string {
	if id := strings.TrimSpace(entityID); id != "" {
		return strings.ReplaceAll(id, "/", "-")
	}
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	if fragment := strings.TrimSpace(parsed.Fragment); fragment != "" {
		return strings.ReplaceAll(fragment, "/", "-")
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	// API paths like /notices/v1/red/2026/5314 → "2026-5314"
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[len(parts)-2] >= "1900" && parts[len(parts)-2] <= "2099" {
		return parts[len(parts)-2] + "-" + parts[len(parts)-1]
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

// interpolWebURL converts an Interpol API self-link into a human-readable
// web URL.  e.g. ".../notices/v1/red/2025-81216" becomes
// "https://www.interpol.int/How-we-work/Notices/Red-Notices/View-Red-Notices#2025-81216".
func interpolWebURL(sourceType string, entityID string, selfHref string) string {
	noticeID := extractInterpolNoticeID(entityID, selfHref)
	base := "https://www.interpol.int/How-we-work/Notices/Red-Notices/View-Red-Notices"
	if sourceType == "interpol-yellow-json" {
		base = "https://www.interpol.int/How-we-work/Notices/Yellow-Notices/View-Yellow-Notices"
	}
	if noticeID != "" {
		return base + "#" + noticeID
	}
	return base
}

func (r Runner) fetchFBIWanted(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	limit := perSourceLimit(nctx.Config, source)
	pageSize := 40
	var allAlerts []model.Alert

	for page := 1; len(allAlerts) < limit; page++ {
		pageURL := fmt.Sprintf("%s&page=%d&pageSize=%d", source.FeedURL, page, pageSize)
		body, err := fetcher.Text(ctx, pageURL, source.FollowRedirects, "application/json")
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		items, total, err := parse.ParseFBIWanted(body)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		for _, item := range items {
			if len(allAlerts) >= limit {
				break
			}
			if strings.TrimSpace(item.Title) == "" {
				continue
			}
			alert := normalize.FBIWantedAlert(nctx, source, item)
			if alert != nil {
				allAlerts = append(allAlerts, *alert)
			}
		}
		// Stop if we've fetched all available or last page was partial.
		if total > 0 && page*pageSize >= total {
			break
		}
		if len(items) < pageSize {
			break
		}
		// Polite delay between pages.
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return allAlerts, nil
		}
	}
	return allAlerts, nil
}

func (r Runner) fetchTravelWarningJSON(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseGermanAATravelWarnings(body)
	if err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		if strings.TrimSpace(item.Title) == "" {
			continue
		}
		alert := normalize.TravelWarningAlert(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchTravelWarningAtom(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetchWithFallback(ctx, fetcher, source, "application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseFCDOAtom(body)
	if err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Link) == "" {
			continue
		}
		alert := normalize.TravelWarningAlert(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func fetchWithFallback(ctx context.Context, fetcher fetch.Fetcher, source model.RegistrySource, accept string) ([]byte, error) {
	body, _, err := fetchWithFallbackURL(ctx, fetcher, source, accept)
	return body, err
}

func fetchWithFallbackURL(ctx context.Context, fetcher fetch.Fetcher, source model.RegistrySource, accept string) ([]byte, string, error) {
	candidates := []string{}
	if strings.TrimSpace(source.FeedURL) != "" {
		candidates = append(candidates, source.FeedURL)
	}
	candidates = append(candidates, source.FeedURLs...)
	// Always follow redirects for feed fetches — 301/302/307 are normal
	// for RSS/Atom feeds (HTTP→HTTPS, www→non-www, CDN routing, etc.).
	var lastErr error
	for _, candidate := range candidates {
		body, err := fetcher.Text(ctx, candidate, true, accept)
		if err == nil {
			return body, candidate, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no feed URLs available")
	}
	return nil, "", lastErr
}

func filterKeywords(items []parse.FeedItem, include []string, exclude []string) []parse.FeedItem {
	include = normalizeKeywords(include)
	exclude = normalizeKeywords(exclude)
	out := []parse.FeedItem{}
	for _, item := range items {
		titleHay := strings.ToLower(item.Title)
		fullHay := strings.ToLower(item.Title + " " + item.Link)
		// Include keywords match against title only — matching against the
		// URL caused false positives when the page URL itself contained a
		// keyword (e.g. /desaparecidos in the path let every link through).
		if len(include) > 0 && !containsKeyword(titleHay, include) {
			continue
		}
		// Exclude keywords match against title + URL (conservative).
		if len(exclude) > 0 && containsKeyword(fullHay, exclude) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterFeedKeywords(items []parse.FeedItem, include []string, exclude []string) []parse.FeedItem {
	include = normalizeKeywords(include)
	exclude = normalizeKeywords(exclude)
	out := []parse.FeedItem{}
	for _, item := range items {
		includeHay := strings.ToLower(strings.Join([]string{
			item.Title,
			item.Summary,
			item.Author,
			strings.Join(item.Tags, " "),
		}, " "))
		excludeHay := strings.ToLower(strings.Join([]string{
			item.Title,
			item.Summary,
			item.Author,
			strings.Join(item.Tags, " "),
			item.Link,
		}, " "))
		if len(include) > 0 && !containsKeyword(includeHay, include) {
			continue
		}
		if len(exclude) > 0 && containsKeyword(excludeHay, exclude) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeKeywords(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func containsKeyword(hay string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(hay, needle) {
			return true
		}
	}
	return false
}

// downgradeNonActionable marks alerts from broad sources (those without
// include_keywords) as informational if their title doesn't contain
// actionable intelligence signals. This reduces noise from general-news
// or government feeds that publish ceremony/conference/sports content.
func downgradeNonActionable(alerts []model.Alert, source model.RegistrySource) []model.Alert {
	if len(source.IncludeKeywords) > 0 {
		return alerts // source already keyword-filtered
	}
	// Only downgrade news-like categories that tend to be noisy.
	switch source.Category {
	case "public_safety", "public_appeal", "legislative", "conflict_monitoring",
		"maritime_security", "logistics_incident", "humanitarian_tasking", "humanitarian_security":
		// These categories are typically curated enough — skip.
		return alerts
	}
	for i := range alerts {
		if alerts[i].Severity == "info" || alerts[i].Category == "informational" {
			continue // already informational
		}
		// Explicitly informational titles (conference, training, partnership,
		// review, etc.) are always downgraded, even if they contain keywords
		// like "nuclear" or "pandemic".
		if normalize.IsInformationalTitle(alerts[i].Title) {
			alerts[i].Severity = "info"
			alerts[i].Category = "informational"
			if alerts[i].Triage != nil {
				alerts[i].Triage.WeakSignals = append(
					[]string{"downgraded: informational title pattern"},
					alerts[i].Triage.WeakSignals...,
				)
			}
			continue
		}
		if normalize.IsActionableTitle(alerts[i].Title) {
			continue
		}
		alerts[i].Severity = "info"
		alerts[i].Category = "informational"
		if alerts[i].Triage != nil {
			alerts[i].Triage.WeakSignals = append(
				[]string{"downgraded: non-actionable title from broad source"},
				alerts[i].Triage.WeakSignals...,
			)
		}
	}
	return alerts
}

func filterCategoryItems(items []parse.FeedItem, source model.RegistrySource, categoryDictionary *dictionary.Store) []parse.FeedItem {
	if categoryDictionary == nil {
		return items
	}
	out := make([]parse.FeedItem, 0, len(items))
	for _, item := range items {
		if categoryDictionary.Match(source.Category, source, item.Title, item.Link) {
			out = append(out, item)
		}
	}
	return out
}

func populateSourceHealth(entries []model.SourceHealthEntry, active []model.Alert, filtered []model.Alert) {
	activeBySource := map[string]int{}
	filteredBySource := map[string]int{}
	for _, alert := range active {
		activeBySource[alert.SourceID]++
	}
	for _, alert := range filtered {
		filteredBySource[alert.SourceID]++
	}
	for i := range entries {
		entries[i].ActiveCount = activeBySource[entries[i].SourceID]
		entries[i].FilteredCount = filteredBySource[entries[i].SourceID]
	}
}

func assertCriticalSourceCoverage(cfg config.Config, entries []model.SourceHealthEntry) error {
	if !cfg.FailOnCriticalSourceGap || len(cfg.CriticalSourcePrefixes) == 0 {
		return nil
	}
	missing := []string{}
	for _, prefix := range cfg.CriticalSourcePrefixes {
		total := 0
		for _, entry := range entries {
			if entry.SourceID == prefix || strings.HasPrefix(entry.SourceID, prefix+"-") {
				total += entry.FetchedCount
			}
		}
		if total == 0 {
			missing = append(missing, prefix)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("critical source coverage gap: no records fetched for %s", strings.Join(missing, ", "))
}

func perSourceLimit(cfg config.Config, source model.RegistrySource) int {
	if source.MaxItems > 0 {
		return source.MaxItems
	}
	return cfg.MaxPerSource
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isSQLitePath(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".db" || ext == ".sqlite" || ext == ".sqlite3"
}

func (r Runner) mergeRegistry(ctx context.Context, cfg config.Config) error {
	db, err := sourcedb.Open(cfg.RegistryPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.MergeRegistry(ctx, cfg.RegistrySeedPath)
}

// cityLookupAdapter wraps sourcedb.DB to satisfy normalize.CityLookup.
type cityLookupAdapter struct {
	db *sourcedb.DB
}

func (a *cityLookupAdapter) LookupCity(ctx context.Context, name string, countryCode string) (normalize.CityLookupResult, bool) {
	r, ok := a.db.LookupCity(ctx, name, countryCode)
	if !ok {
		return normalize.CityLookupResult{}, false
	}
	return normalize.CityLookupResult{
		Name:        r.Name,
		CountryCode: r.CountryCode,
		Lat:         r.Lat,
		Lng:         r.Lng,
		Population:  r.Population,
	}, true
}

func (r Runner) initGeocoder(ctx context.Context, cfg config.Config) *normalize.Geocoder {
	var cities normalize.CityLookup
	var nominatim *normalize.NominatimClient

	// Try to open the source DB for city lookups.
	if isSQLitePath(cfg.RegistryPath) {
		db, err := sourcedb.Open(cfg.RegistryPath)
		if err == nil {
			// Import GeoNames if the cities table is empty and the file exists.
			if !db.HasCities(ctx) && cfg.GeoNamesPath != "" {
				if err := db.ImportGeoNames(ctx, cfg.GeoNamesPath); err != nil {
					fmt.Fprintf(r.stderr, "WARN geonames import: %v\n", err)
				}
			}
			if db.HasCities(ctx) {
				cities = &cityLookupAdapter{db: db}
				// NOTE: we intentionally don't defer db.Close() here because
				// the geocoder is used throughout the run. The DB handle is
				// safe for concurrent reads.
			} else {
				db.Close()
			}
		} else {
			fmt.Fprintf(r.stderr, "WARN geocoder DB open: %v\n", err)
		}
	}

	if cfg.NominatimEnabled {
		nominatim = normalize.NewNominatimClient(cfg.NominatimBaseURL, cfg.WikimediaUserAgent)
	}

	geocoder := normalize.NewGeocoder(cities, nominatim)

	// Wire LLM geocoding fallback if the vetting endpoint is configured.
	if cfg.VettingAPIKey != "" {
		client := vet.NewClient(cfg)
		geocoder.SetLLM(&geoLLMAdapter{client: client, stderr: r.stderr})
	}

	return geocoder
}

// geoLLMAdapter wraps vet.Client to implement normalize.GeoLLM.
type geoLLMAdapter struct {
	client *vet.Client
	stderr io.Writer
}

func (a *geoLLMAdapter) GeoLocate(ctx context.Context, query string) (lat, lng float64, ok bool) {
	resp, err := a.client.Complete(ctx, []vet.Message{
		{Role: "user", Content: query + ", geo coords, nothing else"},
	})
	if err != nil {
		fmt.Fprintf(a.stderr, "WARN llm-geo: %v\n", err)
		return 0, 0, false
	}
	resp = strings.TrimSpace(resp)
	if lat, lng, ok := normalize.ExtractCoordinates(resp); ok {
		return lat, lng, true
	}
	fmt.Fprintf(a.stderr, "WARN llm-geo: could not parse coords from %q\n", resp)
	return 0, 0, false
}

func (r Runner) runDiscoveryLoop(ctx context.Context, cfg config.Config) {
	runOnce := func() {
		if err := discover.Run(ctx, cfg, r.stdout, r.stderr); err != nil && ctx.Err() == nil {
			fmt.Fprintf(r.stderr, "WARN background discovery failed: %v\n", err)
		}
	}

	interval := time.Duration(cfg.DiscoverIntervalMS) * time.Millisecond
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func classifySourceError(err error) (string, bool, string) {
	if err == nil {
		return "", false, ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "status 404"), strings.Contains(msg, "status 410"):
		return "not_found", true, "dead_letter"
	case strings.Contains(msg, "status 401"):
		// Feed requires auth or blocks anonymous clients.
		return "unauthorized", true, "dead_letter"
	case strings.Contains(msg, "status 522"):
		// Cloudflare connection timeout at origin; frequently persistent dead feeds.
		return "origin_unreachable", true, "dead_letter"
	case strings.Contains(msg, "status 301"), strings.Contains(msg, "status 302"), strings.Contains(msg, "status 307"), strings.Contains(msg, "status 308"):
		// Redirects should be followed automatically — if we still see
		// one here it means the chain exceeded 10 hops.
		return "redirect", false, "retry"
	case strings.Contains(msg, "status 403"):
		return "blocked", true, "dead_letter"
	case strings.Contains(msg, "response too large"):
		return "oversized", true, "dead_letter"
	case strings.Contains(msg, "certificate signed by unknown authority"):
		return "tls_invalid", true, "dead_letter"
	case strings.Contains(msg, "no such host"):
		return "dns_error", true, "dead_letter"
	case strings.Contains(msg, "client.timeout exceeded"), strings.Contains(msg, "request canceled"), strings.Contains(msg, "timeout"):
		return "timeout", false, "retry"
	case strings.Contains(msg, ": eof"), strings.HasSuffix(msg, " eof"):
		return "eof", false, "retry"
	default:
		return "transient", false, "retry"
	}
}

func buildDLQEntry(entry model.SourceHealthEntry, source model.RegistrySource) model.SourceReplacementCandidate {
	return model.SourceReplacementCandidate{
		SourceID:        entry.SourceID,
		AuthorityName:   entry.AuthorityName,
		Type:            entry.Type,
		FeedURL:         entry.FeedURL,
		BaseURL:         source.Source.BaseURL,
		Country:         source.Source.Country,
		CountryCode:     source.Source.CountryCode,
		Region:          source.Source.Region,
		AuthorityType:   source.Source.AuthorityType,
		Category:        source.Category,
		Error:           entry.Error,
		ErrorClass:      entry.ErrorClass,
		DiscoveryAction: entry.DiscoveryAction,
	}
}

func buildReplacementQueue(entries []model.SourceHealthEntry, sources []model.RegistrySource) []model.SourceReplacementCandidate {
	byID := make(map[string]model.RegistrySource, len(sources))
	for _, source := range sources {
		byID[source.Source.SourceID] = source
	}

	queue := make([]model.SourceReplacementCandidate, 0)
	for _, entry := range entries {
		if !entry.NeedsReplacement {
			continue
		}
		source, ok := byID[entry.SourceID]
		if !ok {
			continue
		}
		queue = append(queue, model.SourceReplacementCandidate{
			SourceID:        entry.SourceID,
			AuthorityName:   entry.AuthorityName,
			Type:            entry.Type,
			FeedURL:         entry.FeedURL,
			BaseURL:         source.Source.BaseURL,
			Country:         source.Source.Country,
			CountryCode:     source.Source.CountryCode,
			Region:          source.Source.Region,
			AuthorityType:   source.Source.AuthorityType,
			Category:        source.Category,
			Error:           entry.Error,
			ErrorClass:      entry.ErrorClass,
			DiscoveryAction: entry.DiscoveryAction,
			LastAttemptAt:   entry.FinishedAt,
		})
	}
	return queue
}

func deactivateReplacementSources(ctx context.Context, registryPath string, queue []model.SourceReplacementCandidate) error {
	if !isSQLiteRegistryPath(registryPath) || len(queue) == 0 {
		return nil
	}
	db, err := sourcedb.Open(registryPath)
	if err != nil {
		return err
	}
	defer db.Close()

	reasons := make(map[string]string, len(queue))
	for _, candidate := range queue {
		reasons[candidate.SourceID] = candidate.Error
	}
	return db.DeactivateSources(ctx, reasons)
}

func loadPreviousAlerts(ctx context.Context, cfg config.Config) ([]model.Alert, error) {
	if !isSQLiteRegistryPath(cfg.RegistryPath) {
		previous := state.Read(cfg.StateOutputPath)
		if len(previous) == 0 {
			previous = state.Read(cfg.OutputPath)
		}
		return previous, nil
	}

	db, err := sourcedb.Open(cfg.RegistryPath)
	if err != nil {
		return nil, fmt.Errorf("open source DB for alert state: %w", err)
	}
	defer db.Close()

	alerts, err := db.LoadAlerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load alert state from source DB: %w", err)
	}
	return alerts, nil
}

func saveAlertState(ctx context.Context, cfg config.Config, alerts []model.Alert) error {
	if !isSQLiteRegistryPath(cfg.RegistryPath) {
		return nil
	}

	db, err := sourcedb.Open(cfg.RegistryPath)
	if err != nil {
		return fmt.Errorf("open source DB for alert save: %w", err)
	}
	defer db.Close()

	if err := db.SaveAlerts(ctx, alerts); err != nil {
		return fmt.Errorf("save alert state to source DB: %w", err)
	}
	return nil
}

// applyCorpusScores uses BM25 to compute how distinctive each alert's title
// is against the full corpus. The corpus score is blended into the existing
// heuristic RelevanceScore: final = 0.7×heuristic + 0.3×corpus.
func (r Runner) applyCorpusScores(ctx context.Context, cfg config.Config, alerts []model.Alert) ([]model.Alert, error) {
	if !isSQLiteRegistryPath(cfg.RegistryPath) {
		return alerts, nil
	}
	db, err := sourcedb.Open(cfg.RegistryPath)
	if err != nil {
		return alerts, fmt.Errorf("open DB for corpus scoring: %w", err)
	}
	defer db.Close()

	scores, err := db.CorpusScores(ctx)
	if err != nil {
		return alerts, err
	}
	if len(scores) == 0 {
		return alerts, nil
	}

	const heuristicWeight = 0.7
	const corpusWeight = 0.3
	boosted := 0

	for i := range alerts {
		corpusScore, ok := scores[alerts[i].AlertID]
		if !ok || alerts[i].Triage == nil {
			continue
		}
		original := alerts[i].Triage.RelevanceScore
		blended := heuristicWeight*original + corpusWeight*corpusScore
		// Round to 3 decimal places.
		blended = math.Round(blended*1000) / 1000
		if blended != original {
			alerts[i].Triage.RelevanceScore = blended
			alerts[i].Triage.WeakSignals = append(alerts[i].Triage.WeakSignals,
				fmt.Sprintf("corpus-bm25=%.3f (blended %.3f→%.3f)", corpusScore, original, blended))
			boosted++
		}
	}
	if boosted > 0 {
		fmt.Fprintf(r.stderr, "Corpus BM25 scoring: adjusted %d/%d alerts\n", boosted, len(alerts))
	}
	return alerts, nil
}

// recordTrendsAndDetectSpikes records term frequencies from the current
// cycle's alerts and returns any detected spikes.
func (r Runner) recordTrendsAndDetectSpikes(ctx context.Context, cfg config.Config, alerts []model.Alert, now time.Time) ([]trends.Spike, error) {
	if !isSQLiteRegistryPath(cfg.RegistryPath) {
		return nil, nil
	}
	db, err := sourcedb.Open(cfg.RegistryPath)
	if err != nil {
		return nil, fmt.Errorf("open DB for trends: %w", err)
	}
	defer db.Close()

	detector := trends.New(db.RawDB())
	if err := detector.Init(ctx); err != nil {
		return nil, err
	}
	if err := detector.Record(ctx, alerts, now); err != nil {
		return nil, err
	}
	// Prune old trend data (keep 30 days).
	if err := detector.Prune(ctx, now, 30); err != nil {
		fmt.Fprintf(r.stderr, "WARN trend prune: %v\n", err)
	}
	spikes, err := detector.DetectSpikes(ctx, now, 7, 3.0, 3)
	if err != nil {
		return nil, err
	}
	if len(spikes) > 0 {
		trends.AnnotateSpikesWithSamples(spikes, alerts)
		for _, s := range spikes {
			fmt.Fprintf(r.stderr, "  TREND: %q (%s/%s) %dx today vs %.1f avg [%s]\n",
				s.Term, s.Category, s.Region, s.TodayCount, s.AvgCount, s.SampleTitle)
		}
	}
	return spikes, nil
}

// queueTrendHints converts trend spikes into discovery candidates and
// appends them to the candidate queue for the next discovery cycle.
func (r Runner) queueTrendHints(cfg config.Config, hints []trends.DiscoveryHint) error {
	candidates := trends.HintsToCandidates(hints)
	if len(candidates) == 0 {
		return nil
	}

	// Load existing candidate queue, merge, and write back.
	existing := loadCandidateQueueJSON(cfg.CandidateQueuePath)
	merged := trends.MergeCandidateQueue(existing, candidates)
	return writeCandidateQueueJSON(cfg.CandidateQueuePath, merged)
}

func isSQLiteRegistryPath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	default:
		return false
	}
}

func loadCandidateQueueJSON(path string) []model.SourceCandidate {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc model.SourceCandidateDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	return doc.Sources
}

func writeCandidateQueueJSON(path string, candidates []model.SourceCandidate) error {
	doc := model.SourceCandidateDocument{
		Sources: candidates,
	}
	if doc.Sources == nil {
		doc.Sources = []model.SourceCandidate{}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal candidate queue: %w", err)
	}
	return os.WriteFile(path, raw, 0644)
}
