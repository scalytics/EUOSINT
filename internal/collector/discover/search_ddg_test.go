// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"testing"

	"github.com/scalytics/euosint/internal/collector/model"
)

func TestExtractDDGURLs(t *testing.T) {
	html := `
<div class="result">
  <a rel="nofollow" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.ncsc.gov.uk%2Frss%2Fall">NCSC RSS</a>
</div>
<div class="result">
  <a href="https://www.cert.se/feed/rss.xml">CERT-SE Feed</a>
</div>
<div class="result">
  <a href="https://duckduckgo.com/about">About DDG</a>
</div>
<div class="result">
  <a href="https://www.justice.gov/feeds/opa/justice-news.xml">DOJ Feed</a>
</div>
`
	urls := extractDDGURLs(html)

	// Should have 3 URLs (DDG internal filtered out).
	expected := map[string]bool{
		"https://www.ncsc.gov.uk/rss/all":                    false,
		"https://www.cert.se/feed/rss.xml":                   false,
		"https://www.justice.gov/feeds/opa/justice-news.xml": false,
	}

	for _, u := range urls {
		if _, ok := expected[u]; ok {
			expected[u] = true
		}
	}

	for u, found := range expected {
		if !found {
			t.Errorf("expected URL %q not found in results", u)
		}
	}
}

func TestBuildDDGQuery(t *testing.T) {
	target := model.SourceCandidate{
		AuthorityName: "Norway national CERT or CSIRT",
		Country:       "Norway",
		Category:      "cyber_advisory",
		AuthorityType: "cert",
	}

	query := buildDDGQuery(target)

	if query == "" {
		t.Fatal("expected non-empty query")
	}
	// Should contain the authority name and RSS/atom/feed keywords.
	if !containsAll(query, "Norway", "CERT", "RSS OR atom OR feed") {
		t.Errorf("query missing expected parts: %s", query)
	}
	// Country should NOT be duplicated (already in authority name).
	// Actually "Norway" appears in AuthorityName, so it should only appear once.
}

func TestBuildDDGQueryForConflictUsesSocialSites(t *testing.T) {
	target := model.SourceCandidate{
		AuthorityName: "Ukraine defense ministry or armed forces",
		Country:       "Ukraine",
		Category:      "conflict_monitoring",
		AuthorityType: "national_security",
	}
	query := buildDDGQuery(target)
	if !containsAll(query, "site:x.com", "site:t.me", "alerts OR updates OR statements") {
		t.Fatalf("expected social-site DDG query for conflict target, got %q", query)
	}
}

func TestLooksLikeOfficialSite(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.ncsc.gov.uk/rss", true},
		{"https://cert.se/feed", true},
		{"https://www.police.uk/news", true},
		{"https://www.bbc.com/news", false},
		{"https://www.justice.gov/feeds", true},
		{"https://www.reddit.com/r/netsec", false},
	}

	for _, tt := range tests {
		got := looksLikeOfficialSite(tt.url)
		if got != tt.want {
			t.Errorf("looksLikeOfficialSite(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestLooksLikeSocialSignalURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://x.com/IDF", true},
		{"https://x.com/search?q=idf", false},
		{"https://t.me/s/auroraintel", true},
		{"https://t.me", false},
		{"https://example.com/signal", false},
	}
	for _, tt := range tests {
		if got := looksLikeSocialSignalURL(tt.url); got != tt.want {
			t.Errorf("looksLikeSocialSignalURL(%q)=%v want %v", tt.url, got, tt.want)
		}
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
