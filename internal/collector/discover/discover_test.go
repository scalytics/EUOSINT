// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/vet"
)

func TestDetectFeedTypeRSS(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`
	if got := detectFeedType(body); got != "rss" {
		t.Errorf("expected rss, got %q", got)
	}
}

func TestDetectFeedTypeAtom(t *testing.T) {
	body := `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Test</title></feed>`
	if got := detectFeedType(body); got != "atom" {
		t.Errorf("expected atom, got %q", got)
	}
}

func TestDetectFeedTypeHTML(t *testing.T) {
	body := `<!DOCTYPE html><html><head><title>Press Releases</title></head><body></body></html>`
	if got := detectFeedType(body); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://www.example.com/feed/", "www.example.com/feed"},
		{"http://Example.COM/RSS", "example.com/rss"},
		{"  https://foo.bar/  ", "foo.bar"},
	}
	for _, tt := range tests {
		got := normalizeURL(tt.input)
		if got != tt.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCommonPressReleasePathsNotEmpty(t *testing.T) {
	if len(commonPressReleasePaths) == 0 {
		t.Fatal("expected non-empty press release paths")
	}
	for _, p := range commonPressReleasePaths {
		if !strings.HasPrefix(p, "/") {
			t.Errorf("press release path should start with /: %q", p)
		}
	}
}

func TestPoliceAgencyQueryNotEmpty(t *testing.T) {
	query := buildPoliceAgencyQuery(policeAgencyTypeIDs[:2])
	if strings.TrimSpace(query) == "" {
		t.Fatal("SPARQL query should not be empty")
	}
	// Basic sanity — query must select the fields we parse.
	for _, field := range []string{"agencyLabel", "website", "countryLabel", "countryCode"} {
		if !strings.Contains(query, field) {
			t.Errorf("SPARQL query missing field %q", field)
		}
	}
}

func TestDiscoveryHygieneRejectsLocalPolice(t *testing.T) {
	if passesDiscoveryHygiene("City of Valletta Police Department", "https://city.police.example", "police") {
		t.Fatal("expected local police source to fail hygiene gate")
	}
	if !passesDiscoveryHygiene("Europol", "https://www.europol.europa.eu", "police") {
		t.Fatal("expected supranational source to pass hygiene gate")
	}
}

func TestLoadCandidateQueueAndDeadLetterSkip(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.json")
	deadPath := filepath.Join(dir, "dead.json")
	if err := os.WriteFile(candidatePath, []byte(`{"sources":[{"url":"https://example.test/news","authority_name":"Example Agency","authority_type":"police","category":"public_appeal","country":"France","country_code":"FR"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deadPath, []byte(`{"sources":[{"feed_url":"https://example.test/news"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	candidates := loadCandidateQueue(candidatePath)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	dead := loadDeadLetterQueue(deadPath)
	if !isDeadLettered(candidates[0], dead) {
		t.Fatal("expected candidate to be skipped when present in dead-letter queue")
	}
	if isDeadLettered(model.SourceCandidate{URL: "https://other.test/feed"}, dead) {
		t.Fatal("unexpected dead-letter match for unrelated candidate")
	}
}

func TestMergeCandidatesSkipsDeadAndActive(t *testing.T) {
	merged := mergeCandidates(
		[]model.SourceCandidate{
			{URL: "https://existing-queue.test/feed", AuthorityName: "Queued"},
		},
		[]model.SourceCandidate{
			{URL: "https://active.test/feed", AuthorityName: "Active"},
			{URL: "https://dead.test/feed", AuthorityName: "Dead"},
			{URL: "https://new.test/feed", AuthorityName: "New"},
		},
		map[string]struct{}{
			normalizeURL("https://active.test/feed"): {},
		},
		[]model.SourceReplacementCandidate{
			{FeedURL: "https://dead.test/feed"},
		},
	)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged candidates, got %d", len(merged))
	}
	if normalizeURL(merged[0].URL) != normalizeURL("https://existing-queue.test/feed") {
		t.Fatalf("unexpected first merged candidate %#v", merged[0])
	}
	if normalizeURL(merged[1].URL) != normalizeURL("https://new.test/feed") {
		t.Fatalf("unexpected second merged candidate %#v", merged[1])
	}
}

type stubSearchCompleter struct {
	content string
	err     error
}

func (s stubSearchCompleter) Complete(_ context.Context, _ []vet.Message) (string, error) {
	return s.content, s.err
}

func TestDecodeLLMSearchResponse(t *testing.T) {
	resp, err := decodeLLMSearchResponse("```json\n{\"urls\":[{\"url\":\"https://www.europol.europa.eu/cms/api/rss/news\",\"reason\":\"official rss\"}]}\n```")
	if err != nil {
		t.Fatalf("decodeLLMSearchResponse returned error: %v", err)
	}
	if len(resp.URLs) != 1 || resp.URLs[0].URL != "https://www.europol.europa.eu/cms/api/rss/news" {
		t.Fatalf("unexpected decoded search response: %#v", resp)
	}
}

func TestSelectSearchTargetsHonorsCap(t *testing.T) {
	cfg := config.Default()
	cfg.SearchDiscoveryMaxTargets = 2
	targets := selectSearchTargets(cfg, []model.SourceCandidate{
		{AuthorityName: "Europol", URL: "https://www.europol.europa.eu", AuthorityType: "police", Category: "public_appeal"},
		{AuthorityName: "Interpol", URL: "https://www.interpol.int", AuthorityType: "police", Category: "wanted_suspect"},
		{AuthorityName: "FIRST", URL: "https://www.first.org", AuthorityType: "cert", Category: "cyber_advisory"},
	})
	if len(targets) != 2 {
		t.Fatalf("expected 2 search targets, got %d", len(targets))
	}
}

func TestLLMSearchCandidatesReturnsTokenSafeCandidates(t *testing.T) {
	cfg := config.Default()
	cfg.SearchDiscoveryEnabled = true
	cfg.SearchDiscoveryMaxTargets = 1
	cfg.SearchDiscoveryMaxURLsPerTarget = 2
	cfg.VettingProvider = "xai"

	got, err := llmSearchCandidates(context.Background(), cfg, stubSearchCompleter{
		content: `{"urls":[{"url":"https://www.europol.europa.eu/cms/api/rss/news","reason":"official rss"},{"url":"https://www.europol.europa.eu/newsroom","reason":"official newsroom"},{"url":"not-a-url","reason":"ignore"}]}`,
	}, []model.SourceCandidate{
		{AuthorityName: "Europol", URL: "https://www.europol.europa.eu", AuthorityType: "police", Category: "public_appeal", Country: "Netherlands", CountryCode: "NL"},
	})
	if err != nil {
		t.Fatalf("llmSearchCandidates returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 llm-search candidates, got %d", len(got))
	}
	if !strings.HasPrefix(got[0].Notes, "llm-search:xai") {
		t.Fatalf("expected llm-search note, got %q", got[0].Notes)
	}
	if got[0].AuthorityName != "Europol" || got[0].CountryCode != "NL" {
		t.Fatalf("expected metadata to be preserved, got %#v", got[0])
	}
}
