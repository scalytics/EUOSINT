// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/collector/registry"
	"github.com/scalytics/euosint/internal/collector/vet"
	"github.com/scalytics/euosint/internal/sourcedb"
)

// DiscoveredSource represents a newly discovered OSINT feed candidate.
type DiscoveredSource struct {
	FeedURL       string `json:"feed_url"`
	FeedType      string `json:"feed_type"`          // "rss", "atom", or "html-list"
	AuthorityType string `json:"authority_type"`     // "cert", "police", "national_security", etc.
	Category      string `json:"suggested_category"` // suggested category for registry
	OrgName       string `json:"org_name"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code,omitempty"`
	TeamURL       string `json:"team_url"`
	DiscoveredVia string `json:"discovered_via"`
}

// Run executes the candidate crawler pipeline: reads the candidate intake JSON,
// probes each candidate for stable feeds or HTML listing pages, skips dead-letter
// entries, deduplicates against the live registry, and writes a discovery report.
func Run(ctx context.Context, cfg config.Config, stdout io.Writer, stderr io.Writer) error {
	discoveryCfg := cfg
	if discoveryCfg.HTTPTimeoutMS < 45000 {
		discoveryCfg.HTTPTimeoutMS = 45000
	}
	client := fetch.New(discoveryCfg)
	var searchClient *vet.Client
	var sourceVetter *vet.Vetter
	var browser *fetch.BrowserClient
	if cfg.VettingEnabled {
		sourceVetter = vet.New(cfg)
	} else if cfg.SourceVettingRequired {
		fmt.Fprintf(stderr, "Source vetting is required; discovery will not auto-promote candidates without SOURCE_VETTING_ENABLED=true\n")
	}
	if cfg.SearchDiscoveryEnabled {
		searchClient = vet.NewClient(cfg)
	}
	if cfg.DDGSearchEnabled && cfg.BrowserEnabled {
		b, err := fetch.NewBrowser(fetch.BrowserOptions{
			TimeoutMS:           cfg.BrowserTimeoutMS,
			WSURL:               cfg.BrowserWSURL,
			MaxConcurrency:      cfg.BrowserMaxConcurrency,
			ConnectRetries:      cfg.BrowserConnectRetries,
			ConnectRetryDelayMS: cfg.BrowserConnectRetryDelayMS,
		})
		if err != nil {
			fmt.Fprintf(stderr, "WARN DDG search disabled (browser init failed): %v\n", err)
		} else {
			browser = b
			if warning := browser.Warning(); warning != "" {
				fmt.Fprintf(stderr, "WARN browser: %s\n", warning)
			}
			defer browser.Close()
		}
	}

	// Load existing registry for deduplication and gap analysis.
	existing := map[string]struct{}{}
	var registrySources []model.RegistrySource
	if sources, err := registry.Load(cfg.RegistryPath); err == nil {
		registrySources = sources
		for _, src := range sources {
			if src.FeedURL != "" {
				existing[normalizeURL(src.FeedURL)] = struct{}{}
			}
			for _, u := range src.FeedURLs {
				existing[normalizeURL(u)] = struct{}{}
			}
		}
	}

	fmt.Fprintf(stderr, "Starting source discovery (existing registry has %d feed URLs)\n", len(existing))

	var discovered []DiscoveredSource
	totalCandidatesSeen := 0
	totalVetted := 0
	totalPromoted := 0
	rejectionReasons := map[string]int{}
	dead := loadDeadLetterQueue(cfg.ReplacementQueuePath)
	fmt.Fprintf(stderr, "Dead-letter queue: %d sources will be skipped\n", len(dead))
	seededCandidates, err := generateAutonomousCandidates(ctx, cfg, client, browser, searchClient, dead, registrySources, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "WARN autonomous candidate discovery failed: %v\n", err)
	}
	candidates := mergeCandidates(loadCandidateQueue(cfg.CandidateQueuePath), seededCandidates, existing, dead)
	fmt.Fprintf(stderr, "Candidate queue: %d sources queued for crawl\n", len(candidates))
	remainingCandidates := make([]model.SourceCandidate, 0, len(candidates))
	promotedSources := make([]model.RegistrySource, 0)
	var mu sync.Mutex
	addRemaining := func(candidate model.SourceCandidate) {
		mu.Lock()
		remainingCandidates = append(remainingCandidates, candidate)
		mu.Unlock()
	}
	tryReserveFeedURL := func(raw string) bool {
		norm := normalizeURL(raw)
		if norm == "" {
			return false
		}
		mu.Lock()
		defer mu.Unlock()
		if _, ok := existing[norm]; ok {
			return false
		}
		existing[norm] = struct{}{}
		return true
	}

	workers := cfg.FetchWorkers
	if workers <= 0 {
		workers = 12
	}
	if workers > len(candidates) && len(candidates) > 0 {
		workers = len(candidates)
	}
	work := make(chan model.SourceCandidate, len(candidates))
	for _, candidate := range candidates {
		work <- candidate
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range work {
				if ctx.Err() != nil {
					return
				}

				mu.Lock()
				totalCandidatesSeen++
				mu.Unlock()

				if !passesDiscoveryHygiene(candidate.AuthorityName, firstNonEmpty(candidate.BaseURL, candidate.URL), candidate.AuthorityType) {
					continue
				}
				if isDeadLettered(candidate, dead) {
					continue
				}

				baseURL := candidateBaseURL(candidate)
				if baseURL == "" {
					continue
				}
				promotedForCandidate := false
				discoveredForCandidate := false
				results := ProbeFeeds(ctx, client, baseURL)
				for _, r := range results {
					if !tryReserveFeedURL(r.FeedURL) {
						continue
					}
					discoveredForCandidate = true
					found := DiscoveredSource{
						FeedURL:       r.FeedURL,
						FeedType:      r.FeedType,
						AuthorityType: candidate.AuthorityType,
						Category:      candidate.Category,
						OrgName:       candidate.AuthorityName,
						Country:       candidate.Country,
						CountryCode:   candidate.CountryCode,
						TeamURL:       baseURL,
						DiscoveredVia: "candidate-queue",
					}
					mu.Lock()
					discovered = append(discovered, found)
					mu.Unlock()
					if sourceVetter == nil {
						continue
					}
					promoted, verdict, err := vetAndPromote(ctx, cfg, client, sourceVetter, candidate, found)
					mu.Lock()
					totalVetted++
					mu.Unlock()
					if err != nil {
						fmt.Fprintf(stderr, "WARN source vetting failed for %s: %v\n", found.FeedURL, err)
						continue
					}
					if promoted != nil {
						mu.Lock()
						promotedSources = append(promotedSources, *promoted)
						totalPromoted++
						mu.Unlock()
						promotedForCandidate = true
						fmt.Fprintf(stderr, "Promoted source %s via %s (%s)\n", promoted.Source.SourceID, cfg.VettingProvider, verdict.Reason)
					} else if strings.TrimSpace(verdict.Reason) != "" {
						mu.Lock()
						rejectionReasons[verdict.Reason]++
						mu.Unlock()
					}
				}
				if len(results) == 0 {
					target := strings.TrimSpace(candidate.URL)
					if target == "" {
						target = baseURL
					}
					if probeHTMLPage(ctx, client, target) && tryReserveFeedURL(target) {
						discoveredForCandidate = true
						feedType := discoveredFeedTypeFromURL(target)
						found := DiscoveredSource{
							FeedURL:       target,
							FeedType:      feedType,
							AuthorityType: candidate.AuthorityType,
							Category:      candidate.Category,
							OrgName:       candidate.AuthorityName,
							Country:       candidate.Country,
							CountryCode:   candidate.CountryCode,
							TeamURL:       baseURL,
							DiscoveredVia: "candidate-queue",
						}
						mu.Lock()
						discovered = append(discovered, found)
						mu.Unlock()
						if sourceVetter != nil {
							promoted, verdict, err := vetAndPromote(ctx, cfg, client, sourceVetter, candidate, found)
							mu.Lock()
							totalVetted++
							mu.Unlock()
							if err != nil {
								fmt.Fprintf(stderr, "WARN source vetting failed for %s: %v\n", found.FeedURL, err)
							} else if promoted != nil {
								mu.Lock()
								promotedSources = append(promotedSources, *promoted)
								totalPromoted++
								mu.Unlock()
								promotedForCandidate = true
								fmt.Fprintf(stderr, "Promoted source %s via %s (%s)\n", promoted.Source.SourceID, cfg.VettingProvider, verdict.Reason)
							} else if strings.TrimSpace(verdict.Reason) != "" {
								mu.Lock()
								rejectionReasons[verdict.Reason]++
								mu.Unlock()
							}
						}
					}
				}
				if discoveredForCandidate {
					// Candidate is extractable and was already processed; don't
					// keep retrying it in the pending queue.
					continue
				}
				if !promotedForCandidate && shouldRetryCandidateQueue(ctx, client, candidate, baseURL) {
					addRemaining(candidate)
				}
			}
		}()
	}
	wg.Wait()

	// Write results.
	fmt.Fprintf(stderr, "Discovery finished: %d new candidates\n", len(discovered))
	if sourceVetter != nil {
		fmt.Fprintf(stderr, "Discovery KPI: candidates=%d vetted=%d promoted=%d\n", totalCandidatesSeen, totalVetted, totalPromoted)
		for reason, count := range rejectionReasons {
			fmt.Fprintf(stderr, "  rejection: %dx %s\n", count, reason)
		}
	}
	if err := WriteReport(cfg.DiscoverOutputPath, discovered, len(existing), stdout); err != nil {
		return err
	}
	if err := promoteDiscoveredSources(ctx, cfg.RegistryPath, promotedSources); err != nil {
		return err
	}
	if err := writeCandidateQueue(cfg.CandidateQueuePath, remainingCandidates); err != nil {
		return err
	}
	return nil
}

func generateAutonomousCandidates(ctx context.Context, cfg config.Config, client *fetch.Client, browser *fetch.BrowserClient, searchClient searchCompleter, dead []model.SourceReplacementCandidate, registrySources []model.RegistrySource, stderr io.Writer) ([]model.SourceCandidate, error) {
	candidates := make([]model.SourceCandidate, 0)
	var failures []string
	var slowSkips []string
	sovereignSeeds := loadSovereignSeedCandidates(cfg.SovereignSeedPath)
	if len(sovereignSeeds) > 0 {
		fmt.Fprintf(stderr, "Curated sovereign official-statement seeds: %d candidate URLs\n", len(sovereignSeeds))
		candidates = append(candidates, sovereignSeeds...)
	}
	runStructured, nextAt, err := shouldRunStructuredDiscovery(cfg, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(stderr, "WARN structured discovery cadence check failed: %v\n", err)
		runStructured = true
	}
	structuredSuccess := false
	if !runStructured {
		fmt.Fprintf(stderr, "Structured discovery skipped until %s (FIRST/Wikidata cadence gate)\n", nextAt.Format(time.RFC3339))
	} else {
		teams, err := FetchFIRSTTeams(ctx, cfg, client)
		if err != nil {
			if isDiscoveryTimeout(err) {
				slowSkips = append(slowSkips, "FIRST.org")
			} else {
				failures = append(failures, fmt.Sprintf("FIRST.org: %v", err))
			}
		} else {
			structuredSuccess = true
			fmt.Fprintf(stderr, "FIRST.org: %d teams for candidate seeding\n", len(teams))
			for _, team := range teams {
				candidates = append(candidates, model.SourceCandidate{
					URL:           team.Website,
					AuthorityName: team.ShortName,
					AuthorityType: "cert",
					Category:      "cyber_advisory",
					Country:       team.Country,
					BaseURL:       team.Website,
					Notes:         "autonomous seed: first.org",
				})
			}
		}

		agencies, err := FetchPoliceAgencies(ctx, cfg, client)
		if err != nil {
			if isDiscoveryTimeout(err) {
				slowSkips = append(slowSkips, "Wikidata police")
			} else {
				failures = append(failures, fmt.Sprintf("Wikidata police: %v", err))
			}
		} else {
			structuredSuccess = true
			fmt.Fprintf(stderr, "Wikidata: fetched %d police/law-enforcement agencies for candidate seeding\n", len(agencies))
			for _, agency := range agencies {
				if !passesDiscoveryHygiene(agency.Name, agency.Website, agency.AuthorityType) {
					continue
				}
				candidates = append(candidates, model.SourceCandidate{
					URL:           agency.Website,
					AuthorityName: agency.Name,
					AuthorityType: agency.AuthorityType,
					Category:      agency.Category,
					Country:       agency.Country,
					CountryCode:   agency.CountryCode,
					BaseURL:       agency.Website,
					Notes:         "autonomous seed: wikidata-police",
				})
			}
		}

		humOrgs, err := FetchHumanitarianOrgs(ctx, cfg, client)
		if err != nil {
			if isDiscoveryTimeout(err) {
				slowSkips = append(slowSkips, "Wikidata humanitarian")
			} else {
				failures = append(failures, fmt.Sprintf("Wikidata humanitarian: %v", err))
			}
		} else {
			structuredSuccess = true
			fmt.Fprintf(stderr, "Wikidata: fetched %d humanitarian/emergency orgs for candidate seeding\n", len(humOrgs))
			for _, org := range humOrgs {
				if !passesDiscoveryHygiene(org.Name, org.Website, "public_safety_program") {
					continue
				}
				candidates = append(candidates, model.SourceCandidate{
					URL:           org.Website,
					AuthorityName: org.Name,
					AuthorityType: "public_safety_program",
					Category:      "humanitarian_security",
					Country:       org.Country,
					CountryCode:   org.CountryCode,
					BaseURL:       org.Website,
					Notes:         "autonomous seed: wikidata-humanitarian",
				})
			}
		}

		govOrgs, err := FetchGovernmentOrgs(ctx, cfg, client)
		if err != nil {
			if isDiscoveryTimeout(err) {
				slowSkips = append(slowSkips, "Wikidata government")
			} else {
				failures = append(failures, fmt.Sprintf("Wikidata government: %v", err))
			}
		} else {
			structuredSuccess = true
			fmt.Fprintf(stderr, "Wikidata: fetched %d government/legislative/diplomatic orgs for candidate seeding\n", len(govOrgs))
			for _, org := range govOrgs {
				if !passesDiscoveryHygiene(org.Name, org.Website, org.AuthorityType) {
					continue
				}
				candidates = append(candidates, model.SourceCandidate{
					URL:           org.Website,
					AuthorityName: org.Name,
					AuthorityType: org.AuthorityType,
					Category:      org.Category,
					Country:       org.Country,
					CountryCode:   org.CountryCode,
					BaseURL:       org.Website,
					Notes:         "autonomous seed: wikidata-government",
				})
			}
		}
		if structuredSuccess {
			if err := markStructuredDiscoveryRun(cfg, time.Now().UTC()); err != nil {
				fmt.Fprintf(stderr, "WARN structured discovery state update failed: %v\n", err)
			}
		}
	}

	if len(failures) > 0 {
		fmt.Fprintf(stderr, "WARN structured discovery partially failed: %s\n", strings.Join(failures, " | "))
	}
	if len(slowSkips) > 0 {
		fmt.Fprintf(stderr, "Structured discovery skipped slow providers: %s\n", strings.Join(slowSkips, ", "))
	}
	// Gap analysis: find countries with missing categories and seed searches.
	gapCandidates := AnalyzeGaps(registrySources, stderr)
	if len(gapCandidates) > 0 {
		candidates = append(candidates, gapCandidates...)
	}

	searchSeeds := append([]model.SourceCandidate{}, candidates...)
	replacementTargets := buildReplacementSearchTargets(dead)
	if len(replacementTargets) > 0 {
		fmt.Fprintf(stderr, "Replacement search targets: %d dead-source metadata entries queued for feed search\n", len(replacementTargets))
		searchSeeds = append(searchSeeds, replacementTargets...)
	}
	if cfg.DiscoverSocialEnabled {
		socialSeeds := buildSocialDiscoveryTargets(cfg, searchSeeds)
		if len(socialSeeds) > 0 {
			fmt.Fprintf(stderr, "Social discovery seeds: %d X/Telegram targets queued for conflict/piracy/terror monitoring\n", len(socialSeeds))
			searchSeeds = append(searchSeeds, socialSeeds...)
		}
	}

	// DDG search is the first citizen — free, no API key needed.
	// LLM search is the fallback for targets DDG didn't cover.
	ddgCandidates, err := ddgSearchCandidates(ctx, cfg, browser, searchSeeds)
	if err != nil {
		failures = append(failures, fmt.Sprintf("ddg-search: %v", err))
	} else if len(ddgCandidates) > 0 {
		fmt.Fprintf(stderr, "DDG search discovery: found %d candidate URLs\n", len(ddgCandidates))
		candidates = append(candidates, ddgCandidates...)
	}

	// LLM search only for remaining seeds that DDG didn't find results for.
	if cfg.SearchDiscoveryEnabled && searchClient != nil {
		ddgCoveredKeys := map[string]struct{}{}
		for _, c := range ddgCandidates {
			key := strings.ToLower(c.AuthorityName) + "|" + strings.ToUpper(c.CountryCode) + "|" + strings.ToLower(c.Category)
			ddgCoveredKeys[key] = struct{}{}
		}
		var llmSeeds []model.SourceCandidate
		for _, seed := range searchSeeds {
			key := strings.ToLower(seed.AuthorityName) + "|" + strings.ToUpper(seed.CountryCode) + "|" + strings.ToLower(seed.Category)
			if _, covered := ddgCoveredKeys[key]; !covered {
				llmSeeds = append(llmSeeds, seed)
			}
		}
		if len(llmSeeds) > 0 {
			llmCandidates, err := llmSearchCandidates(ctx, cfg, searchClient, llmSeeds)
			if err != nil {
				failures = append(failures, fmt.Sprintf("llm-search: %v", err))
			} else if len(llmCandidates) > 0 {
				fmt.Fprintf(stderr, "LLM search discovery (fallback): generated %d candidate URLs via %s\n", len(llmCandidates), cfg.VettingProvider)
				candidates = append(candidates, llmCandidates...)
			}
		}
	}
	if len(failures) > 0 {
		return candidates, fmt.Errorf("%s", strings.Join(failures, " | "))
	}
	return candidates, nil
}

type structuredDiscoveryState struct {
	LastStructuredRun string `json:"last_structured_run"`
}

func shouldRunStructuredDiscovery(cfg config.Config, now time.Time) (bool, time.Time, error) {
	intervalHours := cfg.StructuredDiscoveryIntervalHours
	if intervalHours <= 0 {
		intervalHours = 168
	}
	statePath := structuredDiscoveryStatePath(cfg)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, now, nil
		}
		return true, now, err
	}
	var state structuredDiscoveryState
	if err := json.Unmarshal(data, &state); err != nil {
		return true, now, nil
	}
	last := strings.TrimSpace(state.LastStructuredRun)
	if last == "" {
		return true, now, nil
	}
	lastTime, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return true, now, nil
	}
	nextAt := lastTime.UTC().Add(time.Duration(intervalHours) * time.Hour)
	return !now.Before(nextAt), nextAt, nil
}

func markStructuredDiscoveryRun(cfg config.Config, now time.Time) error {
	path := structuredDiscoveryStatePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state := structuredDiscoveryState{LastStructuredRun: now.UTC().Format(time.RFC3339)}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func structuredDiscoveryStatePath(cfg config.Config) string {
	base := strings.TrimSpace(cfg.WikidataCachePath)
	if base == "" {
		base = filepath.Join("registry", "wikidata_cache")
	}
	return filepath.Join(base, "structured_discovery_state.json")
}

func isDiscoveryTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "request canceled")
}

func buildReplacementSearchTargets(dead []model.SourceReplacementCandidate) []model.SourceCandidate {
	out := make([]model.SourceCandidate, 0, len(dead))
	seen := map[string]struct{}{}
	for _, entry := range dead {
		target := model.SourceCandidate{
			AuthorityName: strings.TrimSpace(entry.AuthorityName),
			AuthorityType: strings.TrimSpace(entry.AuthorityType),
			Category:      strings.TrimSpace(entry.Category),
			Country:       strings.TrimSpace(entry.Country),
			CountryCode:   strings.ToUpper(strings.TrimSpace(entry.CountryCode)),
			Region:        strings.TrimSpace(entry.Region),
			BaseURL:       strings.TrimSpace(entry.BaseURL),
			Notes:         "replacement-search: dead-source metadata",
		}
		if target.BaseURL == "" {
			target.BaseURL = strings.TrimSpace(entry.FeedURL)
		}
		if !passesDiscoveryHygiene(target.AuthorityName, firstNonEmpty(target.BaseURL, target.URL), target.AuthorityType) {
			continue
		}
		key := strings.ToLower(target.AuthorityName) + "|" + target.CountryCode + "|" + strings.ToLower(target.Category) + "|" + normalizeURL(firstNonEmpty(target.BaseURL, target.URL))
		if key == "|||" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	return out
}

func mergeCandidates(existingQueue []model.SourceCandidate, discovered []model.SourceCandidate, active map[string]struct{}, dead []model.SourceReplacementCandidate) []model.SourceCandidate {
	out := make([]model.SourceCandidate, 0, len(existingQueue)+len(discovered))
	seen := map[string]struct{}{}
	add := func(candidate model.SourceCandidate) {
		if isDeadLettered(candidate, dead) {
			return
		}
		key := normalizeURL(firstNonEmpty(candidate.URL, candidate.BaseURL))
		if key == "" {
			return
		}
		if _, ok := active[key]; ok {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	for _, candidate := range existingQueue {
		add(candidate)
	}
	for _, candidate := range discovered {
		add(candidate)
	}
	return out
}

func normalizeURL(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	return strings.ToLower(u)
}

func loadDeadLetterQueue(path string) []model.SourceReplacementCandidate {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc model.SourceReplacementDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	return doc.Sources
}

func loadCandidateQueue(path string) []model.SourceCandidate {
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

func loadSovereignSeedCandidates(path string) []model.SourceCandidate {
	seeds := loadCandidateQueue(path)
	if len(seeds) == 0 {
		return nil
	}
	out := make([]model.SourceCandidate, 0, len(seeds))
	for _, seed := range seeds {
		seed.URL = strings.TrimSpace(seed.URL)
		seed.BaseURL = strings.TrimSpace(seed.BaseURL)
		if seed.URL == "" && seed.BaseURL == "" {
			continue
		}
		if strings.TrimSpace(seed.Category) == "" {
			seed.Category = "legislative"
		}
		if strings.TrimSpace(seed.AuthorityType) == "" {
			seed.AuthorityType = "government"
		}
		seed.CountryCode = strings.ToUpper(strings.TrimSpace(seed.CountryCode))
		seed.Notes = firstNonEmpty(seed.Notes, "autonomous seed: sovereign-official-statements")
		out = append(out, seed)
	}
	return out
}

func writeCandidateQueue(path string, candidates []model.SourceCandidate) error {
	doc := model.SourceCandidateDocument{
		GeneratedAt: "",
		Sources:     candidates,
	}
	if doc.Sources == nil {
		doc.Sources = []model.SourceCandidate{}
	}
	return writeJSON(path, doc)
}

func candidateBaseURL(candidate model.SourceCandidate) string {
	for _, value := range []string{candidate.BaseURL, candidate.URL} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil {
			continue
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	}
	return ""
}

func isDeadLettered(candidate model.SourceCandidate, dead []model.SourceReplacementCandidate) bool {
	candidateURLs := compactNormalizedURLs(candidate.URL, candidate.BaseURL)
	for _, entry := range dead {
		for _, deadURL := range compactNormalizedURLs(entry.FeedURL, entry.BaseURL) {
			for _, candidateURL := range candidateURLs {
				if candidateURL == deadURL {
					return true
				}
			}
		}
	}
	return false
}

func compactNormalizedURLs(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		norm := normalizeURL(value)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out
}

func shouldRetryCandidateQueue(ctx context.Context, client *fetch.Client, candidate model.SourceCandidate, baseURL string) bool {
	target := strings.TrimSpace(candidate.URL)
	if target == "" {
		target = baseURL
	}
	if target == "" {
		return false
	}
	status, err := client.HeadStatus(ctx, target, true)
	if err != nil {
		// Keep retries for transient network/probe errors.
		return true
	}
	if isSocialSignalURL(target) && (status == 401 || status == 403 || status == 429) {
		return true
	}
	// Queue should only retain potentially valid sources.
	if status == 404 || status == 410 || status >= 500 {
		return false
	}
	return status >= 200 && status < 400
}

func vetAndPromote(ctx context.Context, cfg config.Config, client *fetch.Client, sourceVetter *vet.Vetter, candidate model.SourceCandidate, discovered DiscoveredSource) (*model.RegistrySource, vet.Verdict, error) {
	samples, err := sampleSource(ctx, client, discovered, cfg.VettingMaxSampleItems)
	if err != nil {
		return nil, vet.Verdict{}, err
	}
	verdict, err := sourceVetter.Evaluate(ctx, vet.Input{
		AuthorityName: candidate.AuthorityName,
		AuthorityType: candidate.AuthorityType,
		Category:      candidate.Category,
		Country:       candidate.Country,
		CountryCode:   candidate.CountryCode,
		URL:           discovered.FeedURL,
		BaseURL:       candidateBaseURL(candidate),
		FeedType:      discovered.FeedType,
		Samples:       samples,
	})
	if err != nil {
		return nil, vet.Verdict{}, err
	}
	if !verdict.Approve || verdict.PromotionStatus == "rejected" {
		return nil, verdict, nil
	}
	if cfg.SourceVettingRequired {
		minQuality := cfg.SourceMinQuality
		minOperationalRelevance := cfg.SourceMinOperationalRelevance
		if isOfficialStatementsCandidate(candidate) {
			minQuality = maxFloat(minQuality, cfg.OfficialStatementsMinQuality)
			minOperationalRelevance = maxFloat(minOperationalRelevance, cfg.OfficialStatementsMinOperational)
		}

		if float64(verdict.SourceQuality) < minQuality {
			verdict.Approve = false
			verdict.PromotionStatus = "rejected"
			verdict.Reason = fmt.Sprintf("below source quality threshold %.2f < %.2f", float64(verdict.SourceQuality), minQuality)
			return nil, verdict, nil
		}
		if !isOfficialStatementsCandidate(candidate) {
			switch strings.ToLower(strings.TrimSpace(verdict.Level)) {
			case "local":
				// Local official law-enforcement is valuable for country-scoped views.
				// Keep a lower operational floor so we can retain trusted local signals.
				minOperationalRelevance = minFloat(minOperationalRelevance, 0.35)
			case "regional":
				minOperationalRelevance = minFloat(minOperationalRelevance, 0.5)
			}
		}
		if float64(verdict.OperationalRelevance) < minOperationalRelevance {
			verdict.Approve = false
			verdict.PromotionStatus = "rejected"
			verdict.Reason = fmt.Sprintf("below operational relevance threshold %.2f < %.2f", float64(verdict.OperationalRelevance), minOperationalRelevance)
			return nil, verdict, nil
		}
	}

	src := model.RegistrySource{
		Type:            discoveredTypeToRegistryType(discovered.FeedType),
		FeedURL:         discovered.FeedURL,
		Category:        firstNonEmpty(verdict.Category, candidate.Category, discovered.Category),
		RegionTag:       strings.ToUpper(strings.TrimSpace(candidate.CountryCode)),
		SourceQuality:   float64(verdict.SourceQuality),
		PromotionStatus: verdict.PromotionStatus,
		Reporting:       model.ReportingMetadata{},
		Source: model.SourceMetadata{
			SourceID:             sourceIDForCandidate(candidate, discovered),
			AuthorityName:        firstNonEmpty(candidate.AuthorityName, discovered.OrgName),
			Country:              firstNonEmpty(candidate.Country, discovered.Country),
			CountryCode:          strings.ToUpper(firstNonEmpty(candidate.CountryCode, discovered.CountryCode)),
			Region:               firstNonEmpty(candidate.Region, "International"),
			AuthorityType:        firstNonEmpty(candidate.AuthorityType, discovered.AuthorityType, "public_safety_program"),
			BaseURL:              candidateBaseURL(candidate),
			Scope:                verdict.Level,
			Level:                verdict.Level,
			MissionTags:          verdict.MissionTags,
			OperationalRelevance: float64(verdict.OperationalRelevance),
			LanguageCode:         "",
		},
	}
	if src.Source.BaseURL == "" {
		src.Source.BaseURL = discovered.TeamURL
	}
	return &src, verdict, nil
}

func isOfficialStatementsCandidate(candidate model.SourceCandidate) bool {
	if !strings.EqualFold(strings.TrimSpace(candidate.Category), "legislative") {
		return false
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(candidate.Notes)), "official-statements") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(candidate.AuthorityType)) {
	case "government", "legislative", "diplomatic", "national_security":
		return true
	}
	return false
}

func promoteDiscoveredSources(ctx context.Context, registryPath string, sources []model.RegistrySource) error {
	if len(sources) == 0 || !isSQLitePath(registryPath) {
		return nil
	}
	db, err := sourcedb.Open(registryPath)
	if err != nil {
		return fmt.Errorf("open source DB for promoted sources: %w", err)
	}
	defer db.Close()
	if err := db.UpsertRegistrySources(ctx, sources); err != nil {
		return fmt.Errorf("promote discovered sources: %w", err)
	}
	return nil
}

func sampleSource(ctx context.Context, client *fetch.Client, discovered DiscoveredSource, limit int) ([]vet.Sample, error) {
	accept := "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8"
	if discovered.FeedType == "html-list" || discovered.FeedType == "telegram" || discovered.FeedType == "x" {
		accept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	}
	body, err := client.Text(ctx, discovered.FeedURL, true, accept)
	if err != nil {
		return nil, fmt.Errorf("sample source fetch %s: %w", discovered.FeedURL, err)
	}
	var items []parse.FeedItem
	if discovered.FeedType == "telegram" {
		items = parse.ParseTelegram(string(body), extractTelegramChannel(discovered.FeedURL))
	} else if discovered.FeedType == "html-list" || discovered.FeedType == "x" {
		items = parse.ParseHTMLAnchors(string(body), discovered.FeedURL)
	} else {
		items = parse.ParseFeed(string(body))
	}
	return vet.SamplesFromFeedItems(items, limit), nil
}

func discoveredTypeToRegistryType(feedType string) string {
	switch strings.TrimSpace(feedType) {
	case "x":
		return "x"
	case "telegram":
		return "telegram"
	case "html-list":
		return "html-list"
	default:
		return "rss"
	}
}

func sourceIDForCandidate(candidate model.SourceCandidate, discovered DiscoveredSource) string {
	base := firstNonEmpty(candidate.AuthorityName, discovered.OrgName, candidate.URL, discovered.FeedURL)
	base = strings.ToLower(base)
	replacer := strings.NewReplacer("https://", "", "http://", "", ".", "-", "/", "-", " ", "-", "_", "-", ":", "-", "&", "and")
	base = replacer.Replace(base)
	base = strings.Trim(base, "-")
	for strings.Contains(base, "--") {
		base = strings.ReplaceAll(base, "--", "-")
	}
	if base == "" {
		return "candidate-source"
	}
	return base
}

func discoveredFeedTypeFromURL(raw string) string {
	if !looksLikeSocialSignalURL(raw) {
		return "html-list"
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "html-list"
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	if host == "t.me" || host == "telegram.me" {
		return "telegram"
	}
	if host == "x.com" || host == "twitter.com" {
		return "x"
	}
	return "html-list"
}

func buildSocialDiscoveryTargets(cfg config.Config, seeds []model.SourceCandidate) []model.SourceCandidate {
	maxTargets := cfg.DiscoverSocialMaxTargets
	if maxTargets <= 0 {
		maxTargets = 24
	}
	out := make([]model.SourceCandidate, 0, maxTargets)
	seen := map[string]struct{}{}
	for _, seed := range seeds {
		if !socialDiscoveryCategory(seed.Category) {
			continue
		}
		cc := strings.ToUpper(strings.TrimSpace(seed.CountryCode))
		if cc == "" || cc == "INT" {
			continue
		}
		for _, host := range []string{"https://x.com", "https://t.me"} {
			key := cc + "|" + strings.ToLower(strings.TrimSpace(seed.Category)) + "|" + host
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, model.SourceCandidate{
				URL:           host,
				BaseURL:       host,
				AuthorityName: firstNonEmpty(seed.AuthorityName, seed.Country+" conflict monitor"),
				AuthorityType: firstNonEmpty(seed.AuthorityType, "national_security"),
				Category:      seed.Category,
				Country:       seed.Country,
				CountryCode:   cc,
				Region:        seed.Region,
				Notes:         "autonomous seed: social-discovery",
			})
			if len(out) >= maxTargets {
				return out
			}
		}
	}
	return out
}

func extractTelegramChannel(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	if parts[0] == "s" && len(parts) > 1 {
		return parts[1]
	}
	return parts[0]
}

// writeJSON is a helper that marshals data to indented JSON and writes to a file.
func writeJSON(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
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

func minFloat(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func isSQLitePath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	default:
		return false
	}
}
