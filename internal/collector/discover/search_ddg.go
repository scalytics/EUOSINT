// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
)

// ddgResultRe extracts href URLs from DDG result links.
// DDG wraps results in <a class="result__a" href="..."> or
// <a rel="nofollow" ... href="https://duckduckgo.com/l/?...uddg=REAL_URL">
var ddgResultRe = regexp.MustCompile(`(?i)href="(https?://[^"]+)"`)

// ddgRedirectRe extracts the real URL from DDG redirect wrappers.
var ddgRedirectRe = regexp.MustCompile(`[?&]uddg=([^&]+)`)

// ddgSearchCandidates uses DuckDuckGo via a headless browser to find
// RSS/Atom feed URLs for gap-analysis targets. This is the zero-dependency
// fallback when no LLM API key is configured.
func ddgSearchCandidates(ctx context.Context, cfg config.Config, browser *fetch.BrowserClient, seeds []model.SourceCandidate) ([]model.SourceCandidate, error) {
	if !cfg.DDGSearchEnabled || browser == nil {
		return nil, nil
	}

	targets := selectSearchTargets(cfg, seeds)
	if len(targets) == 0 {
		return nil, nil
	}

	maxQueries := cfg.DDGSearchMaxQueries
	if maxQueries <= 0 {
		maxQueries = 10
	}
	delay := time.Duration(cfg.DDGSearchDelayMS) * time.Millisecond
	if delay < 5*time.Second {
		delay = 5 * time.Second
	}

	// Limit to maxQueries to stay polite.
	if len(targets) > maxQueries {
		targets = targets[:maxQueries]
	}

	var out []model.SourceCandidate
	var failures []string
	seen := map[string]struct{}{}

	for i, target := range targets {
		if ctx.Err() != nil {
			break
		}
		// Polite delay between queries.
		if i > 0 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(delay):
			}
		}

		query := buildDDGQuery(target)
		found, err := ddgSearch(ctx, browser, query, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", query, err))
			continue
		}
		for _, c := range found {
			key := normalizeURL(c.URL)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, c)
		}
	}

	if len(failures) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("ddg search: %s", strings.Join(failures, " | "))
	}
	return out, nil
}

// buildDDGQuery creates a DDG search query for a discovery target.
func buildDDGQuery(target model.SourceCandidate) string {
	parts := []string{}

	name := strings.TrimSpace(target.AuthorityName)
	if name != "" {
		parts = append(parts, name)
	}

	country := strings.TrimSpace(target.Country)
	if country != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(country)) {
		parts = append(parts, country)
	}

	topic := searchTopicLabel(target.Category, target.AuthorityType)
	if topic != "" {
		parts = append(parts, topic)
	}

	if socialDiscoveryCategory(target.Category) {
		parts = append(parts, "(site:x.com OR site:t.me)")
		parts = append(parts, "alerts OR updates OR statements")
	} else {
		parts = append(parts, "RSS OR atom OR feed")
	}

	return strings.Join(parts, " ")
}

// ddgSearch performs a single DuckDuckGo search and extracts URLs.
func ddgSearch(ctx context.Context, browser *fetch.BrowserClient, query string, target model.SourceCandidate) ([]model.SourceCandidate, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)

	body, err := browser.Text(ctx, searchURL, true, "")
	if err != nil {
		return nil, fmt.Errorf("ddg fetch: %w", err)
	}

	content := string(body)
	urls := extractDDGURLs(content)
	if len(urls) == 0 {
		return nil, nil
	}

	var candidates []model.SourceCandidate
	for _, raw := range urls {
		if !looksLikeURL(raw) {
			continue
		}
		// Skip DDG internal links.
		if strings.Contains(raw, "duckduckgo.com") {
			continue
		}
		// Only keep URLs that look like they could be feeds or official sites.
		if !looksLikeFeedURL(raw) && !looksLikeOfficialSite(raw) && !looksLikeSocialSignalURL(raw) {
			continue
		}
		candidates = append(candidates, model.SourceCandidate{
			URL:           raw,
			AuthorityName: target.AuthorityName,
			AuthorityType: target.AuthorityType,
			Category:      target.Category,
			Country:       target.Country,
			CountryCode:   target.CountryCode,
			Region:        target.Region,
			BaseURL:       extractBaseURL(raw),
			Notes:         "ddg-search: " + query,
		})
		if len(candidates) >= 5 {
			break
		}
	}
	return candidates, nil
}

// extractDDGURLs parses URLs from DDG HTML search results.
func extractDDGURLs(html string) []string {
	var urls []string
	seen := map[string]struct{}{}

	for _, match := range ddgResultRe.FindAllStringSubmatch(html, -1) {
		if len(match) < 2 {
			continue
		}
		rawURL := match[1]

		// DDG wraps results in redirect URLs — extract the real target.
		if strings.Contains(rawURL, "duckduckgo.com/l/") {
			if redir := ddgRedirectRe.FindStringSubmatch(rawURL); len(redir) >= 2 {
				decoded, err := url.QueryUnescape(redir[1])
				if err == nil {
					rawURL = decoded
				}
			}
		}

		// Skip DDG assets and internal pages.
		if strings.Contains(rawURL, "duckduckgo.com") {
			continue
		}

		norm := strings.ToLower(strings.TrimRight(rawURL, "/"))
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		urls = append(urls, rawURL)
	}
	return urls
}

// looksLikeOfficialSite checks if a URL looks like an official government
// or organization site worth probing for feeds.
func looksLikeOfficialSite(raw string) bool {
	lower := strings.ToLower(raw)
	officialTLDs := []string{
		".gov", ".gob", ".gouv", ".govt",
		".mil", ".edu",
		".int", ".org",
		".police", ".cert",
	}
	for _, tld := range officialTLDs {
		if strings.Contains(lower, tld) {
			return true
		}
	}
	officialKeywords := []string{
		"ministry", "department", "agency",
		"security", "intelligence", "police",
		"cert", "csirt", "ncsc",
	}
	for _, kw := range officialKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// extractBaseURL returns the scheme + host portion of a URL.
func extractBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
