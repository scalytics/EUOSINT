// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"strings"

	"github.com/scalytics/euosint/internal/collector/fetch"
)

// Common RSS/Atom feed path suffixes to probe on a website.
var feedPaths = []string{
	"/feed",
	"/rss",
	"/rss.xml",
	"/feed.xml",
	"/atom.xml",
	"/.rss",
	"/advisories/feed",
	"/feed/rss",
	"/index.xml",
	// Press-release / news patterns common on police & government sites.
	"/news/feed",
	"/news/rss",
	"/news/rss.xml",
	"/en/feed",
	"/en/rss",
	"/en/rss.xml",
	"/press-releases/feed",
	"/media/news/feed",
	"/resources/press-releases/feed",
	"/newsroom/feed",
	"/latest/feed",
	// Government / DOJ / ministry patterns.
	"/feeds/opa/justice-news.xml",
	"/feeds/news.xml",
	"/feeds/press-releases.xml",
	"/feeds/alerts.xml",
	"/blog/feed",
	"/blog/rss",
	"/updates/feed",
	"/publications/feed",
	"/advisories.xml",
	"/warnings/feed",
	"/alerts/feed",
	"/releases/feed",
	// Multi-language government sites.
	"/de/feed",
	"/fr/feed",
	"/es/feed",
	"/it/feed",
	"/nl/feed",
	"/sv/feed",
	"/no/feed",
	"/da/feed",
	"/fi/feed",
	"/pl/feed",
	"/pt/feed",
}

// ProbedFeed is a single discovered feed from probing.
type ProbedFeed struct {
	FeedURL  string
	FeedType string // "rss" or "atom"
}

// ProbeFeeds tries known RSS/Atom path suffixes on the given base URL and
// returns all valid feeds found (typically one, but a site may have several).
func ProbeFeeds(ctx context.Context, client *fetch.Client, baseURL string) []ProbedFeed {
	baseURL = strings.TrimRight(baseURL, "/")
	var found []ProbedFeed
	seen := map[string]struct{}{}
	for _, path := range feedPaths {
		if ctx.Err() != nil {
			break
		}
		candidate := baseURL + path
		body, err := client.Text(ctx, candidate, true, "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
		if err != nil {
			continue
		}
		content := string(body)
		feedType := detectFeedType(content)
		if feedType == "" {
			continue
		}
		norm := strings.ToLower(candidate)
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		found = append(found, ProbedFeed{FeedURL: candidate, FeedType: feedType})
		// Stop after first hit — one valid feed per site is enough.
		break
	}
	return found
}

// ProbeRSSFeed is a convenience wrapper that returns the first discovered
// feed URL and type (for backward compatibility).
func ProbeRSSFeed(ctx context.Context, client *fetch.Client, baseURL string) (string, string, error) {
	results := ProbeFeeds(ctx, client, baseURL)
	if len(results) == 0 {
		return "", "", nil
	}
	return results[0].FeedURL, results[0].FeedType, nil
}

// probeHTMLPage checks whether a URL returns a reachable HTML page that
// looks like a press-release listing (contains anchor tags with text).
func probeHTMLPage(ctx context.Context, client *fetch.Client, url string) bool {
	body, err := client.Text(ctx, url, true, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if err != nil {
		return false
	}
	content := strings.ToLower(string(body))
	// Minimal validation: must look like HTML with some links.
	return strings.Contains(content, "<html") && strings.Contains(content, "<a ")
}

// detectFeedType inspects the body content to determine if it is a valid
// RSS or Atom feed. Returns "rss", "atom", or "" if not a feed.
func detectFeedType(content string) string {
	// Quick checks on the first 2KB of content.
	prefix := content
	if len(prefix) > 2048 {
		prefix = prefix[:2048]
	}
	lower := strings.ToLower(prefix)
	if strings.Contains(lower, "<feed") && strings.Contains(lower, "xmlns") {
		return "atom"
	}
	if strings.Contains(lower, "<rss") || (strings.Contains(lower, "<channel") && strings.Contains(lower, "<item")) {
		return "rss"
	}
	return ""
}
