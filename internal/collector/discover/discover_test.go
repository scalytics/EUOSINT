// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/fetch"
	"github.com/scalytics/kafSIEM/internal/collector/model"
	"github.com/scalytics/kafSIEM/internal/collector/vet"
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
	query := buildPoliceAgencyQuery(policeAgencyTypeIDs[0])
	if strings.TrimSpace(query) == "" {
		t.Fatal("SPARQL query should not be empty")
	}
	// Basic sanity — query must select the fields we parse.
	for _, field := range []string{"website", "countryCode"} {
		if !strings.Contains(query, field) {
			t.Errorf("SPARQL query missing field %q", field)
		}
	}
}

func TestDiscoveryHygieneAllowsOfficialPoliceHosts(t *testing.T) {
	if !passesDiscoveryHygiene("New Zealand Police", "https://www.police.govt.nz", "police") {
		t.Fatal("expected official police source to pass hygiene gate")
	}
	if passesDiscoveryHygiene("City of Valletta Police Department", "https://city.police.example", "police") {
		t.Fatal("expected city-scoped host to fail hygiene gate")
	}
	if !passesDiscoveryHygiene("Europol", "https://www.europol.europa.eu", "police") {
		t.Fatal("expected supranational source to pass hygiene gate")
	}
}

func TestDiscoveryHygieneDoesNotTreatTransportAsSport(t *testing.T) {
	if !passesDiscoveryHygiene("Ministry of Transport", "https://transport.gov.example", "national_security") {
		t.Fatal("expected transport ministry to avoid sport false-positive")
	}
}

func TestDiscoveryHygieneSocialSignals(t *testing.T) {
	if !passesDiscoveryHygiene("IDF Official", "https://x.com/IDF", "national_security") {
		t.Fatal("expected vetted national-security X source to pass hygiene gate")
	}
	if passesDiscoveryHygiene("Generic CERT", "https://x.com/someaccount", "cert") {
		t.Fatal("expected non-security authority type to fail social-source hygiene gate")
	}
}

func TestSearchTopicLabelIncludesNewCategories(t *testing.T) {
	if got := searchTopicLabel("maritime_security", "national_security"); !strings.Contains(got, "maritime security") {
		t.Fatalf("expected maritime topic label, got %q", got)
	}
	if got := searchTopicLabel("legislative", "regulatory"); !strings.Contains(got, "sanctions") {
		t.Fatalf("expected legislative topic label, got %q", got)
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

func TestLoadSovereignSeedCandidatesAppliesLegislativeDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sovereign.json")
	if err := os.WriteFile(path, []byte(`{"sources":[{"url":"https://president.example/news","authority_name":"Presidency of Example","country":"Exampleland","country_code":"ex"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadSovereignSeedCandidates(path)
	if len(got) != 1 {
		t.Fatalf("expected 1 sovereign seed candidate, got %d", len(got))
	}
	if got[0].Category != "legislative" {
		t.Fatalf("expected legislative default category, got %q", got[0].Category)
	}
	if got[0].AuthorityType != "government" {
		t.Fatalf("expected government default authority_type, got %q", got[0].AuthorityType)
	}
	if got[0].CountryCode != "EX" {
		t.Fatalf("expected uppercase country code, got %q", got[0].CountryCode)
	}
	if !strings.Contains(got[0].Notes, "official-statements") {
		t.Fatalf("expected official-statements seed note, got %q", got[0].Notes)
	}
}

func TestIsOfficialStatementsCandidateOnlyForLegislative(t *testing.T) {
	if isOfficialStatementsCandidate(model.SourceCandidate{
		Category:      "cyber_advisory",
		AuthorityType: "government",
		Notes:         "autonomous seed: sovereign-official-statements",
	}) {
		t.Fatal("expected non-legislative category to bypass official-statements strict gate")
	}
	if !isOfficialStatementsCandidate(model.SourceCandidate{
		Category:      "legislative",
		AuthorityType: "government",
	}) {
		t.Fatal("expected legislative government source to use official-statements strict gate")
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
		content: `{"urls":[{"url":"https://www.europol.europa.eu/cms/api/rss/news","reason":"official rss"},{"url":"https://www.europol.europa.eu/feed.xml","reason":"official atom"},{"url":"https://www.europol.europa.eu/newsroom","reason":"ignore non-feed"}]}`,
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

func TestFirstWebsiteFieldAcceptsStringOrArray(t *testing.T) {
	var got struct {
		Website firstWebsiteField `json:"website"`
	}
	if err := json.Unmarshal([]byte(`{"website":"https://example.test"}`), &got); err != nil {
		t.Fatalf("unmarshal string website: %v", err)
	}
	if string(got.Website) != "https://example.test" {
		t.Fatalf("unexpected string website %q", got.Website)
	}
	if err := json.Unmarshal([]byte(`{"website":["","https://array.example/feed"]}`), &got); err != nil {
		t.Fatalf("unmarshal array website: %v", err)
	}
	if string(got.Website) != "https://array.example/feed" {
		t.Fatalf("unexpected array website %q", got.Website)
	}
}

func TestNormalizeFIRSTWebsite(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "host only", in: "cert.example.org", want: "https://cert.example.org"},
		{name: "full url", in: "https://csirt.example.org/feed", want: "https://csirt.example.org/feed"},
		{name: "invalid spaces", in: "A1 Telekom Austria AG", want: ""},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		if got := normalizeFIRSTWebsite(tt.in); got != tt.want {
			t.Fatalf("%s: normalizeFIRSTWebsite(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestShouldRetryCandidateQueueSkipsHardHTTPFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.Default()
	client := fetch.New(cfg)
	candidate := model.SourceCandidate{URL: srv.URL}
	if shouldRetryCandidateQueue(context.Background(), client, candidate, srv.URL) {
		t.Fatal("expected 404 candidate to be removed from queue")
	}
}

func TestBuildReplacementSearchTargetsUsesMetadataNotDeadURL(t *testing.T) {
	targets := buildReplacementSearchTargets([]model.SourceReplacementCandidate{
		{
			SourceID:      "bka",
			AuthorityName: "Bundeskriminalamt",
			FeedURL:       "https://dead.example/rss",
			BaseURL:       "https://www.bka.de",
			Country:       "Germany",
			CountryCode:   "DE",
			Region:        "Europe",
			AuthorityType: "police",
			Category:      "wanted_suspect",
		},
	})
	if len(targets) != 1 {
		t.Fatalf("expected 1 replacement search target, got %d", len(targets))
	}
	if targets[0].BaseURL != "https://www.bka.de" {
		t.Fatalf("expected base URL to be used for replacement search, got %#v", targets[0])
	}
	if targets[0].URL != "" {
		t.Fatalf("expected dead feed URL not to be reintroduced as direct candidate, got %#v", targets[0])
	}
	if !strings.HasPrefix(targets[0].Notes, "replacement-search:") {
		t.Fatalf("expected replacement-search note, got %#v", targets[0])
	}
}

func TestIsDiscoveryTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "timeout text", err: errors.New("fetch failed: context deadline exceeded"), want: true},
		{name: "request canceled", err: errors.New("request canceled while awaiting headers"), want: true},
		{name: "parse failure", err: errors.New("json parse error"), want: false},
	}
	for _, tt := range tests {
		if got := isDiscoveryTimeout(tt.err); got != tt.want {
			t.Fatalf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
}

func TestWikidataCacheRoundTrip(t *testing.T) {
	cfg := config.Default()
	cfg.WikidataCachePath = t.TempDir()
	cfg.WikidataCacheTTLHours = 24

	url := "https://query.wikidata.org/sparql?format=json&query=test"
	want := []byte(`{"results":{"bindings":[]}}`)
	writeWikidataCache(cfg, url, want)

	got, ok := readWikidataCache(cfg, url)
	if !ok {
		t.Fatal("expected cached wikidata response to be readable")
	}
	if string(got) != string(want) {
		t.Fatalf("unexpected cache body %q", string(got))
	}
}

func TestVetAndPromoteUsesVettedCategory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<?xml version="1.0"?><rss><channel><item><title>Actionable update</title><link>https://example.test/item/1</link><pubDate>Mon, 02 Jan 2026 15:04:05 MST</pubDate><description>Operational bulletin</description></item></channel></rss>`)
	})
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"approve":true,"promotion_status":"active","category":"informational","level":"national","mission_tags":["briefing"],"source_quality":0.9,"operational_relevance":0.85,"reason":"approved"}`}},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.Default()
	cfg.VettingBaseURL = server.URL
	cfg.SourceVettingRequired = true
	cfg.SourceMinQuality = 0.6
	cfg.SourceMinOperationalRelevance = 0.6
	cfg.VettingMaxSampleItems = 2

	candidate := model.SourceCandidate{
		URL:           server.URL + "/feed",
		BaseURL:       server.URL,
		AuthorityName: "Europol",
		AuthorityType: "police",
		Category:      "public_appeal",
		Country:       "Netherlands",
		CountryCode:   "NL",
	}
	discovered := DiscoveredSource{
		FeedURL:       server.URL + "/feed",
		FeedType:      "rss",
		AuthorityType: "police",
		Category:      "wanted_suspect",
		OrgName:       "Europol",
		Country:       "Netherlands",
		CountryCode:   "NL",
		TeamURL:       server.URL,
	}

	promoted, verdict, err := vetAndPromote(context.Background(), cfg, fetch.New(cfg), vet.New(cfg), candidate, discovered)
	if err != nil {
		t.Fatalf("vetAndPromote returned error: %v", err)
	}
	if promoted == nil {
		t.Fatalf("expected promoted source, got nil (verdict=%#v)", verdict)
	}
	if promoted.Category != "informational" {
		t.Fatalf("expected vetted category to be persisted, got %q", promoted.Category)
	}
}

func TestVetAndPromoteThresholdGateRejectsLowQuality(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<?xml version="1.0"?><rss><channel><item><title>Threat bulletin</title><link>https://example.test/item/2</link><pubDate>Mon, 02 Jan 2026 15:04:05 MST</pubDate><description>Operational bulletin</description></item></channel></rss>`)
	})
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"approve":true,"promotion_status":"active","category":"cyber_advisory","level":"national","mission_tags":["threat-intel"],"source_quality":0.25,"operational_relevance":0.9,"reason":"approved"}`}},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.Default()
	cfg.VettingBaseURL = server.URL
	cfg.SourceVettingRequired = true
	cfg.SourceMinQuality = 0.6
	cfg.SourceMinOperationalRelevance = 0.6
	cfg.VettingMaxSampleItems = 2

	candidate := model.SourceCandidate{
		URL:           server.URL + "/feed",
		BaseURL:       server.URL,
		AuthorityName: "National CERT",
		AuthorityType: "cert",
		Category:      "cyber_advisory",
		Country:       "Germany",
		CountryCode:   "DE",
	}
	discovered := DiscoveredSource{
		FeedURL:       server.URL + "/feed",
		FeedType:      "rss",
		AuthorityType: "cert",
		Category:      "cyber_advisory",
		OrgName:       "National CERT",
		Country:       "Germany",
		CountryCode:   "DE",
		TeamURL:       server.URL,
	}

	promoted, verdict, err := vetAndPromote(context.Background(), cfg, fetch.New(cfg), vet.New(cfg), candidate, discovered)
	if err != nil {
		t.Fatalf("vetAndPromote returned error: %v", err)
	}
	if promoted != nil {
		t.Fatalf("expected source to be rejected by threshold gate, got %+v", *promoted)
	}
	if verdict.Approve {
		t.Fatalf("expected approve=false after threshold gate, got %+v", verdict)
	}
	if !strings.Contains(verdict.Reason, "below source quality threshold") {
		t.Fatalf("expected quality-threshold reason, got %q", verdict.Reason)
	}
}

func TestVetAndPromoteOfficialStatementsUsesStricterThresholds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<?xml version="1.0"?><rss><channel><item><title>Official statement</title><link>https://example.test/item/7</link><pubDate>Mon, 02 Jan 2026 15:04:05 MST</pubDate><description>Strategic update</description></item></channel></rss>`)
	})
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"approve":true,"promotion_status":"active","category":"legislative","level":"national","mission_tags":["official_statement"],"source_quality":0.72,"operational_relevance":0.72,"reason":"approved"}`}},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.Default()
	cfg.VettingBaseURL = server.URL
	cfg.SourceVettingRequired = true
	cfg.SourceMinQuality = 0.6
	cfg.SourceMinOperationalRelevance = 0.6
	cfg.OfficialStatementsMinQuality = 0.75
	cfg.OfficialStatementsMinOperational = 0.7
	cfg.VettingMaxSampleItems = 2

	candidate := model.SourceCandidate{
		URL:           server.URL + "/feed",
		BaseURL:       server.URL,
		AuthorityName: "Presidency of Example",
		AuthorityType: "government",
		Category:      "legislative",
		Country:       "Exampleland",
		CountryCode:   "EX",
		Notes:         "autonomous seed: sovereign-official-statements",
	}
	discovered := DiscoveredSource{
		FeedURL:       server.URL + "/feed",
		FeedType:      "rss",
		AuthorityType: "government",
		Category:      "legislative",
		OrgName:       "Presidency of Example",
		Country:       "Exampleland",
		CountryCode:   "EX",
		TeamURL:       server.URL,
	}

	promoted, verdict, err := vetAndPromote(context.Background(), cfg, fetch.New(cfg), vet.New(cfg), candidate, discovered)
	if err != nil {
		t.Fatalf("vetAndPromote returned error: %v", err)
	}
	if promoted != nil {
		t.Fatalf("expected official-statement source to be rejected by stricter quality threshold, got %+v", *promoted)
	}
	if verdict.Approve {
		t.Fatalf("expected approve=false after official-statement strict gate, got %+v", verdict)
	}
	if !strings.Contains(verdict.Reason, "below source quality threshold") {
		t.Fatalf("expected quality-threshold reason, got %q", verdict.Reason)
	}
}

func TestSampleSourceParsesRSSItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<?xml version="1.0"?><rss><channel><item><title>Bulletin A</title><link>https://example.test/a</link></item><item><title>Bulletin B</title><link>https://example.test/b</link></item></channel></rss>`)
	}))
	defer server.Close()

	cfg := config.Default()
	samples, err := sampleSource(context.Background(), fetch.New(cfg), DiscoveredSource{
		FeedURL:  server.URL,
		FeedType: "rss",
	}, 1)
	if err != nil {
		t.Fatalf("sampleSource returned error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected sample limit to apply, got %d samples", len(samples))
	}
	if samples[0].Title != "Bulletin A" {
		t.Fatalf("unexpected sample title: %q", samples[0].Title)
	}
}

func TestStructuredDiscoveryWeeklyCadence(t *testing.T) {
	cfg := config.Default()
	cfg.WikidataCachePath = t.TempDir()
	cfg.StructuredDiscoveryIntervalHours = 168

	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	due, nextAt, err := shouldRunStructuredDiscovery(cfg, now)
	if err != nil {
		t.Fatalf("shouldRunStructuredDiscovery initial: %v", err)
	}
	if !due {
		t.Fatalf("expected due on first run, nextAt=%s", nextAt)
	}
	if err := markStructuredDiscoveryRun(cfg, now); err != nil {
		t.Fatalf("markStructuredDiscoveryRun: %v", err)
	}

	due, nextAt, err = shouldRunStructuredDiscovery(cfg, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("shouldRunStructuredDiscovery +24h: %v", err)
	}
	if due {
		t.Fatalf("expected not due at +24h, nextAt=%s", nextAt)
	}

	due, _, err = shouldRunStructuredDiscovery(cfg, now.Add(8*24*time.Hour))
	if err != nil {
		t.Fatalf("shouldRunStructuredDiscovery +8d: %v", err)
	}
	if !due {
		t.Fatal("expected due after weekly interval elapsed")
	}
}

// ---------- inferLanguageCode ----------

func TestInferLanguageCodeLLMPreferred(t *testing.T) {
	// LLM detection should take priority over country fallback
	got := inferLanguageCode("fr", "US")
	if got != "fr" {
		t.Errorf("expected LLM lang 'fr', got %q", got)
	}
}

func TestInferLanguageCodeCountryFallback(t *testing.T) {
	cases := map[string]string{
		"IS": "is", "HU": "hu", "JP": "ja", "BR": "pt",
		"DE": "de", "FR": "fr", "SA": "ar", "CN": "zh",
		"KE": "en", "GH": "en", "SN": "fr", "CM": "fr",
		"GE": "ka", "AM": "hy", "AZ": "az", "UZ": "uz",
		"CU": "es", "HT": "fr", "AO": "pt", "MK": "mk",
		"AL": "sq", "BY": "ru", "KH": "km", "LA": "lo",
	}
	for cc, want := range cases {
		got := inferLanguageCode("", cc)
		if got != want {
			t.Errorf("inferLanguageCode('', %q) = %q, want %q", cc, got, want)
		}
	}
}

func TestInferLanguageCodeSyncWithNormalize(t *testing.T) {
	// Verify that discovery and normalize agree on key mappings.
	// If this test fails, the two maps are out of sync.
	spotCheck := map[string]string{
		"IS": "is", "HU": "hu", "DE": "de", "FR": "fr",
		"ES": "es", "PT": "pt", "RU": "ru", "JP": "ja",
		"SA": "ar", "CN": "zh", "TH": "th", "PL": "pl",
		"GE": "ka", "AM": "hy", "KH": "km",
	}
	for cc, want := range spotCheck {
		got := inferLanguageCode("", cc)
		if got != want {
			t.Errorf("inferLanguageCode('', %q) = %q, want %q — discovery/normalize out of sync?", cc, got, want)
		}
	}
}
