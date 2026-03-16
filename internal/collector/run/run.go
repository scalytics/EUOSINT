package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/normalize"
	"github.com/scalytics/euosint/internal/collector/output"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/collector/registry"
	"github.com/scalytics/euosint/internal/collector/state"
	"github.com/scalytics/euosint/internal/collector/translate"
)

type Runner struct {
	stdout        io.Writer
	stderr        io.Writer
	clientFactory func(config.Config) *fetch.Client
}

func New(stdout io.Writer, stderr io.Writer) Runner {
	return Runner{
		stdout:        stdout,
		stderr:        stderr,
		clientFactory: fetch.New,
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
	sources, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		return err
	}
	client := r.clientFactory(cfg)
	now := time.Now().UTC()
	nctx := normalize.Context{Config: cfg, Now: now}

	alerts := []model.Alert{normalize.StaticInterpolEntry(now)}
	sourceHealth := make([]model.SourceHealthEntry, 0, len(sources))
	for _, source := range sources {
		startedAt := time.Now().UTC()
		batch, err := r.fetchSource(ctx, client, nctx, source)
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
			sourceHealth = append(sourceHealth, entry)
			fmt.Fprintf(r.stderr, "WARN %s: %v\n", source.Source.AuthorityName, err)
			continue
		}
		entry.Status = "ok"
		entry.FetchedCount = len(batch)
		sourceHealth = append(sourceHealth, entry)
		alerts = append(alerts, batch...)
	}

	deduped, duplicateAudit := normalize.Deduplicate(alerts)
	active, filtered := normalize.FilterActive(cfg, deduped)
	populateSourceHealth(sourceHealth, active, filtered)
	if err := assertCriticalSourceCoverage(cfg, sourceHealth); err != nil {
		return err
	}

	previous := state.Read(cfg.StateOutputPath)
	if len(previous) == 0 {
		previous = state.Read(cfg.OutputPath)
	}
	currentActive, currentFiltered, fullState := state.Reconcile(cfg, active, filtered, previous, now)
	if err := output.Write(cfg, currentActive, currentFiltered, fullState, sourceHealth, duplicateAudit); err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdout, "Wrote %d active alerts -> %s (%d filtered in %s)\n", len(currentActive), cfg.OutputPath, len(currentFiltered), cfg.FilteredOutputPath)
	return err
}

func (r Runner) fetchSource(ctx context.Context, client *fetch.Client, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	switch source.Type {
	case "rss":
		return r.fetchRSS(ctx, client, nctx, source)
	case "html-list":
		return r.fetchHTML(ctx, client, nctx, source)
	case "kev-json":
		return r.fetchKEV(ctx, client, nctx, source)
	case "interpol-red-json", "interpol-yellow-json":
		return r.fetchInterpol(ctx, client, nctx, source)
	default:
		return nil, fmt.Errorf("unsupported source type %s", source.Type)
	}
}

func (r Runner) fetchRSS(ctx context.Context, client *fetch.Client, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := fetchWithFallback(ctx, client, source, "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
	if err != nil {
		return nil, err
	}
	items := parse.ParseFeed(string(body))
	if nctx.Config.TranslateEnabled {
		if translated, err := translate.Batch(ctx, client, items); err == nil {
			items = translated
		} else {
			fmt.Fprintf(r.stderr, "WARN %s: translate batch failed: %v\n", source.Source.AuthorityName, err)
		}
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
		alert := normalize.RSSItem(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func (r Runner) fetchHTML(ctx context.Context, client *fetch.Client, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, finalURL, err := fetchWithFallbackURL(ctx, client, source, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if err != nil {
		return nil, err
	}
	items := parse.ParseHTMLAnchors(string(body), finalURL)
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
	return out, nil
}

func (r Runner) fetchKEV(ctx context.Context, client *fetch.Client, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := client.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
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

func (r Runner) fetchInterpol(ctx context.Context, client *fetch.Client, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	body, err := client.Text(ctx, source.FeedURL, source.FollowRedirects, "application/json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Embedded struct {
			Notices []struct {
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
	limit := perSourceLimit(nctx.Config, source)
	out := []model.Alert{}
	for _, notice := range doc.Embedded.Notices {
		if len(out) == limit {
			break
		}
		titlePrefix := "INTERPOL Red Notice"
		if source.Type == "interpol-yellow-json" {
			titlePrefix = "INTERPOL Yellow Notice"
		}
		label := strings.TrimSpace(strings.TrimSpace(notice.Forename) + " " + strings.TrimSpace(notice.Name))
		title := titlePrefix
		if label != "" {
			title = titlePrefix + ": " + label
		}
		link := notice.Links.Self.Href
		if strings.TrimSpace(link) != "" {
			if _, err := url.Parse(link); err == nil && !strings.HasPrefix(link, "http") {
				link = (&url.URL{Scheme: "https", Host: "ws-public.interpol.int", Path: link}).String()
			}
		}
		countryCode := ""
		if len(notice.CountriesLikelyToVisit) > 0 {
			countryCode = notice.CountriesLikelyToVisit[0]
		} else if len(notice.Nationalities) > 0 {
			countryCode = notice.Nationalities[0]
		}
		summary := strings.TrimSpace(notice.IssuingEntity + " " + notice.PlaceOfBirth)
		tags := append([]string{}, notice.Nationalities...)
		tags = append(tags, notice.CountriesLikelyToVisit...)
		alert := normalize.InterpolAlert(nctx, source, title, link, countryCode, summary, tags)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
}

func fetchWithFallback(ctx context.Context, client *fetch.Client, source model.RegistrySource, accept string) ([]byte, error) {
	body, _, err := fetchWithFallbackURL(ctx, client, source, accept)
	return body, err
}

func fetchWithFallbackURL(ctx context.Context, client *fetch.Client, source model.RegistrySource, accept string) ([]byte, string, error) {
	candidates := []string{}
	if strings.TrimSpace(source.FeedURL) != "" {
		candidates = append(candidates, source.FeedURL)
	}
	candidates = append(candidates, source.FeedURLs...)
	var lastErr error
	for _, candidate := range candidates {
		body, err := client.Text(ctx, candidate, source.FollowRedirects, accept)
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
		hay := strings.ToLower(item.Title + " " + item.Link)
		if len(include) > 0 && !containsKeyword(hay, include) {
			continue
		}
		if len(exclude) > 0 && containsKeyword(hay, exclude) {
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
