// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/dictionary"
	"github.com/scalytics/euosint/internal/collector/discover"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/noisegate"
	"github.com/scalytics/euosint/internal/collector/normalize"
	"github.com/scalytics/euosint/internal/collector/output"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/collector/registry"
	"github.com/scalytics/euosint/internal/collector/state"
	"github.com/scalytics/euosint/internal/collector/translate"
	"github.com/scalytics/euosint/internal/collector/trends"
	"github.com/scalytics/euosint/internal/collector/vet"
	"github.com/scalytics/euosint/internal/collector/zonebrief"
	"github.com/scalytics/euosint/internal/sourcedb"
)

type Runner struct {
	stdout         io.Writer
	stderr         io.Writer
	clientFactory  func(config.Config) *fetch.Client
	browserFactory func(config.Config) (*fetch.BrowserClient, error)
	noiseGate      *noisegate.Engine
}

func New(stdout io.Writer, stderr io.Writer) Runner {
	return Runner{
		stdout:        stdout,
		stderr:        stderr,
		clientFactory: fetch.New,
		browserFactory: func(cfg config.Config) (*fetch.BrowserClient, error) {
			return fetch.NewBrowser(fetch.BrowserOptions{
				TimeoutMS:           cfg.BrowserTimeoutMS,
				WSURL:               cfg.BrowserWSURL,
				MaxConcurrency:      cfg.BrowserMaxConcurrency,
				ConnectRetries:      cfg.BrowserConnectRetries,
				ConnectRetryDelayMS: cfg.BrowserConnectRetryDelayMS,
			})
		},
	}
}

func (r Runner) Run(ctx context.Context, cfg config.Config) error {
	if cfg.Watch {
		return r.watch(ctx, cfg)
	}
	return r.runOnce(ctx, cfg)
}

func (r Runner) SyncZoneBriefings(ctx context.Context, cfg config.Config) error {
	sources, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		return err
	}
	var runStore *sourcedb.DB
	if isSQLiteRegistryPath(cfg.RegistryPath) {
		db, err := sourcedb.Open(cfg.RegistryPath)
		if err != nil {
			return err
		}
		runStore = db
		defer runStore.Close()
	}
	if err := r.syncZoneBriefings(ctx, cfg, sources, runStore, true); err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdout, "Zone briefings cache sync completed -> %s\n", cfg.ZoneBriefingsOutputPath)
	return err
}

func (r Runner) watch(ctx context.Context, cfg config.Config) error {
	// Discovery is fully independent — start it immediately on boot
	// instead of waiting for the first collection sweep to finish.
	// It only appends new sources into the registry; the collector
	// picks them up on its next cycle via registry.Load().
	if cfg.DiscoverBackground {
		go r.runDiscoveryLoop(ctx, cfg)
	}

	ticker := time.NewTicker(time.Duration(cfg.IntervalMS) * time.Millisecond)
	defer ticker.Stop()

	go r.runRegistrySyncLoop(ctx, cfg)

	for {
		if err := r.runOnce(ctx, cfg); err != nil {
			fmt.Fprintf(r.stderr, "collector run failed: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r Runner) runOnce(ctx context.Context, cfg config.Config) error {
	noiseEngine, err := noisegate.LoadAB(cfg.NoisePolicyPath, cfg.NoisePolicyBPath, cfg.NoisePolicyBPercent)
	if err != nil {
		fmt.Fprintf(r.stderr, "WARN noise policy load failed: %v\n", err)
	} else {
		r.noiseGate = noiseEngine
	}

	// In one-shot mode we perform a seed sync inline. In watch mode,
	// the dedicated registry-sync loop handles it at its own cadence.
	if !cfg.Watch {
		if err := r.syncRegistrySeed(ctx, cfg); err != nil {
			fmt.Fprintf(r.stderr, "WARN registry sync: %v\n", err)
		}
	}

	sources, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		return err
	}
	sources = prioritizeSources(sources)
	client := r.clientFactory(cfg)
	if err := r.refreshMilitaryBasesLayer(ctx, cfg, client); err != nil {
		fmt.Fprintf(r.stderr, "WARN military bases layer refresh failed: %v\n", err)
	}
	var runStore *sourcedb.DB
	if isSQLiteRegistryPath(cfg.RegistryPath) {
		db, err := sourcedb.Open(cfg.RegistryPath)
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN source-run DB open failed: %v\n", err)
		} else {
			runStore = db
			defer runStore.Close()
		}
	}
	if err := r.syncZoneBriefings(ctx, cfg, sources, runStore, false); err != nil {
		fmt.Fprintf(r.stderr, "WARN zone briefings: %v\n", err)
	}

	var browser *fetch.BrowserClient
	if cfg.BrowserEnabled && r.browserFactory != nil {
		b, err := r.browserFactory(cfg)
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN browser init failed (falling back to stealth): %v\n", err)
		} else {
			browser = b
			if warning := browser.Warning(); warning != "" {
				fmt.Fprintf(r.stderr, "WARN browser: %s\n", warning)
			}
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
	if dlq.Len() > 0 {
		fmt.Fprintf(r.stderr, "DLQ loaded: %d dead sources will be skipped\n", dlq.Len())
	}

	// Load previous alerts early so progress snapshots include them.
	// This prevents the dashboard from going blank during a sweep.
	previousAlerts, err := loadPreviousAlerts(ctx, cfg)
	if err != nil {
		fmt.Fprintf(r.stderr, "WARN previous alerts load failed: %v\n", err)
	}
	// Load previous source health so progress snapshots can carry forward
	// entries for sources not yet fetched this cycle. Without this, the UI
	// sees a shrinking total_sources count during each sweep.
	previousSourceHealth := loadPreviousSourceHealth(cfg)

	// Load watermarks so working feeds can use conditional GET (ETag /
	// Last-Modified / content-hash). Unchanged feeds get a cheap 304 and
	// carry forward their previous alerts without re-parsing.
	watermarks := loadAllWatermarks(ctx, runStore)

	// Split sources into explicit execution lanes:
	//   - fast: cheap document feeds with short timeouts
	//   - api: structured/API-backed sources with full HTTP timeout
	//   - browser: browser-backed or cadence-sensitive sources
	var fastSources, apiSources, browserSources []model.RegistrySource
	for _, s := range sources {
		if usesFastLane(s) {
			fastSources = append(fastSources, s)
		} else if usesBrowserLane(s) {
			browserSources = append(browserSources, s)
		} else {
			apiSources = append(apiSources, s)
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
						entry := model.SourceHealthEntry{
							SourceID:      source.Source.SourceID,
							AuthorityName: source.Source.AuthorityName,
							Type:          source.Type,
							FeedURL:       source.FeedURL,
							Status:        "skipped",
							Error:         "dead letter queue",
							ErrorClass:    "dlq",
							StartedAt:     now.Format(time.RFC3339),
							FinishedAt:    time.Now().UTC().Format(time.RFC3339),
						}
						mu.Lock()
						sourceHealth = append(sourceHealth, entry)
						completed++
						mu.Unlock()
						recordSourceRun(ctx, runStore, source, entry, nil, 0, map[string]any{"reason": "dlq"})
						continue
					}
					wm := watermarks[source.Source.SourceID]
					batch, entry := r.fetchOneSource(ctx, fastClient, nil, nctx, source, categoryDictionary, cursors, wm, client)
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
						r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth, previousSourceHealth, len(sources))
						// Flush DLQ so background discovery can see dead sources early.
						_ = dlq.Write(cfg.ReplacementQueuePath)
					}
					mu.Unlock()
					var runMeta map[string]any
					if entry.ErrorClass == "not_modified" {
						runMeta = map[string]any{"not_modified": true}
					}
					recordSourceRun(ctx, runStore, source, entry, batch, inferHTTPStatus(entry), runMeta)
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
		r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth, previousSourceHealth, len(sources))
	}

	// API pass — structured sources with full timeout, but no browser transport.
	apiWorkers := cfg.FetchWorkers
	if apiWorkers <= 0 {
		apiWorkers = 12
	}
	if apiWorkers > len(apiSources) && len(apiSources) > 0 {
		apiWorkers = len(apiSources)
	}

	if len(apiSources) > 0 {
		work := make(chan model.RegistrySource, len(apiSources))
		for _, s := range apiSources {
			work <- s
		}
		close(work)

		var wg sync.WaitGroup
		for i := 0; i < apiWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for source := range work {
					if ctx.Err() != nil {
						return
					}
					if dlq.ShouldSkip(source.Source.SourceID, now) {
						entry := model.SourceHealthEntry{
							SourceID:      source.Source.SourceID,
							AuthorityName: source.Source.AuthorityName,
							Type:          source.Type,
							FeedURL:       source.FeedURL,
							Status:        "skipped",
							Error:         "dead letter queue",
							ErrorClass:    "dlq",
							StartedAt:     now.Format(time.RFC3339),
							FinishedAt:    time.Now().UTC().Format(time.RFC3339),
						}
						mu.Lock()
						sourceHealth = append(sourceHealth, entry)
						completed++
						mu.Unlock()
						recordSourceRun(ctx, runStore, source, entry, nil, 0, map[string]any{"reason": "dlq"})
						continue
					}

					wm := watermarks[source.Source.SourceID]
					batch, entry := r.fetchOneSource(ctx, client, nil, nctx, source, categoryDictionary, cursors, wm)
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
					if completed%25 == 0 {
						r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth, previousSourceHealth, len(sources))
						_ = dlq.Write(cfg.ReplacementQueuePath)
					}
					mu.Unlock()
					var runMeta map[string]any
					if entry.ErrorClass == "not_modified" {
						runMeta = map[string]any{"not_modified": true}
					}
					recordSourceRun(ctx, runStore, source, entry, batch, inferHTTPStatus(entry), runMeta)
					if entry.Status == "error" && entry.DiscoveryAction == "dead_letter" {
						fmt.Fprintf(r.stderr, "WARN %s: %s (added to DLQ)\n", source.Source.AuthorityName, entry.Error)
					} else if entry.Status == "error" {
						fmt.Fprintf(r.stderr, "WARN %s: %s\n", source.Source.AuthorityName, entry.Error)
					}
				}
			}()
		}
		wg.Wait()
		r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth, previousSourceHealth, len(sources))
	}

	// Browser pass — sequential, with cadence/rate-limit handling.
	var lastXFetch time.Time
	for _, source := range browserSources {
		if ctx.Err() != nil {
			break
		}
		if source.Type == "x" && cfg.XFetchPauseMS > 0 && !lastXFetch.IsZero() {
			pause := time.Duration(cfg.XFetchPauseMS) * time.Millisecond
			nextAllowed := lastXFetch.Add(pause)
			if wait := time.Until(nextAllowed); wait > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
		}
		if dlq.ShouldSkip(source.Source.SourceID, now) {
			entry := model.SourceHealthEntry{
				SourceID:      source.Source.SourceID,
				AuthorityName: source.Source.AuthorityName,
				Type:          source.Type,
				FeedURL:       source.FeedURL,
				Status:        "skipped",
				Error:         "dead letter queue",
				ErrorClass:    "dlq",
				StartedAt:     now.Format(time.RFC3339),
				FinishedAt:    time.Now().UTC().Format(time.RFC3339),
			}
			sourceHealth = append(sourceHealth, entry)
			completed++
			recordSourceRun(ctx, runStore, source, entry, nil, 0, map[string]any{"reason": "dlq"})
			continue
		}
		if source.Type == "html-list" && runStore != nil {
			shouldSkip, probeStatus, reason := r.shouldSkipHTMLScrape(ctx, cfg, client, runStore, source, now)
			if shouldSkip {
				entry := model.SourceHealthEntry{
					SourceID:      source.Source.SourceID,
					AuthorityName: source.Source.AuthorityName,
					Type:          source.Type,
					FeedURL:       source.FeedURL,
					Status:        "skipped",
					Error:         reason,
					ErrorClass:    "cadence",
					StartedAt:     now.Format(time.RFC3339),
					FinishedAt:    time.Now().UTC().Format(time.RFC3339),
				}
				sourceHealth = append(sourceHealth, entry)
				completed++
				recordSourceRun(ctx, runStore, source, entry, nil, probeStatus, map[string]any{"reason": reason, "probe_only": true})
				continue
			}
		}
		batch, entry := r.fetchOneSourceSlow(ctx, client, browser, nctx, source, categoryDictionary, cursors)
		if source.Type == "x" {
			lastXFetch = time.Now()
		}
		sourceHealth = append(sourceHealth, entry)
		alerts = append(alerts, batch...)
		completed++
		recordSourceRun(ctx, runStore, source, entry, batch, inferHTTPStatus(entry), nil)
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
			r.writeProgressSnapshot(cfg, alerts, previousAlerts, sourceHealth, previousSourceHealth, len(sources))
			_ = dlq.Write(cfg.ReplacementQueuePath)
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
	deduped = normalize.ApplySignalLanes(cfg, deduped)
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
	// Keep sources active even when queued for replacement. This avoids
	// collapsing active feed coverage due to transient source failures.
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
	if err := r.writeNoiseMetrics(ctx, cfg, currentActive, runStore); err != nil {
		fmt.Fprintf(r.stderr, "WARN noise metrics: %v\n", err)
	}
	_, err = fmt.Fprintf(r.stdout, "Wrote %d active alerts -> %s (%d filtered in %s)\n", len(currentActive), cfg.OutputPath, len(currentFiltered), cfg.FilteredOutputPath)
	return err
}

func (r Runner) runRegistrySyncLoop(ctx context.Context, cfg config.Config) {
	if err := r.syncRegistrySeed(ctx, cfg); err != nil {
		fmt.Fprintf(r.stderr, "WARN registry sync: %v\n", err)
	}
	interval := cfg.IntervalMS
	if interval <= 0 {
		interval = 60000
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.syncRegistrySeed(ctx, cfg); err != nil {
				fmt.Fprintf(r.stderr, "WARN registry sync: %v\n", err)
			}
		}
	}
}

func (r Runner) syncRegistrySeed(ctx context.Context, cfg config.Config) error {
	if !isSQLitePath(cfg.RegistryPath) {
		return nil
	}
	seedPath := strings.TrimSpace(cfg.RegistrySeedPath)
	if seedPath == "" {
		return nil
	}
	data, err := os.ReadFile(seedPath)
	if err != nil {
		return fmt.Errorf("read registry seed: %w", err)
	}
	hash := sha1.Sum(data)
	hashHex := hex.EncodeToString(hash[:])
	syncKey := "seed:" + filepath.Clean(seedPath)

	db, err := sourcedb.Open(cfg.RegistryPath)
	if err != nil {
		return err
	}
	defer db.Close()

	prev, ok, err := db.GetRegistrySyncState(ctx, syncKey)
	if err != nil {
		return err
	}
	if ok && strings.EqualFold(strings.TrimSpace(prev.LastHash), hashHex) {
		return nil
	}
	if err := db.MergeRegistry(ctx, seedPath); err != nil {
		return err
	}
	sourceCount, err := db.CountSources(ctx)
	if err != nil {
		return err
	}
	if err := db.UpsertRegistrySyncState(ctx, sourcedb.RegistrySyncState{
		SyncKey:      syncKey,
		LastHash:     hashHex,
		LastSyncedAt: time.Now().UTC().Format(time.RFC3339),
		SourceCount:  sourceCount,
	}); err != nil {
		return err
	}
	fmt.Fprintf(r.stderr, "Registry sync applied: seed=%s sources=%d hash=%s\n", seedPath, sourceCount, hashHex[:12])
	return nil
}

func (r Runner) writeProgressSnapshot(cfg config.Config, freshAlerts []model.Alert, previousAlerts []model.Alert, sourceHealth []model.SourceHealthEntry, previousSourceHealth []model.SourceHealthEntry, totalRegistrySources int) {
	// Merge fresh alerts with previous state so the dashboard never goes
	// blank during a sweep. Previous alerts that aren't in the fresh batch
	// are carried forward as-is — but only for sources that haven't been
	// fetched yet this cycle. Once a source has fresh alerts, its old ones
	// are dropped so paginated sources (Interpol) don't accumulate beyond
	// their max_items limit.
	freshByID := make(map[string]struct{}, len(freshAlerts))
	freshSourceIDs := make(map[string]struct{})
	for _, a := range freshAlerts {
		freshByID[a.AlertID] = struct{}{}
		freshSourceIDs[a.SourceID] = struct{}{}
	}
	merged := make([]model.Alert, 0, len(freshAlerts)+len(previousAlerts))
	merged = append(merged, freshAlerts...)
	for _, prev := range previousAlerts {
		if _, ok := freshByID[prev.AlertID]; ok {
			continue // already in fresh batch
		}
		if _, ok := freshSourceIDs[prev.SourceID]; ok {
			continue // source already fetched this cycle — don't carry forward old alerts
		}
		if prev.Status == "removed" || prev.Status == "filtered" {
			continue // don't resurrect removed/filtered alerts in snapshots
		}
		merged = append(merged, prev)
	}
	// Merge source health: carry forward previous entries for sources not
	// yet fetched this cycle so the UI always sees the full feed list.
	mergedHealth := mergeSourceHealth(sourceHealth, previousSourceHealth)
	deduped, duplicateAudit := normalize.Deduplicate(merged)
	deduped = normalize.ApplySignalLanes(cfg, deduped)
	active, filtered := normalize.FilterActive(cfg, deduped)
	if err := output.WriteWithTotal(cfg, active, filtered, active, mergedHealth, duplicateAudit, nil, totalRegistrySources); err != nil {
		fmt.Fprintf(r.stderr, "WARN progress snapshot write failed: %v\n", err)
		return
	}
	fmt.Fprintf(r.stdout, "Progress snapshot: %d active alerts (%d fresh + %d previous) after %d/%d sources\n", len(active), len(freshAlerts), len(previousAlerts), len(sourceHealth), totalRegistrySources)
}

// mergeSourceHealth carries forward previous-run health entries for sources
// not yet fetched in the current sweep. This keeps the UI feed list stable.
func mergeSourceHealth(current []model.SourceHealthEntry, previous []model.SourceHealthEntry) []model.SourceHealthEntry {
	if len(previous) == 0 {
		return current
	}
	seen := make(map[string]struct{}, len(current))
	for _, e := range current {
		seen[e.SourceID] = struct{}{}
	}
	merged := make([]model.SourceHealthEntry, 0, len(current)+len(previous))
	merged = append(merged, current...)
	for _, prev := range previous {
		if _, ok := seen[prev.SourceID]; ok {
			continue
		}
		// Mark as pending so the UI can distinguish stale-from-previous-run.
		prev.Status = "pending"
		prev.Error = ""
		prev.ErrorClass = ""
		merged = append(merged, prev)
	}
	return merged
}

// loadPreviousSourceHealth reads the last-written source-health.json so that
// progress snapshots can carry forward entries for sources not yet fetched.
func loadPreviousSourceHealth(cfg config.Config) []model.SourceHealthEntry {
	data, err := os.ReadFile(cfg.SourceHealthOutputPath)
	if err != nil {
		return nil
	}
	var doc model.SourceHealthDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	return doc.Sources
}

// loadAllWatermarks loads every source watermark from the DB into a map.
// Returns an empty map when the DB is unavailable.
func loadAllWatermarks(ctx context.Context, store *sourcedb.DB) map[string]*sourcedb.SourceWatermark {
	if store == nil {
		return map[string]*sourcedb.SourceWatermark{}
	}
	wms, err := store.GetAllWatermarks(ctx)
	if err != nil {
		return map[string]*sourcedb.SourceWatermark{}
	}
	return wms
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

// usesFastLane returns true only for cheap document feeds that are expected to
// respond quickly and do not fan out into multiple API calls.
func usesFastLane(s model.RegistrySource) bool {
	switch s.Type {
	case "rss", "travelwarning-atom":
		return true
	}
	return false
}

// usesBrowserLane returns true for sources that either explicitly fetch via the
// browser bridge or need browser/cadence-aware handling.
func usesBrowserLane(s model.RegistrySource) bool {
	switch s.Type {
	case "html-list", "telegram", "x", "interpol-red-json", "interpol-yellow-json":
		return true
	}
	return strings.EqualFold(strings.TrimSpace(s.FetchMode), "browser")
}

// fetcherForSource selects the transport that best matches the source shape.
// Social surfaces should prefer the browser bridge when available so the
// collector sees the same rendered page an analyst would.
func fetcherForSource(source model.RegistrySource, client *fetch.Client, browser *fetch.BrowserClient) fetch.Fetcher {
	if browser == nil {
		return client
	}
	switch source.Type {
	case "telegram", "x":
		return browser
	default:
		return fetch.FetcherFor(source.FetchMode, client, browser)
	}
}

// fetchOneSource fetches a single source with retry logic, returning the
// batch of alerts and a health entry. When customFetcher is nil, the
// defaultClient is used directly. When a watermark with ETag/Last-Modified
// is available, a conditional GET is tried first:
//
//   - 304 Not Modified → no new content, return empty (reconciler carries
//     forward previous alerts). Cost: ~100 bytes, ~50-200ms.
//   - 200 OK → body is fed through the parse pipeline via PrefetchedFetcher
//     so there is no double-fetch.
//
// As a fallback (no watermark or server doesn't support conditional GET),
// the content hash from the previous run is compared after parsing.
func (r Runner) fetchOneSource(ctx context.Context, defaultClient *fetch.Client, customFetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store, cursors state.Cursors, wm *sourcedb.SourceWatermark, retryClient ...*fetch.Client) ([]model.Alert, model.SourceHealthEntry) {
	startedAt := time.Now().UTC()

	var fetcher fetch.Fetcher
	if customFetcher != nil {
		fetcher = customFetcher
	} else {
		fetcher = defaultClient
	}

	// ── Conditional GET fast-path ────────────────────────────────────
	// For known-working sources with cached ETag/Last-Modified, ask the
	// server whether anything changed. A 304 costs almost nothing.
	// A 200 gives us the body which we feed into the normal parse chain
	// via PrefetchedFetcher — zero wasted requests.
	var respETag, respLastModified string
	if wm != nil && wm.LastStatus == "ok" && (wm.LastETag != "" || wm.LastModified != "") {
		if client, ok := fetcher.(*fetch.Client); ok {
			accept := acceptForType(source.Type)
			result, err := client.TextConditional(ctx, source.FeedURL, source.FollowRedirects, accept, wm.LastETag, wm.LastModified)
			if err == nil && result.NotModified {
				return nil, model.SourceHealthEntry{
					SourceID:         source.Source.SourceID,
					AuthorityName:    source.Source.AuthorityName,
					Type:             source.Type,
					FeedURL:          source.FeedURL,
					Status:           "ok",
					ErrorClass:       "not_modified",
					FetchedCount:     0,
					StartedAt:        startedAt.Format(time.RFC3339),
					FinishedAt:       time.Now().UTC().Format(time.RFC3339),
					RespETag:         wm.LastETag,
					RespLastModified: wm.LastModified,
				}
			}
			if err == nil && len(result.Body) > 0 {
				// Got a 200 with new content — wrap the body so
				// fetchSource uses it instead of re-requesting.
				fetcher = &fetch.PrefetchedFetcher{
					Inner: fetcher,
					URL:   source.FeedURL,
					Body:  result.Body,
				}
				respETag = result.ETag
				respLastModified = result.LastModified
			}
			// On error, fall through to normal fetch.
		}
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
				// Retry with the raw fetcher (not prefetched — body was
				// already consumed or the request failed).
				// For timeout retries, use the retry client (full timeout)
				// if provided — the fast client's 3s may be too short for
				// Cloudflare-protected feeds.
				var retryFetcher fetch.Fetcher
				if customFetcher != nil {
					retryFetcher = customFetcher
				} else if errClass == "timeout" && len(retryClient) > 0 && retryClient[0] != nil {
					retryFetcher = retryClient[0]
				} else {
					retryFetcher = defaultClient
				}
				batch, err = r.fetchSource(ctx, retryFetcher, nil, nctx, source, categoryDictionary, cursors)
			}
		}
	}

	entry := model.SourceHealthEntry{
		SourceID:         source.Source.SourceID,
		AuthorityName:    source.Source.AuthorityName,
		Type:             source.Type,
		FeedURL:          source.FeedURL,
		StartedAt:        startedAt.Format(time.RFC3339),
		FinishedAt:       time.Now().UTC().Format(time.RFC3339),
		RespETag:         respETag,
		RespLastModified: respLastModified,
	}

	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
		entry.ErrorClass, entry.NeedsReplacement, entry.DiscoveryAction = classifySourceError(err)
		return nil, entry
	}

	// Content-hash gate (fallback for servers without ETag/Last-Modified):
	// if the parsed alerts produce the same hash as last run, the feed
	// hasn't changed — return empty so the reconciler carries forward.
	if wm != nil && wm.LastStatus == "ok" && wm.LastContentHash != "" && len(batch) > 0 {
		hash := batchContentHash(batch)
		if hash == wm.LastContentHash {
			entry.Status = "ok"
			entry.ErrorClass = "not_modified"
			entry.FetchedCount = 0
			return nil, entry
		}
	}

	entry.Status = "ok"
	entry.FetchedCount = len(batch)
	return batch, entry
}

// acceptForType returns the Accept header value appropriate for the source type.
func acceptForType(sourceType string) string {
	switch sourceType {
	case "rss":
		return "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8"
	case "x":
		return "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	case "kev-json", "fbi-wanted-json", "travelwarning-json", "acled-json",
		"usgs-geojson", "eonet-json", "gdelt-json", "feodo-json", "ucdp-json":
		return "application/json"
	case "travelwarning-atom":
		return "application/atom+xml, application/xml;q=0.9, */*;q=0.8"
	default:
		return "text/html,application/xhtml+xml,*/*;q=0.8"
	}
}

// fetchOneSourceSlow fetches a source that needs the browser or full timeout.
// Unlike fetchOneSource, this passes the browser instance through for
// Interpol and browser-mode sources.
func (r Runner) fetchOneSourceSlow(ctx context.Context, client *fetch.Client, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store, cursors state.Cursors) ([]model.Alert, model.SourceHealthEntry) {
	startedAt := time.Now().UTC()
	fetcher := fetcherForSource(source, client, browser)

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
	case "usgs-geojson", "eonet-json", "feodo-json", "gdelt-json", "ucdp-json":
		return 4
	case "rss":
		return 3
	case "x":
		return 2
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
	case "x":
		return r.fetchX(ctx, fetcher, browser, nctx, source, categoryDictionary)
	case "html-list":
		return r.fetchHTML(ctx, fetcher, nctx, source, categoryDictionary)
	case "kev-json":
		return r.fetchKEV(ctx, fetcher, nctx, source)
	case "interpol-red-json", "interpol-yellow-json":
		return r.fetchInterpol(ctx, fetcher, browser, nctx, source, cursors)
	case "fbi-wanted-json":
		return r.fetchFBIWanted(ctx, fetcher, nctx, source)
	case "acled-json":
		return r.fetchACLED(ctx, fetcher, nctx, source)
	case "travelwarning-json":
		return r.fetchTravelWarningJSON(ctx, fetcher, nctx, source)
	case "travelwarning-atom":
		return r.fetchTravelWarningAtom(ctx, fetcher, nctx, source)
	case "telegram":
		return r.fetchTelegram(ctx, fetcher, nctx, source, categoryDictionary)
	case "usgs-geojson":
		return r.fetchUSGSGeoJSON(ctx, fetcher, nctx, source)
	case "eonet-json":
		return r.fetchEONET(ctx, fetcher, nctx, source)
	case "feodo-json":
		return r.fetchFeodo(ctx, fetcher, nctx, source)
	case "gdelt-json":
		return r.fetchGDELT(ctx, fetcher, nctx, source)
	case "ucdp-json":
		return r.fetchUCDP(ctx, nctx, source)
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
	items = filterFeedKeywords(items, source.IncludeKeywords, source.ExcludeKeywords, nctx.Config.StopWords)
	items, decisions := r.applyNoiseGate(source, items)
	sortFeedItemsNewest(items)
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
			if decision, ok := decisions[itemDecisionKey(item)]; ok {
				applyNoiseDecision(alert, decision)
			}
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
	items = filterKeywords(items, source.IncludeKeywords, source.ExcludeKeywords, nctx.Config.StopWords)
	items = filterCategoryItems(items, source, categoryDictionary)
	items, decisions := r.applyNoiseGate(source, items)
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
			VettingTimeoutMS:   nctx.Config.VettingTimeoutMS,
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
					if decision, ok := decisions[itemDecisionKey(classifiedItem.Item)]; ok {
						applyNoiseDecision(alert, decision)
					}
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
			if decision, ok := decisions[itemDecisionKey(item)]; ok {
				applyNoiseDecision(alert, decision)
			}
			out = append(out, *alert)
		}
	}
	out = downgradeNonActionable(out, source)
	return out, nil
}

func (r Runner) fetchTelegram(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, true, "text/html,application/xhtml+xml,*/*;q=0.8")
	if err != nil {
		return nil, err
	}
	channel := extractTelegramChannel(source.FeedURL)
	items := parse.ParseTelegram(string(body), channel)
	items = filterKeywords(items, source.IncludeKeywords, source.ExcludeKeywords, nctx.Config.StopWords)
	items = filterCategoryItems(items, source, categoryDictionary)
	items, decisions := r.applyNoiseGate(source, items)
	sortFeedItemsNewest(items)
	limit := perSourceLimit(nctx.Config, source)
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]model.Alert, 0, len(items))
	for _, item := range items {
		alert := normalize.HTMLItem(nctx, source, item)
		if alert != nil {
			if decision, ok := decisions[itemDecisionKey(item)]; ok {
				applyNoiseDecision(alert, decision)
			}
			out = append(out, *alert)
		}
	}
	out = downgradeNonActionable(out, source)
	return out, nil
}

func (r Runner) fetchX(ctx context.Context, fetcher fetch.Fetcher, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store) ([]model.Alert, error) {
	limit := perSourceLimit(nctx.Config, source)
	items := []parse.FeedItem{}
	if browser != nil {
		if captured, err := browser.CaptureJSONResponses(ctx, source.FeedURL, "UserTweets"); err == nil {
			for _, body := range captured {
				items = append(items, parseXTweetsFromGraphQLJSON(body, source.FeedURL)...)
			}
		}
	}
	if len(items) == 0 {
		body, finalURL, err := fetchWithFallbackURL(ctx, fetcher, source, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		if err != nil {
			return nil, err
		}
		items = parseXStatusAnchors(string(body), finalURL)
	}
	items = filterFeedKeywords(items, source.IncludeKeywords, source.ExcludeKeywords, nctx.Config.StopWords)
	items = filterCategoryItems(items, source, categoryDictionary)
	items, decisions := r.applyNoiseGate(source, items)
	sortFeedItemsNewest(items)
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]model.Alert, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Link) == "" {
			continue
		}
		alert := normalize.RSSItem(nctx, source, item)
		if alert != nil {
			if decision, ok := decisions[itemDecisionKey(item)]; ok {
				applyNoiseDecision(alert, decision)
			}
			out = append(out, *alert)
		}
	}
	out = downgradeNonActionable(out, source)
	return out, nil
}

func parseXStatusAnchors(body string, baseURL string) []parse.FeedItem {
	anchorRe := regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
	matches := anchorRe.FindAllStringSubmatch(body, -1)
	resolved, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]parse.FeedItem, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		href := strings.TrimSpace(match[1])
		if href == "" || strings.HasPrefix(href, "#") {
			continue
		}
		rawURL, err := url.Parse(href)
		if err != nil {
			continue
		}
		link := resolved.ResolveReference(rawURL).String()
		if !isXStatusURL(link) {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		title := parse.StripHTML(match[2])
		if strings.TrimSpace(title) == "" {
			title = xStatusTitleFallback(link)
		}
		out = append(out, parse.FeedItem{Title: title, Link: link, Summary: title})
	}
	return out
}

func isXStatusURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host != "x.com" && host != "twitter.com" {
		return false
	}
	path := strings.Trim(strings.TrimSpace(u.Path), "/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return false
	}
	if !strings.EqualFold(parts[1], "status") {
		return false
	}
	return strings.TrimSpace(parts[2]) != ""
}

func xStatusTitleFallback(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "X post"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 3 {
		return "X post by @" + parts[0]
	}
	return "X post"
}

func parseXTweetsFromGraphQLJSON(body []byte, feedURL string) []parse.FeedItem {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	handle := xHandleFromFeedURL(feedURL)
	seen := map[string]struct{}{}
	out := make([]parse.FeedItem, 0, 32)
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if legacyRaw, ok := v["legacy"]; ok {
				if legacy, ok := legacyRaw.(map[string]any); ok {
					id := strings.TrimSpace(anyToString(legacy["id_str"]))
					title := strings.TrimSpace(anyToString(legacy["full_text"]))
					published := xCreatedAtToRFC3339(anyToString(legacy["created_at"]))
					if id != "" {
						linkHandle := handle
						if linkHandle == "" {
							linkHandle = "i"
						}
						link := fmt.Sprintf("https://x.com/%s/status/%s", linkHandle, id)
						if _, exists := seen[link]; !exists {
							seen[link] = struct{}{}
							if title == "" {
								title = xStatusTitleFallback(link)
							}
							out = append(out, parse.FeedItem{
								Title:     title,
								Link:      link,
								Published: published,
								Summary:   title,
							})
						}
					}
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(decoded)
	return out
}

func anyToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func xCreatedAtToRFC3339(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse("Mon Jan 02 15:04:05 -0700 2006", raw)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339)
}

func xHandleFromFeedURL(feedURL string) string {
	u, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil {
		return ""
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	handle := strings.TrimSpace(parts[0])
	for _, reserved := range []string{"home", "explore", "i", "search", "settings", "compose"} {
		if strings.EqualFold(handle, reserved) {
			return ""
		}
	}
	return handle
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

func (r Runner) fetchACLED(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	if nctx.Config.ACLEDUsername == "" || nctx.Config.ACLEDPassword == "" {
		return nil, nil // skip silently when credentials not configured
	}

	// Obtain OAuth access token.
	token, err := acledOAuthToken(ctx, nctx.Config.ACLEDUsername, nctx.Config.ACLEDPassword)
	if err != nil {
		return nil, fmt.Errorf("ACLED OAuth: %w", err)
	}

	limit := perSourceLimit(nctx.Config, source)
	pageSize := 500
	var allAlerts []model.Alert

	// Build date range: last 7 days.
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7).Format("2006-01-02")
	to := now.Format("2006-01-02")

	for page := 1; len(allAlerts) < limit; page++ {
		pageURL := fmt.Sprintf("%s?_format=json&event_date=%s|%s&event_date_where=BETWEEN&order=desc&sort=event_date&page=%d&limit=%d",
			source.FeedURL, from, to, page, pageSize)
		body, err := acledAuthGet(ctx, pageURL, token)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		items, total, err := parse.ParseACLED(body)
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
			alert := normalize.ACLEDAlert(nctx, source, item)
			if alert != nil {
				allAlerts = append(allAlerts, *alert)
			}
		}
		if total > 0 && page*pageSize >= total {
			break
		}
		if len(items) < pageSize {
			break
		}
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return allAlerts, nil
		}
	}
	return allAlerts, nil
}

// acledOAuthToken obtains a Bearer token from ACLED's OAuth endpoint.
func acledOAuthToken(ctx context.Context, username, password string) (string, error) {
	data := url.Values{
		"username":   {username},
		"password":   {password},
		"grant_type": {"password"},
		"client_id":  {"acled"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://acleddata.com/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("OAuth token request failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}
	return tokenResp.AccessToken, nil
}

// acledAuthGet performs a GET request with ACLED Bearer token auth.
func acledAuthGet(ctx context.Context, reqURL, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("ACLED API 403: account lacks API access — register at https://developer.acleddata.com/")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ACLED API %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
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

func (r Runner) fetchUSGSGeoJSON(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseUSGSGeoJSON(body)
	if err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		alert := normalize.USGSAlert(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchEONET(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseEONET(body)
	if err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		alert := normalize.EONETAlert(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchFeodo(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetcher.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseFeodo(body)
	if err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		alert := normalize.FeodoAlert(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchGDELT(ctx context.Context, fetcher fetch.Fetcher, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	// GDELT supports multiple query URLs via feed_urls for multi-query merge.
	urls := source.FeedURLs
	if len(urls) == 0 && source.FeedURL != "" {
		urls = []string{source.FeedURL}
	}

	limit := perSourceLimit(nctx.Config, source)
	var allItems []parse.FeedItem
	seen := make(map[string]struct{})

	for _, qURL := range urls {
		body, err := fetcher.Text(ctx, qURL, source.FollowRedirects, "application/json")
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN %s: GDELT query failed: %v\n", source.Source.AuthorityName, err)
			continue
		}
		items, err := parse.ParseGDELT(body)
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN %s: GDELT parse failed: %v\n", source.Source.AuthorityName, err)
			continue
		}
		for _, item := range items {
			if _, dup := seen[item.Link]; dup {
				continue
			}
			seen[item.Link] = struct{}{}
			allItems = append(allItems, item)
		}
		// Brief pause between queries to be polite.
		if len(urls) > 1 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				break
			}
		}
	}

	allItems = filterFeedKeywords(allItems, source.IncludeKeywords, source.ExcludeKeywords, nctx.Config.StopWords)
	sortFeedItemsNewest(allItems)
	if len(allItems) > limit {
		allItems = allItems[:limit]
	}

	out := make([]model.Alert, 0, limit)
	for _, item := range allItems {
		if len(out) == limit {
			break
		}
		// Extract source country from tags (first non-"gdelt" tag is the country).
		var sourceCountry string
		for _, tag := range item.Tags {
			if tag != "gdelt" {
				sourceCountry = tag
				break
			}
		}
		alert := normalize.GDELTAlert(nctx, source, item, sourceCountry)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchUCDP(ctx context.Context, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	items, err := r.fetchUCDPItems(ctx, nctx, source)
	if err != nil {
		return nil, err
	}
	limit := perSourceLimit(nctx.Config, source)
	out := make([]model.Alert, 0, limit)
	for _, item := range items {
		if len(out) == limit {
			break
		}
		alert := normalize.UCDPAlert(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchUCDPItems(ctx context.Context, nctx normalize.Context, source model.RegistrySource) ([]parse.UCDPItem, error) {
	token := strings.TrimSpace(nctx.Config.UCDPAccessToken)
	if token == "" {
		// Silent drop by configuration: no token means UCDP is disabled.
		return nil, nil
	}
	ucdpCfg := nctx.Config
	// UCDP API responses are often slower than RSS/API feeds in the fast lanes.
	// Keep UCDP out of short-timeout behavior to avoid persistent stale cache.
	if ucdpCfg.HTTPTimeoutMS < 30000 {
		ucdpCfg.HTTPTimeoutMS = 30000
	}
	client := r.clientFactory(ucdpCfg)
	headers := map[string]string{
		"x-ucdp-access-token": token,
	}
	feedURL, explicitPage := ensureUCDPQuery(source.FeedURL, 100)
	body, err := client.TextWithHeaders(ctx, feedURL, source.FollowRedirects, "application/json", headers)
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseUCDP(body)
	if err != nil {
		return nil, err
	}
	if !explicitPage {
		totalPages := parseUCDPTotalPages(body)
		if totalPages > 1 {
			const recentTailPages = 8
			start := totalPages - recentTailPages
			if start < 0 {
				start = 0
			}
			recentItems := make([]parse.UCDPItem, 0, len(items)*recentTailPages)
			for page := start; page < totalPages; page++ {
				pageURL := setUCDPPage(feedURL, page)
				pageBody, pageErr := client.TextWithHeaders(ctx, pageURL, source.FollowRedirects, "application/json", headers)
				if pageErr != nil {
					continue
				}
				pageItems, parseErr := parse.ParseUCDP(pageBody)
				if parseErr != nil {
					continue
				}
				recentItems = append(recentItems, pageItems...)
			}
			if len(recentItems) > 0 {
				items = recentItems
			}
		}
	}
	return items, nil
}

func (r Runner) fetchUCDPCurrentConflictCountryIDs(ctx context.Context, nctx normalize.Context, source model.RegistrySource, windowDays int) ([]string, error) {
	token := strings.TrimSpace(nctx.Config.UCDPAccessToken)
	if token == "" {
		return nil, nil
	}
	ucdpCfg := nctx.Config
	if ucdpCfg.HTTPTimeoutMS < 30000 {
		ucdpCfg.HTTPTimeoutMS = 30000
	}
	client := r.clientFactory(ucdpCfg)
	headers := map[string]string{
		"x-ucdp-access-token": token,
	}
	endDate := nctx.Now.UTC().Format("2006-01-02")
	startDate := nctx.Now.UTC().AddDate(0, 0, -windowDays).Format("2006-01-02")
	filteredURL := withUCDPFilters(source.FeedURL, map[string]string{
		"StartDate": startDate,
		"EndDate":   endDate,
	})
	feedURL, explicitPage := ensureUCDPQuery(filteredURL, 1000)
	body, err := client.TextWithHeaders(ctx, feedURL, source.FollowRedirects, "application/json", headers)
	if err != nil {
		return nil, err
	}
	items, err := parse.ParseUCDP(body)
	if err != nil {
		return nil, err
	}
	countryIDs := collectUCDPCountryIDs(items)
	if !explicitPage {
		totalPages := parseUCDPTotalPages(body)
		const maxPages = 64
		if totalPages > 1 {
			if totalPages > maxPages {
				totalPages = maxPages
			}
			for page := 1; page < totalPages; page++ {
				pageURL := setUCDPPage(feedURL, page)
				pageBody, pageErr := client.TextWithHeaders(ctx, pageURL, source.FollowRedirects, "application/json", headers)
				if pageErr != nil {
					continue
				}
				pageItems, parseErr := parse.ParseUCDP(pageBody)
				if parseErr != nil {
					continue
				}
				countryIDs = append(countryIDs, collectUCDPCountryIDs(pageItems)...)
			}
		}
	}
	uniq := map[string]struct{}{}
	for _, id := range countryIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		uniq[id] = struct{}{}
	}
	out := make([]string, 0, len(uniq))
	for id := range uniq {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (r Runner) fetchUCDPItemsForCountries(ctx context.Context, nctx normalize.Context, source model.RegistrySource, countryIDs []string, windowDays int) ([]parse.UCDPItem, error) {
	token := strings.TrimSpace(nctx.Config.UCDPAccessToken)
	if token == "" || len(countryIDs) == 0 {
		return nil, nil
	}
	ucdpCfg := nctx.Config
	if ucdpCfg.HTTPTimeoutMS < 30000 {
		ucdpCfg.HTTPTimeoutMS = 30000
	}
	client := r.clientFactory(ucdpCfg)
	headers := map[string]string{
		"x-ucdp-access-token": token,
	}
	endDate := nctx.Now.UTC().Format("2006-01-02")
	startDate := nctx.Now.UTC().AddDate(0, 0, -windowDays).Format("2006-01-02")
	const maxPagesPerCountry = 64
	all := make([]parse.UCDPItem, 0)
	for _, countryID := range countryIDs {
		countryID = strings.TrimSpace(countryID)
		if countryID == "" {
			continue
		}
		filteredURL := withUCDPFilters(source.FeedURL, map[string]string{
			"Country":   countryID,
			"StartDate": startDate,
			"EndDate":   endDate,
		})
		feedURL, explicitPage := ensureUCDPQuery(filteredURL, 1000)
		body, err := client.TextWithHeaders(ctx, feedURL, source.FollowRedirects, "application/json", headers)
		if err != nil {
			return nil, err
		}
		items, err := parse.ParseUCDP(body)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if !explicitPage {
			totalPages := parseUCDPTotalPages(body)
			if totalPages > maxPagesPerCountry {
				totalPages = maxPagesPerCountry
			}
			for page := 1; page < totalPages; page++ {
				pageURL := setUCDPPage(feedURL, page)
				pageBody, pageErr := client.TextWithHeaders(ctx, pageURL, source.FollowRedirects, "application/json", headers)
				if pageErr != nil {
					continue
				}
				pageItems, parseErr := parse.ParseUCDP(pageBody)
				if parseErr != nil {
					continue
				}
				all = append(all, pageItems...)
			}
		}
	}
	return dedupeUCDPItems(all), nil
}

func (r Runner) syncZoneBriefings(ctx context.Context, cfg config.Config, sources []model.RegistrySource, store *sourcedb.DB, forceRefresh bool) error {
	if strings.TrimSpace(cfg.ZoneBriefingsOutputPath) == "" {
		return nil
	}
	now := time.Now().UTC()
	ttl := time.Duration(cfg.ZoneBriefingsTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if store != nil {
		cached, err := store.LoadZoneBriefingsCache(ctx, now)
		if err != nil {
			return err
		}
		if cached.Found {
			if err := r.renderZoneBriefingsArtifacts(cfg, cached.Briefings); err != nil {
				return err
			}
			if !forceRefresh && !cached.Stale {
				return nil
			}
		}
	}
	var ucdpSource *model.RegistrySource
	for i := range sources {
		if sources[i].Type == "ucdp-json" {
			ucdpSource = &sources[i]
			break
		}
	}
	if ucdpSource == nil {
		if store != nil {
			if err := store.UpsertZoneBriefingsCache(ctx, []model.ZoneBriefingRecord{}, now, ttl); err != nil {
				return err
			}
		}
		return r.renderZoneBriefingsArtifacts(cfg, []model.ZoneBriefingRecord{})
	}
	nctx := normalize.Context{Config: cfg, Now: now}
	countryIDs, err := r.fetchUCDPCurrentConflictCountryIDs(ctx, nctx, *ucdpSource, 30)
	if err != nil {
		return err
	}
	items, err := r.fetchUCDPItemsForCountries(ctx, nctx, *ucdpSource, countryIDs, 120)
	if err != nil {
		return err
	}
	briefings := zonebrief.Build(items, nctx.Now)
	if store != nil {
		if err := store.UpsertZoneBriefingsCache(ctx, briefings, now, ttl); err != nil {
			return err
		}
	}
	if err := r.renderZoneBriefingsArtifacts(cfg, briefings); err != nil {
		return err
	}
	return nil
}

func withUCDPFilters(raw string, filters map[string]string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return raw
	}
	q := u.Query()
	for k, v := range filters {
		if strings.TrimSpace(v) == "" {
			continue
		}
		q.Set(k, strings.TrimSpace(v))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func collectUCDPCountryIDs(items []parse.UCDPItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.CountryID)
		if id == "" {
			if ref, ok := parse.UCDPCountryRefByISO2(item.CountryCode); ok && strings.TrimSpace(ref.ID) != "" {
				id = strings.TrimSpace(ref.ID)
			}
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func dedupeUCDPItems(items []parse.UCDPItem) []parse.UCDPItem {
	out := make([]parse.UCDPItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		key := strings.Join([]string{
			strings.TrimSpace(item.Link),
			strings.TrimSpace(item.Published),
			strings.TrimSpace(item.CountryID),
			strings.TrimSpace(item.SideA),
			strings.TrimSpace(item.SideB),
		}, "|")
		if strings.Trim(key, "|") == "" {
			key = strings.TrimSpace(item.Title)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (r Runner) renderZoneBriefingsArtifacts(cfg config.Config, briefings []model.ZoneBriefingRecord) error {
	if err := output.WriteZoneBriefings(cfg.ZoneBriefingsOutputPath, briefings); err != nil {
		return err
	}
	geoDir := filepath.Join(filepath.Dir(cfg.ZoneBriefingsOutputPath), "geo")
	conflictZones, terrorZones := zonebrief.BuildConflictZonesGeoJSON(briefings), zonebrief.BuildTerrorZonesGeoJSON(briefings)
	if strings.TrimSpace(cfg.CountryBoundariesPath) != "" {
		if data, err := zonebrief.BuildConflictZonesGeoJSONFromBoundaries(briefings, cfg.CountryBoundariesPath); err == nil {
			conflictZones = data
		}
		if data, err := zonebrief.BuildTerrorZonesGeoJSONFromBoundaries(briefings, cfg.CountryBoundariesPath); err == nil {
			terrorZones = data
		}
	}
	if err := writeJSONArtifact(filepath.Join(geoDir, "conflict-zones.geojson"), conflictZones); err != nil {
		return err
	}
	if err := writeJSONArtifact(filepath.Join(geoDir, "terrorism-zones.geojson"), terrorZones); err != nil {
		return err
	}
	return nil
}

func writeJSONArtifact(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func ensureUCDPQuery(raw string, defaultPageSize int) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return raw, false
	}
	q := u.Query()
	if strings.TrimSpace(q.Get("pagesize")) == "" {
		q.Set("pagesize", strconv.Itoa(defaultPageSize))
	}
	explicitPage := strings.TrimSpace(q.Get("page")) != ""
	u.RawQuery = q.Encode()
	return u.String(), explicitPage
}

func setUCDPPage(raw string, page int) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return raw
	}
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.String()
}

func parseUCDPTotalPages(body []byte) int {
	var envelope struct {
		TotalPages int `json:"TotalPages"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0
	}
	if envelope.TotalPages < 0 {
		return 0
	}
	return envelope.TotalPages
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

func itemDecisionKey(item parse.FeedItem) string {
	return strings.ToLower(strings.TrimSpace(item.Title + "|" + item.Link))
}

func (r Runner) applyNoiseGate(source model.RegistrySource, items []parse.FeedItem) ([]parse.FeedItem, map[string]noisegate.Decision) {
	if r.noiseGate == nil || len(items) == 0 {
		return items, map[string]noisegate.Decision{}
	}
	out := make([]parse.FeedItem, 0, len(items))
	decisions := make(map[string]noisegate.Decision, len(items))
	for _, item := range items {
		decision := r.noiseGate.Evaluate(source, item)
		if decision.Outcome == noisegate.OutcomeDrop {
			continue
		}
		key := itemDecisionKey(item)
		decisions[key] = decision
		out = append(out, item)
	}
	return out, decisions
}

func applyNoiseDecision(alert *model.Alert, decision noisegate.Decision) {
	if alert == nil {
		return
	}
	if alert.Triage == nil {
		alert.Triage = &model.Triage{}
	}
	if alert.Triage.Metadata == nil {
		alert.Triage.Metadata = &model.TriageMetadata{}
	}
	alert.Triage.Metadata.NoiseDecision = string(decision.Outcome)
	alert.Triage.Metadata.NoisePolicyVersion = decision.PolicyVersion
	alert.Triage.Metadata.NoisePolicyVariant = decision.PolicyVariant
	alert.Triage.Metadata.NoiseBlockScore = decision.BlockScore
	alert.Triage.Metadata.NoiseScore = decision.NoiseScore
	alert.Triage.Metadata.NoiseActionability = decision.ActionabilityScore
	alert.Triage.Metadata.NoiseReasons = append([]string{}, decision.Reasons...)
	alert.Triage.Metadata.NoiseDecisionTimestamp = time.Now().UTC().Format(time.RFC3339)
	if decision.Outcome == noisegate.OutcomeDowngrade {
		alert.Severity = "info"
		alert.Category = "informational"
		alert.Triage.WeakSignals = append([]string{"noise-gate downgraded to informational"}, alert.Triage.WeakSignals...)
	}
}

func filterKeywords(items []parse.FeedItem, include []string, exclude []string, globalExclude ...[]string) []parse.FeedItem {
	include = normalizeKeywords(include)
	exclude = normalizeKeywords(exclude)
	global := []string{}
	for _, extra := range globalExclude {
		global = append(global, normalizeKeywords(extra)...)
	}
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
		if len(global) > 0 && shouldExcludeByGlobalStopWords(item, global) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterFeedKeywords(items []parse.FeedItem, include []string, exclude []string, globalExclude ...[]string) []parse.FeedItem {
	include = normalizeKeywords(include)
	exclude = normalizeKeywords(exclude)
	global := []string{}
	for _, extra := range globalExclude {
		global = append(global, normalizeKeywords(extra)...)
	}
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
		if len(global) > 0 && shouldExcludeByGlobalStopWords(item, global) {
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

var stopWordActionableOverrides = []string{
	"sexual assault",
	"sex trafficking",
	"human trafficking",
	"sextortion",
	"rape",
	"molestation",
	"child exploitation",
	"domestic violence",
	"police appeal",
	"wanted",
	"missing person",
}

func shouldExcludeByGlobalStopWords(item parse.FeedItem, stopWords []string) bool {
	hay := strings.ToLower(strings.Join([]string{
		item.Title,
		item.Summary,
		item.Author,
		strings.Join(item.Tags, " "),
		item.Link,
	}, " "))
	if !containsStopWord(hay, stopWords) {
		return false
	}
	context := strings.ToLower(strings.TrimSpace(item.Title + " " + item.Summary))
	for _, phrase := range stopWordActionableOverrides {
		if strings.Contains(context, phrase) {
			return false
		}
	}
	return true
}

func containsStopWord(hay string, needles []string) bool {
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		if strings.Contains(needle, " ") {
			if strings.Contains(hay, needle) {
				return true
			}
			continue
		}
		if containsWholeWord(hay, needle) {
			return true
		}
	}
	return false
}

func containsWholeWord(hay string, needle string) bool {
	start := 0
	for {
		idx := strings.Index(hay[start:], needle)
		if idx < 0 {
			return false
		}
		idx += start
		leftOK := idx == 0 || !isWordChar(rune(hay[idx-1]))
		end := idx + len(needle)
		rightOK := end >= len(hay) || !isWordChar(rune(hay[end]))
		if leftOK && rightOK {
			return true
		}
		start = idx + len(needle)
		if start >= len(hay) {
			return false
		}
	}
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
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
		// Keep curated sources untouched; broad sources without include keywords
		// still need title-level actionability checks.
		if len(source.IncludeKeywords) > 0 {
			return alerts
		}
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

type noiseMetricsDocument struct {
	GeneratedAt            string                          `json:"generated_at"`
	NoisePolicyVersion     string                          `json:"noise_policy_version,omitempty"`
	LaneDistribution       map[string]int                  `json:"lane_distribution"`
	LaneDistributionDrift  map[string]float64              `json:"lane_distribution_drift,omitempty"`
	GeoConfidenceAverage   float64                         `json:"geo_confidence_average"`
	GeoConfidenceDrift     float64                         `json:"geo_confidence_drift,omitempty"`
	GeoConfidenceBySource  map[string]float64              `json:"geo_confidence_by_source,omitempty"`
	SourcePrecision        []sourcedb.NoiseSourcePrecision `json:"source_precision,omitempty"`
	SourceLaneDistribution map[string]map[string]int       `json:"source_lane_distribution,omitempty"`
}

func (r Runner) writeNoiseMetrics(ctx context.Context, cfg config.Config, alerts []model.Alert, runStore *sourcedb.DB) error {
	path := strings.TrimSpace(cfg.NoiseMetricsOutputPath)
	if path == "" {
		return nil
	}
	doc := noiseMetricsDocument{
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
		LaneDistribution:       map[string]int{"alarm": 0, "intel": 0, "info": 0},
		LaneDistributionDrift:  map[string]float64{},
		GeoConfidenceBySource:  map[string]float64{},
		SourceLaneDistribution: map[string]map[string]int{},
	}
	if r.noiseGate != nil {
		doc.NoisePolicyVersion = r.noiseGate.Version()
	}
	previous := readNoiseMetrics(path)

	sumConfidence := 0.0
	for _, alert := range alerts {
		lane := strings.ToLower(strings.TrimSpace(string(alert.SignalLane)))
		if lane == "" {
			lane = "intel"
		}
		doc.LaneDistribution[lane]++
		if _, ok := doc.SourceLaneDistribution[alert.SourceID]; !ok {
			doc.SourceLaneDistribution[alert.SourceID] = map[string]int{"alarm": 0, "intel": 0, "info": 0}
		}
		doc.SourceLaneDistribution[alert.SourceID][lane]++
		sumConfidence += alert.EventGeoConfidence
		doc.GeoConfidenceBySource[alert.SourceID] += alert.EventGeoConfidence
	}
	if len(alerts) > 0 {
		doc.GeoConfidenceAverage = round3(sumConfidence / float64(len(alerts)))
	}
	for sourceID, total := range doc.GeoConfidenceBySource {
		count := 0
		for _, alert := range alerts {
			if alert.SourceID == sourceID {
				count++
			}
		}
		if count > 0 {
			doc.GeoConfidenceBySource[sourceID] = round3(total / float64(count))
		}
	}

	totalCurrent := len(alerts)
	totalPrevious := 0
	if previous != nil {
		for _, v := range previous.LaneDistribution {
			totalPrevious += v
		}
		doc.GeoConfidenceDrift = round3(doc.GeoConfidenceAverage - previous.GeoConfidenceAverage)
		for lane, count := range doc.LaneDistribution {
			curPct := 0.0
			if totalCurrent > 0 {
				curPct = float64(count) / float64(totalCurrent)
			}
			prevPct := 0.0
			if totalPrevious > 0 {
				prevPct = float64(previous.LaneDistribution[lane]) / float64(totalPrevious)
			}
			doc.LaneDistributionDrift[lane] = round3(curPct - prevPct)
		}
	}

	if runStore != nil {
		if precision, err := runStore.NoiseFeedbackPrecisionBySource(ctx); err == nil {
			doc.SourcePrecision = precision
		}
	}

	return writeJSONFile(path, doc)
}

func readNoiseMetrics(path string) *noiseMetricsDocument {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	var doc noiseMetricsDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	if doc.LaneDistribution == nil {
		doc.LaneDistribution = map[string]int{}
	}
	return &doc
}

func round3(v float64) float64 {
	return float64(int(v*1000+0.5)) / 1000
}

func writeJSONFile(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func (r Runner) refreshMilitaryBasesLayer(ctx context.Context, cfg config.Config, client *fetch.Client) error {
	if !cfg.MilitaryBasesEnabled {
		return nil
	}
	url := strings.TrimSpace(cfg.MilitaryBasesURL)
	path := strings.TrimSpace(cfg.MilitaryBasesOutputPath)
	if url == "" || path == "" {
		return nil
	}
	if !shouldRefreshOutput(path, cfg.MilitaryBasesRefreshHours, time.Now().UTC()) {
		return nil
	}
	body, err := client.Text(ctx, url, true, "application/geo+json,application/json;q=0.9,*/*;q=0.8")
	if err != nil {
		return err
	}
	fetchedFeatures, err := geoJSONFeaturesFromBytes(body)
	if err != nil {
		return fmt.Errorf("parse fetched geojson: %w", err)
	}
	if len(fetchedFeatures) == 0 {
		return fmt.Errorf("fetched geojson has no features")
	}

	sets := [][]json.RawMessage{fetchedFeatures}
	if existingFeatures, err := geoJSONFeaturesFromFile(path); err == nil && len(existingFeatures) > 0 {
		sets = append(sets, existingFeatures)
	}
	canonicalPath := filepath.Join("public", "geo", "military-bases.geojson")
	if filepath.Clean(path) != filepath.Clean(canonicalPath) {
		if canonicalFeatures, err := geoJSONFeaturesFromFile(canonicalPath); err == nil && len(canonicalFeatures) > 0 {
			sets = append(sets, canonicalFeatures)
		}
	}
	mergedFeatures := mergeGeoJSONFeatures(sets...)
	if len(mergedFeatures) == 0 {
		return fmt.Errorf("merged military-bases layer has no features")
	}
	doc := struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}{
		Type:     "FeatureCollection",
		Features: mergedFeatures,
	}
	mergedBody, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal merged geojson: %w", err)
	}
	mergedBody = append(mergedBody, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, mergedBody, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(r.stderr, "Military bases layer refreshed: fetched=%d merged=%d -> %s\n", len(fetchedFeatures), len(mergedFeatures), path)
	return nil
}

func geoJSONFeaturesFromFile(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return geoJSONFeaturesFromBytes(data)
}

func geoJSONFeaturesFromBytes(data []byte) ([]json.RawMessage, error) {
	var doc struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Features, nil
}

func mergeGeoJSONFeatures(featureSets ...[]json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0)
	seen := map[string]struct{}{}
	for _, set := range featureSets {
		for _, raw := range set {
			key := geoJSONFeatureKey(raw)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, raw)
		}
	}
	return out
}

func geoJSONFeatureKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var feature map[string]any
	if err := json.Unmarshal(raw, &feature); err != nil {
		sum := sha1.Sum(raw)
		return "raw:" + hex.EncodeToString(sum[:])
	}

	if id, ok := feature["id"]; ok {
		idKey := strings.TrimSpace(fmt.Sprintf("%v", id))
		if idKey != "" && idKey != "<nil>" {
			return "id:" + strings.ToLower(idKey)
		}
	}

	props, _ := feature["properties"].(map[string]any)
	for _, key := range []string{"id", "ID", "site_id", "SITE_ID", "OBJECTID", "objectid", "full_name", "FULLNAME", "name", "NAME"} {
		if props == nil {
			continue
		}
		if rawValue, ok := props[key]; ok {
			value := strings.TrimSpace(fmt.Sprintf("%v", rawValue))
			if value != "" && value != "<nil>" {
				return "prop:" + strings.ToLower(key) + ":" + strings.ToLower(value)
			}
		}
	}

	sum := sha1.Sum(raw)
	return "raw:" + hex.EncodeToString(sum[:])
}

func shouldRefreshOutput(path string, refreshHours int, now time.Time) bool {
	if refreshHours <= 0 {
		refreshHours = 168
	}
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	age := now.Sub(info.ModTime().UTC())
	return age >= time.Duration(refreshHours)*time.Hour
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
	limit := cfg.MaxPerSource
	if source.MaxItems > 0 {
		limit = source.MaxItems
	}
	if limit <= 0 {
		limit = 20
	}
	// Stream-like sources should stay in a small rolling window per run.
	if strings.ToLower(strings.TrimSpace(source.Type)) != "html-list" && cfg.RecentWindowPerSource > 0 && limit > cfg.RecentWindowPerSource {
		limit = cfg.RecentWindowPerSource
	}
	return limit
}

func sortFeedItemsNewest(items []parse.FeedItem) {
	sort.SliceStable(items, func(i, j int) bool {
		ti := parseFeedPublished(items[i].Published)
		tj := parseFeedPublished(items[j].Published)
		if ti.IsZero() && tj.IsZero() {
			return i < j
		}
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.After(tj)
	})
}

func parseFeedPublished(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.ANSIC,
		"2006-01-02",
	}
	for _, format := range formats {
		if parsed, err := time.Parse(format, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func inferHTTPStatus(entry model.SourceHealthEntry) int {
	if entry.Status == "ok" {
		return http.StatusOK
	}
	if entry.Error == "" {
		return 0
	}
	const marker = "status "
	i := strings.Index(strings.ToLower(entry.Error), marker)
	if i < 0 {
		return 0
	}
	start := i + len(marker)
	end := start
	for end < len(entry.Error) && entry.Error[end] >= '0' && entry.Error[end] <= '9' {
		end++
	}
	if end == start {
		return 0
	}
	code, err := strconv.Atoi(entry.Error[start:end])
	if err != nil {
		return 0
	}
	return code
}

func batchContentHash(batch []model.Alert) string {
	if len(batch) == 0 {
		return ""
	}
	parts := make([]string, 0, len(batch))
	for _, alert := range batch {
		parts = append(parts, alert.AlertID+"|"+alert.CanonicalURL+"|"+alert.Title)
	}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func recordSourceRun(ctx context.Context, store *sourcedb.DB, source model.RegistrySource, entry model.SourceHealthEntry, batch []model.Alert, httpStatus int, meta map[string]any) {
	if store == nil {
		return
	}
	if httpStatus == 0 {
		httpStatus = inferHTTPStatus(entry)
	}
	in := sourcedb.SourceRunInput{
		SourceID:      source.Source.SourceID,
		RunStartedAt:  entry.StartedAt,
		RunFinishedAt: entry.FinishedAt,
		Status:        entry.Status,
		HTTPStatus:    httpStatus,
		FetchedCount:  entry.FetchedCount,
		Error:         entry.Error,
		ErrorClass:    entry.ErrorClass,
		ContentHash:   batchContentHash(batch),
		ETag:          entry.RespETag,
		LastModified:  entry.RespLastModified,
		Metadata:      meta,
	}
	_ = store.RecordSourceRun(ctx, in)
}

func (r Runner) shouldSkipHTMLScrape(ctx context.Context, cfg config.Config, client *fetch.Client, store *sourcedb.DB, source model.RegistrySource, now time.Time) (bool, int, string) {
	if cfg.HTMLScrapeIntervalHours <= 0 {
		return false, 0, ""
	}
	probeStatus, err := client.HeadStatus(ctx, source.FeedURL, source.FollowRedirects)
	if err != nil {
		return false, 0, ""
	}
	wm, ok, err := store.GetSourceWatermark(ctx, source.Source.SourceID)
	if err != nil || !ok {
		return false, probeStatus, ""
	}
	if probeStatus < 200 || probeStatus >= 300 {
		return false, probeStatus, ""
	}
	lastSuccess, err := time.Parse(time.RFC3339, strings.TrimSpace(wm.LastSuccessAt))
	if err != nil || lastSuccess.IsZero() {
		return false, probeStatus, ""
	}
	nextDue := lastSuccess.Add(time.Duration(cfg.HTMLScrapeIntervalHours) * time.Hour)
	if now.Before(nextDue) {
		return true, probeStatus, fmt.Sprintf("html cadence gate (%dh)", cfg.HTMLScrapeIntervalHours)
	}
	return false, probeStatus, ""
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

	// Execute one pass immediately so discovery starts on boot.
	runOnce()

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
		// Feed may require auth temporarily; keep retrying.
		return "unauthorized", false, "retry"
	case strings.Contains(msg, "status 522"):
		// Origin timeout is often transient for stressed sources.
		return "origin_unreachable", false, "retry"
	case strings.Contains(msg, "stopped after 10 redirects"):
		// Redirect loop — feed URL is broken.
		return "redirect_loop", true, "dead_letter"
	case strings.Contains(msg, "status 301"), strings.Contains(msg, "status 302"), strings.Contains(msg, "status 307"), strings.Contains(msg, "status 308"):
		// Redirects should be followed automatically — if we still see
		// one here it means the chain exceeded 10 hops.
		return "redirect", true, "dead_letter"
	case strings.Contains(msg, "status 403"):
		// WAF/country blocks can be temporary or path-specific.
		return "blocked", false, "retry"
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
