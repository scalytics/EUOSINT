// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
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
	if cfg.VettingEnabled {
		sourceVetter = vet.New(cfg)
	}
	if cfg.SearchDiscoveryEnabled {
		searchClient = vet.NewClient(cfg)
	}

	// Load existing registry for deduplication.
	existing := map[string]struct{}{}
	if sources, err := registry.Load(cfg.RegistryPath); err == nil {
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
	dead := loadDeadLetterQueue(cfg.ReplacementQueuePath)
	fmt.Fprintf(stderr, "Dead-letter queue: %d sources will be skipped\n", len(dead))
	seededCandidates, err := generateAutonomousCandidates(ctx, cfg, client, searchClient, dead, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "WARN autonomous candidate discovery failed: %v\n", err)
	}
	candidates := mergeCandidates(loadCandidateQueue(cfg.CandidateQueuePath), seededCandidates, existing, dead)
	fmt.Fprintf(stderr, "Candidate queue: %d sources queued for crawl\n", len(candidates))
	remainingCandidates := make([]model.SourceCandidate, 0, len(candidates))
	promotedSources := make([]model.RegistrySource, 0)

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		if !passesDiscoveryHygiene(candidate.AuthorityName, firstNonEmpty(candidate.BaseURL, candidate.URL), candidate.AuthorityType) {
			remainingCandidates = append(remainingCandidates, candidate)
			continue
		}
		if isDeadLettered(candidate, dead) {
			continue
		}

		baseURL := candidateBaseURL(candidate)
		if baseURL == "" {
			remainingCandidates = append(remainingCandidates, candidate)
			continue
		}
		promotedForCandidate := false
		results := ProbeFeeds(ctx, client, baseURL)
		for _, r := range results {
			if _, ok := existing[normalizeURL(r.FeedURL)]; ok {
				continue
			}
			existing[normalizeURL(r.FeedURL)] = struct{}{}
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
			discovered = append(discovered, found)
			if sourceVetter == nil {
				continue
			}
			promoted, verdict, err := vetAndPromote(ctx, cfg, client, sourceVetter, candidate, found)
			if err != nil {
				fmt.Fprintf(stderr, "WARN source vetting failed for %s: %v\n", found.FeedURL, err)
				continue
			}
			if promoted != nil {
				promotedSources = append(promotedSources, *promoted)
				promotedForCandidate = true
				fmt.Fprintf(stderr, "Promoted source %s via %s (%s)\n", promoted.Source.SourceID, cfg.VettingProvider, verdict.Reason)
			}
		}
		if len(results) == 0 {
			target := strings.TrimSpace(candidate.URL)
			if target == "" {
				target = baseURL
			}
			if _, ok := existing[normalizeURL(target)]; !ok && probeHTMLPage(ctx, client, target) {
				existing[normalizeURL(target)] = struct{}{}
				found := DiscoveredSource{
					FeedURL:       target,
					FeedType:      "html-list",
					AuthorityType: candidate.AuthorityType,
					Category:      candidate.Category,
					OrgName:       candidate.AuthorityName,
					Country:       candidate.Country,
					CountryCode:   candidate.CountryCode,
					TeamURL:       baseURL,
					DiscoveredVia: "candidate-queue",
				}
				discovered = append(discovered, found)
				if sourceVetter != nil {
					promoted, verdict, err := vetAndPromote(ctx, cfg, client, sourceVetter, candidate, found)
					if err != nil {
						fmt.Fprintf(stderr, "WARN source vetting failed for %s: %v\n", found.FeedURL, err)
					} else if promoted != nil {
						promotedSources = append(promotedSources, *promoted)
						promotedForCandidate = true
						fmt.Fprintf(stderr, "Promoted source %s via %s (%s)\n", promoted.Source.SourceID, cfg.VettingProvider, verdict.Reason)
					}
				}
			}
		}
		if !promotedForCandidate {
			remainingCandidates = append(remainingCandidates, candidate)
		}
	}

	// Write results.
	fmt.Fprintf(stderr, "Discovery finished: %d new candidates\n", len(discovered))
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

func generateAutonomousCandidates(ctx context.Context, cfg config.Config, client *fetch.Client, searchClient searchCompleter, dead []model.SourceReplacementCandidate, stderr io.Writer) ([]model.SourceCandidate, error) {
	candidates := make([]model.SourceCandidate, 0)
	var failures []string
	var slowSkips []string

	teams, err := FetchFIRSTTeams(ctx, client)
	if err != nil {
		if isDiscoveryTimeout(err) {
			slowSkips = append(slowSkips, "FIRST.org")
		} else {
			failures = append(failures, fmt.Sprintf("FIRST.org: %v", err))
		}
	} else {
		fmt.Fprintf(stderr, "FIRST.org: fetched %d teams for candidate seeding\n", len(teams))
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

	if len(failures) > 0 {
		fmt.Fprintf(stderr, "WARN structured discovery partially failed: %s\n", strings.Join(failures, " | "))
	}
	if len(slowSkips) > 0 {
		fmt.Fprintf(stderr, "Structured discovery skipped slow providers: %s\n", strings.Join(slowSkips, ", "))
	}
	searchSeeds := append([]model.SourceCandidate{}, candidates...)
	replacementTargets := buildReplacementSearchTargets(dead)
	if len(replacementTargets) > 0 {
		fmt.Fprintf(stderr, "Replacement search targets: %d dead-source metadata entries queued for feed search\n", len(replacementTargets))
		searchSeeds = append(searchSeeds, replacementTargets...)
	}
	llmCandidates, err := llmSearchCandidates(ctx, cfg, searchClient, searchSeeds)
	if err != nil {
		failures = append(failures, fmt.Sprintf("llm-search: %v", err))
	} else if len(llmCandidates) > 0 {
		fmt.Fprintf(stderr, "LLM search discovery: generated %d candidate URLs via %s\n", len(llmCandidates), cfg.VettingProvider)
		candidates = append(candidates, llmCandidates...)
	}
	if len(failures) > 0 {
		return candidates, fmt.Errorf("%s", strings.Join(failures, " | "))
	}
	return candidates, nil
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

	src := model.RegistrySource{
		Type:            discoveredTypeToRegistryType(discovered.FeedType),
		FeedURL:         discovered.FeedURL,
		Category:        firstNonEmpty(candidate.Category, discovered.Category),
		RegionTag:       strings.ToUpper(strings.TrimSpace(candidate.CountryCode)),
		SourceQuality:   verdict.SourceQuality,
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
			OperationalRelevance: verdict.OperationalRelevance,
			LanguageCode:         "",
		},
	}
	if src.Source.BaseURL == "" {
		src.Source.BaseURL = discovered.TeamURL
	}
	return &src, verdict, nil
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
	if discovered.FeedType == "html-list" {
		accept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	}
	body, err := client.Text(ctx, discovered.FeedURL, true, accept)
	if err != nil {
		return nil, fmt.Errorf("sample source fetch %s: %w", discovered.FeedURL, err)
	}
	var items []parse.FeedItem
	if discovered.FeedType == "html-list" {
		items = parse.ParseHTMLAnchors(string(body), discovered.FeedURL)
	} else {
		items = parse.ParseFeed(string(body))
	}
	return vet.SamplesFromFeedItems(items, limit), nil
}

func discoveredTypeToRegistryType(feedType string) string {
	switch strings.TrimSpace(feedType) {
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

func isSQLitePath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	default:
		return false
	}
}
