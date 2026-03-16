// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
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
	if cfg.Watch && cfg.DiscoverBackground {
		go r.runDiscoveryLoop(ctx, cfg)
	}
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
	nctx := normalize.Context{Config: cfg, Now: now}
	categoryDictionary, err := dictionary.Load(cfg.CategoryDictionaryPath)
	if err != nil {
		fmt.Fprintf(r.stderr, "WARN category dictionary load failed (falling back to legacy filters): %v\n", err)
	}

	alerts := []model.Alert{normalize.StaticInterpolEntry(now)}
	sourceHealth := make([]model.SourceHealthEntry, 0, len(sources))
	for _, source := range sources {
		startedAt := time.Now().UTC()
		fetcher := fetch.FetcherFor(source.FetchMode, client, browser)
		batch, err := r.fetchSource(ctx, fetcher, browser, nctx, source, categoryDictionary)

		// Retry once for transient errors (timeout, EOF) after a short backoff.
		if err != nil {
			errClass, _, _ := classifySourceError(err)
			if (errClass == "timeout" || errClass == "eof" || errClass == "transient") && ctx.Err() == nil {
				fmt.Fprintf(r.stderr, "RETRY %s (transient %s): %v\n", source.Source.AuthorityName, errClass, err)
				retryDelay := 3 * time.Second
				select {
				case <-time.After(retryDelay):
				case <-ctx.Done():
				}
				if ctx.Err() == nil {
					batch, err = r.fetchSource(ctx, fetcher, browser, nctx, source, categoryDictionary)
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

	previous, err := loadPreviousAlerts(ctx, cfg)
	if err != nil {
		return err
	}
	currentActive, currentFiltered, fullState := state.Reconcile(cfg, active, filtered, previous, now)
	replacementQueue := buildReplacementQueue(sourceHealth, sources)
	if err := deactivateReplacementSources(ctx, cfg.RegistryPath, replacementQueue); err != nil {
		return err
	}
	if err := saveAlertState(ctx, cfg, fullState); err != nil {
		return err
	}
	if err := output.Write(cfg, currentActive, currentFiltered, fullState, sourceHealth, duplicateAudit, replacementQueue); err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdout, "Wrote %d active alerts -> %s (%d filtered in %s)\n", len(currentActive), cfg.OutputPath, len(currentFiltered), cfg.FilteredOutputPath)
	return err
}

func (r Runner) fetchSource(ctx context.Context, fetcher fetch.Fetcher, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource, categoryDictionary *dictionary.Store) ([]model.Alert, error) {
	switch source.Type {
	case "rss":
		return r.fetchRSS(ctx, fetcher, nctx, source)
	case "html-list":
		return r.fetchHTML(ctx, fetcher, nctx, source, categoryDictionary)
	case "kev-json":
		return r.fetchKEV(ctx, fetcher, nctx, source)
	case "interpol-red-json", "interpol-yellow-json":
		return r.fetchInterpol(ctx, fetcher, browser, nctx, source)
	case "travelwarning-json":
		return r.fetchTravelWarningJSON(ctx, fetcher, nctx, source)
	case "travelwarning-atom":
		return r.fetchTravelWarningAtom(ctx, fetcher, nctx, source)
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
			return out, nil
		}
	}
	for _, item := range items {
		alert := normalize.HTMLItem(nctx, source, item)
		if alert != nil {
			out = append(out, *alert)
		}
	}
	return out, nil
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

func (r Runner) fetchInterpol(ctx context.Context, fetcher fetch.Fetcher, browser *fetch.BrowserClient, nctx normalize.Context, source model.RegistrySource) ([]model.Alert, error) {
	limit := perSourceLimit(nctx.Config, source)
	pageSize := 20
	var allNotices []model.Alert

	// Interpol's API sits behind Akamai WAF and requires XHR-style headers
	// with Referer/Origin pointing to the Interpol website.
	interpolHeaders := map[string]string{
		"Referer":         "https://www.interpol.int/How-we-work/Notices/View-Notices",
		"Origin":          "https://www.interpol.int",
		"Sec-Fetch-Dest":  "empty",
		"Sec-Fetch-Mode":  "cors",
		"Sec-Fetch-Site":  "same-site",
		"X-Requested-With": "XMLHttpRequest",
	}

	// Use TextWithHeaders if the fetcher is a *Client (stealth HTTP).
	clientFetcher, isClient := fetcher.(*fetch.Client)

	for page := 1; len(allNotices) < limit; page++ {
		pageURL := buildInterpolPageURL(source.FeedURL, page, pageSize)
		var body []byte
		var err error
		if isClient {
			body, err = clientFetcher.TextWithHeaders(ctx, pageURL, source.FollowRedirects, "application/json", interpolHeaders)
		} else {
			body, err = fetcher.Text(ctx, pageURL, source.FollowRedirects, "application/json")
		}
		if err != nil {
			// If first page fails, try browser fallback for the whole batch.
			if page == 1 && browser != nil {
				fmt.Fprintf(r.stderr, "WARN %s: stealth fetch failed, trying browser fallback: %v\n", source.Source.AuthorityName, err)
				bBody, bErr := fetchInterpolViaBrowser(ctx, browser, source)
				if bErr == nil && len(bBody) > 0 {
					return parseInterpolNotices(nctx, source, bBody)
				}
			}
			break
		}
		batch, err := parseInterpolNotices(nctx, source, body)
		if err != nil {
			break
		}
		allNotices = append(allNotices, batch...)
		if len(batch) < pageSize {
			break // last page
		}
		// Polite delay between page requests.
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return allNotices, nil
		}
	}
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
		link := notice.Links.Self.Href
		if strings.TrimSpace(link) != "" {
			if _, err := url.Parse(link); err == nil && !strings.HasPrefix(link, "http") {
				link = (&url.URL{Scheme: "https", Host: "ws-public.interpol.int", Path: link}).String()
			}
		}
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
		return id
	}
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	if fragment := strings.TrimSpace(parsed.Fragment); fragment != "" {
		return fragment
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return strings.TrimSpace(parts[len(parts)-1])
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
	var lastErr error
	for _, candidate := range candidates {
		body, err := fetcher.Text(ctx, candidate, source.FollowRedirects, accept)
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

func (r Runner) runDiscoveryLoop(ctx context.Context, cfg config.Config) {
	runOnce := func() {
		if err := discover.Run(ctx, cfg, r.stdout, r.stderr); err != nil && ctx.Err() == nil {
			fmt.Fprintf(r.stderr, "WARN background discovery failed: %v\n", err)
		}
	}

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
	case strings.Contains(msg, "status 301"), strings.Contains(msg, "status 302"), strings.Contains(msg, "status 307"), strings.Contains(msg, "status 308"):
		return "redirect", true, "dead_letter"
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

func isSQLiteRegistryPath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	default:
		return false
	}
}
