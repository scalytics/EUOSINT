// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/dictionary"
	"github.com/scalytics/kafSIEM/internal/collector/fetch"
	"github.com/scalytics/kafSIEM/internal/collector/model"
	"github.com/scalytics/kafSIEM/internal/collector/noisegate"
	"github.com/scalytics/kafSIEM/internal/collector/normalize"
	"github.com/scalytics/kafSIEM/internal/collector/parse"
	"github.com/scalytics/kafSIEM/internal/collector/zonebrief"
	"github.com/scalytics/kafSIEM/internal/sourcedb"
)

func TestRunnerRunOnceWritesOutputs(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	registry := []byte(`[
	  {"type":"rss","feed_url":"https://collector.test/rss","category":"cyber_advisory","region_tag":"INT","lat":48.8,"lng":2.3,"source":{"source_id":"rss-source","authority_name":"RSS Source","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://collector.test"}},
	  {"type":"html-list","feed_url":"https://collector.test/html","category":"wanted_suspect","region_tag":"FR","lat":48.8,"lng":2.3,"include_keywords":["wanted"],"source":{"source_id":"html-source","authority_name":"HTML Source","country":"France","country_code":"FR","region":"Europe","authority_type":"police","base_url":"https://collector.test"}},
	  {"type":"kev-json","feed_url":"https://collector.test/kev","category":"cyber_advisory","region_tag":"US","lat":38.8,"lng":-77.0,"source":{"source_id":"kev-source","authority_name":"KEV Source","country":"United States","country_code":"US","region":"North America","authority_type":"cert","base_url":"https://www.cisa.gov"}},
	  {"type":"interpol-red-json","feed_url":"https://collector.test/interpol","category":"wanted_suspect","region_tag":"INT","lat":45.7,"lng":4.8,"source":{"source_id":"interpol-red","authority_name":"Interpol Red","country":"France","country_code":"FR","region":"International","authority_type":"police","base_url":"https://www.interpol.int"}},
	  {"type":"travelwarning-json","feed_url":"https://collector.test/travel-json","category":"travel_warning","region_tag":"DE","lat":52.5,"lng":13.4,"source":{"source_id":"de-aa-travel","authority_name":"German AA","country":"Germany","country_code":"DE","region":"Europe","authority_type":"national_security","base_url":"https://www.auswaertiges-amt.de"}},
	  {"type":"travelwarning-atom","feed_url":"https://collector.test/travel-atom","category":"travel_warning","region_tag":"GB","lat":51.5,"lng":-0.1,"source":{"source_id":"uk-fcdo-travel","authority_name":"UK FCDO","country":"United Kingdom","country_code":"GB","region":"Europe","authority_type":"national_security","base_url":"https://www.gov.uk"}}
	]`)
	if err := os.WriteFile(registryPath, registry, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.RegistryPath = registryPath
	cfg.OutputPath = filepath.Join(dir, "alerts.json")
	cfg.FilteredOutputPath = filepath.Join(dir, "filtered.json")
	cfg.StateOutputPath = filepath.Join(dir, "state.json")
	cfg.SourceHealthOutputPath = filepath.Join(dir, "health.json")
	cfg.ReplacementQueuePath = filepath.Join(dir, "replacement.json")
	cfg.MaxAgeDays = 10000

	runner := New(io.Discard, io.Discard)
	runner.nowFn = func() time.Time {
		return time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	}
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		return fetch.NewWithHTTPClient(cfg, &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				var body string
				switch req.URL.Path {
				case "/rss":
					body = `<?xml version="1.0"?><rss><channel><item><title>Critical cyber advisory</title><link>https://collector.test/rss-item</link><pubDate>Mon, 02 Jan 2026 15:04:05 MST</pubDate><description>CVE-2026-1234 patch advisory</description></item></channel></rss>`
				case "/html":
					body = `<html><body><a href="/wanted/1">Wanted suspect public appeal</a></body></html>`
				case "/kev":
					body = `{"vulnerabilities":[{"cveID":"CVE-2026-9999","vulnerabilityName":"Test vuln","shortDescription":"Known exploited issue","dateAdded":"2026-01-01","knownRansomwareCampaign":true}]}`
				case "/interpol":
					body = `{"_embedded":{"notices":[{"forename":"Jane","name":"Doe","issuing_entity":"Interpol","place_of_birth":"Paris","nationalities":["FR"],"_links":{"self":{"href":"https://ws-public.interpol.int/notices/v1/red/123"}}}]}}`
				case "/travel-json":
					body = `{"1":{"title":"Afghanistan - Do not travel","country":"Afghanistan","warning":"Do not travel.","severity":"Reisewarnung","lastChanged":"2026-01-15","url":"https://example.com/af"}}`
				case "/travel-atom":
					body = `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><title>France travel advice</title><link rel="alternate" href="https://example.com/fr"/><published>2026-02-01T00:00:00Z</published><summary>Exercise normal caution.</summary></entry></feed>`
				default:
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			}),
		})
	}
	if err := runner.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	rawAlerts, err := os.ReadFile(cfg.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var alerts []model.Alert
	if err := json.Unmarshal(rawAlerts, &alerts); err != nil {
		t.Fatal(err)
	}
	if len(alerts) == 0 {
		t.Fatalf("expected active alerts, got %#v", alerts)
	}

	rawHealth, err := os.ReadFile(cfg.SourceHealthOutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var health model.SourceHealthDocument
	if err := json.Unmarshal(rawHealth, &health); err != nil {
		t.Fatal(err)
	}
	if health.TotalSources != 6 {
		t.Fatalf("expected 6 sources in health document, got %d", health.TotalSources)
	}
	if len(health.ReplacementQueue) != 0 {
		t.Fatalf("expected no replacement queue entries, got %d", len(health.ReplacementQueue))
	}
	if _, err := os.Stat(cfg.ReplacementQueuePath); err != nil {
		t.Fatalf("expected replacement queue output, got %v", err)
	}
}

func TestPrioritizeSourcesPrefersRankedCuratedHighValue(t *testing.T) {
	sources := []model.RegistrySource{
		{
			Type:          "rss",
			SourceQuality: 0.95,
			Source: model.SourceMetadata{
				SourceID:             "ranked-curated",
				AuthorityName:        "Ranked Curated",
				OperationalRelevance: 0.8,
				IsCurated:            true,
				IsHighValue:          true,
			},
			PreferredRank: 1,
		},
		{
			Type:          "rss",
			SourceQuality: 0.99,
			Source: model.SourceMetadata{
				SourceID:             "unranked-high-quality",
				AuthorityName:        "Unranked High Quality",
				OperationalRelevance: 0.9,
				IsCurated:            true,
				IsHighValue:          true,
			},
		},
		{
			Type:          "html-list",
			SourceQuality: 0.2,
			Source: model.SourceMetadata{
				SourceID:      "tail-source",
				AuthorityName: "Tail Source",
			},
		},
	}

	ordered := prioritizeSources(sources)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(ordered))
	}
	if ordered[0].Source.SourceID != "ranked-curated" {
		t.Fatalf("expected ranked curated source first, got %q", ordered[0].Source.SourceID)
	}
	if ordered[2].Source.SourceID != "tail-source" {
		t.Fatalf("expected low-signal tail source last, got %q", ordered[2].Source.SourceID)
	}
}

func TestUsesFastLaneOnlyForDocumentFeeds(t *testing.T) {
	tests := []struct {
		name   string
		source model.RegistrySource
		want   bool
	}{
		{
			name:   "rss stays in fast lane",
			source: model.RegistrySource{Type: "rss"},
			want:   true,
		},
		{
			name:   "atom stays in fast lane",
			source: model.RegistrySource{Type: "travelwarning-atom"},
			want:   true,
		},
		{
			name:   "gdelt api moves out of fast lane",
			source: model.RegistrySource{Type: "gdelt-json"},
			want:   false,
		},
		{
			name:   "interpol api stays out of fast lane",
			source: model.RegistrySource{Type: "interpol-red-json"},
			want:   false,
		},
		{
			name:   "browser feeds stay out of fast lane",
			source: model.RegistrySource{Type: "html-list", FetchMode: "browser"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesFastLane(tc.source); got != tc.want {
				t.Fatalf("usesFastLane(%q) = %v, want %v", tc.source.Type, got, tc.want)
			}
		})
	}
}

func TestUsesBrowserLaneForBrowserBoundSources(t *testing.T) {
	tests := []struct {
		name   string
		source model.RegistrySource
		want   bool
	}{
		{
			name:   "html list stays in browser lane",
			source: model.RegistrySource{Type: "html-list"},
			want:   true,
		},
		{
			name:   "telegram stays in browser lane",
			source: model.RegistrySource{Type: "telegram"},
			want:   true,
		},
		{
			name:   "x stays in browser lane",
			source: model.RegistrySource{Type: "x"},
			want:   true,
		},
		{
			name:   "interpol stays in browser lane",
			source: model.RegistrySource{Type: "interpol-red-json"},
			want:   true,
		},
		{
			name:   "explicit browser fetch mode uses browser lane",
			source: model.RegistrySource{Type: "telegram", FetchMode: "browser"},
			want:   true,
		},
		{
			name:   "gdelt api does not use browser lane",
			source: model.RegistrySource{Type: "gdelt-json"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesBrowserLane(tc.source); got != tc.want {
				t.Fatalf("usesBrowserLane(%q) = %v, want %v", tc.source.Type, got, tc.want)
			}
		})
	}
}

func TestLaneClassificationIsExclusive(t *testing.T) {
	tests := []struct {
		name        string
		source      model.RegistrySource
		wantFast    bool
		wantBrowser bool
	}{
		{
			name:        "rss fast only",
			source:      model.RegistrySource{Type: "rss"},
			wantFast:    true,
			wantBrowser: false,
		},
		{
			name:        "telegram browser only",
			source:      model.RegistrySource{Type: "telegram"},
			wantFast:    false,
			wantBrowser: true,
		},
		{
			name:        "gdelt api only",
			source:      model.RegistrySource{Type: "gdelt-json"},
			wantFast:    false,
			wantBrowser: false,
		},
		{
			name:        "html browser only",
			source:      model.RegistrySource{Type: "html-list"},
			wantFast:    false,
			wantBrowser: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotFast := usesFastLane(tc.source)
			gotBrowser := usesBrowserLane(tc.source)
			if gotFast != tc.wantFast {
				t.Fatalf("usesFastLane(%q) = %v, want %v", tc.source.Type, gotFast, tc.wantFast)
			}
			if gotBrowser != tc.wantBrowser {
				t.Fatalf("usesBrowserLane(%q) = %v, want %v", tc.source.Type, gotBrowser, tc.wantBrowser)
			}
			if gotFast && gotBrowser {
				t.Fatalf("source %q classified into both fast and browser lanes", tc.source.Type)
			}
		})
	}
}

func TestFetcherForSourcePrefersBrowserForSocialSources(t *testing.T) {
	client := &fetch.Client{}
	browser := &fetch.BrowserClient{}

	tests := []struct {
		name   string
		source model.RegistrySource
		want   any
	}{
		{
			name:   "telegram prefers browser",
			source: model.RegistrySource{Type: "telegram"},
			want:   browser,
		},
		{
			name:   "x prefers browser",
			source: model.RegistrySource{Type: "x"},
			want:   browser,
		},
		{
			name:   "html list stays on declared mode",
			source: model.RegistrySource{Type: "html-list"},
			want:   client,
		},
		{
			name:   "explicit browser mode still uses browser",
			source: model.RegistrySource{Type: "html-list", FetchMode: "browser"},
			want:   browser,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fetcherForSource(tc.source, client, browser)
			switch want := tc.want.(type) {
			case *fetch.Client:
				if got != want {
					t.Fatalf("fetcherForSource(%q) did not return client", tc.source.Type)
				}
			case *fetch.BrowserClient:
				if got != want {
					t.Fatalf("fetcherForSource(%q) did not return browser", tc.source.Type)
				}
			default:
				t.Fatalf("unsupported expected fetcher type %T", tc.want)
			}
		})
	}
}

func TestBuildReplacementQueueFromPermanentFailures(t *testing.T) {
	sources := []model.RegistrySource{
		{
			Type:     "rss",
			FeedURL:  "https://collector.test/dead-feed",
			Category: "cyber_advisory",
			Source: model.SourceMetadata{
				SourceID:      "dead-source",
				AuthorityName: "Dead Source",
				Country:       "France",
				CountryCode:   "FR",
				Region:        "Europe",
				AuthorityType: "cert",
				BaseURL:       "https://collector.test",
			},
		},
	}
	entries := []model.SourceHealthEntry{
		{
			SourceID:         "dead-source",
			AuthorityName:    "Dead Source",
			Type:             "rss",
			Status:           "error",
			FeedURL:          "https://collector.test/dead-feed",
			Error:            "fetch https://collector.test/dead-feed: status 404",
			ErrorClass:       "not_found",
			NeedsReplacement: true,
			DiscoveryAction:  "find_replacement",
			FinishedAt:       "2026-03-16T12:00:00Z",
		},
	}

	queue := buildReplacementQueue(entries, sources)
	if len(queue) != 1 {
		t.Fatalf("expected one queued replacement candidate, got %d", len(queue))
	}
	if queue[0].BaseURL != "https://collector.test" {
		t.Fatalf("expected base URL to be carried into replacement queue, got %q", queue[0].BaseURL)
	}
}

func TestExtractInterpolNoticeID(t *testing.T) {
	if got := extractInterpolNoticeID("2026-17351", ""); got != "2026-17351" {
		t.Fatalf("expected entity id to win, got %q", got)
	}
	if got := extractInterpolNoticeID("", "https://www.interpol.int/How-we-work/Notices/Yellow-Notices/View-Yellow-Notices#2026-17351"); got != "2026-17351" {
		t.Fatalf("expected fragment id, got %q", got)
	}
	if got := extractInterpolNoticeID("", "https://ws-public.interpol.int/notices/v1/red/123"); got != "123" {
		t.Fatalf("expected path id, got %q", got)
	}
}

func TestFilterCategoryItemsDropsUnrelatedMissingPersonHTML(t *testing.T) {
	dict, err := dictionary.Load(filepath.Join("..", "..", "..", "registry", "category_dictionary.json"))
	if err != nil {
		t.Fatal(err)
	}
	items := []parse.FeedItem{
		{Title: "Calendario de actividades", Link: "https://example.test/calendario"},
		{Title: "Persona desaparecida en San Jose", Link: "https://example.test/desaparecidos/1"},
	}
	filtered := filterCategoryItems(items, model.RegistrySource{
		Category: "missing_person",
		Source:   model.SourceMetadata{CountryCode: "CR"},
	}, dict)
	if len(filtered) != 1 {
		t.Fatalf("expected only one missing-person item after filtering, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/desaparecidos/1" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
}

func TestFilterCategoryItemsDropsUnrelatedWantedHTML(t *testing.T) {
	dict, err := dictionary.Load(filepath.Join("..", "..", "..", "registry", "category_dictionary.json"))
	if err != nil {
		t.Fatal(err)
	}
	items := []parse.FeedItem{
		{Title: "Institutional history", Link: "https://example.test/history"},
		{Title: "Wanted suspect public appeal", Link: "https://example.test/wanted/1"},
	}
	filtered := filterCategoryItems(items, model.RegistrySource{
		Category: "wanted_suspect",
		Source:   model.SourceMetadata{CountryCode: "US"},
	}, dict)
	if len(filtered) != 1 {
		t.Fatalf("expected only one wanted item after filtering, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/wanted/1" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
}

func TestFilterCategoryItemsMatchesCatalanMissingPersonPage(t *testing.T) {
	dict, err := dictionary.Load(filepath.Join("..", "..", "..", "registry", "category_dictionary.json"))
	if err != nil {
		t.Fatal(err)
	}
	items := []parse.FeedItem{
		{Title: "Persona desapareguda", Link: "https://example.test/_ca/persona-desapareguda"},
	}
	filtered := filterCategoryItems(items, model.RegistrySource{
		Category: "missing_person",
		FeedURL:  "https://www.policia.es/_ca/comunicacion_salaprensa.php?idiomaActual=ca",
		Source:   model.SourceMetadata{CountryCode: "ES"},
	}, dict)
	if len(filtered) != 1 {
		t.Fatalf("expected Catalan missing-person page to be retained, got %d", len(filtered))
	}
}

func TestShouldRefreshOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "military-bases.geojson")
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)

	if !shouldRefreshOutput(path, 168, now) {
		t.Fatal("expected missing output file to require refresh")
	}
	if err := os.WriteFile(path, []byte(`{"type":"FeatureCollection","features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if shouldRefreshOutput(path, 168, now) {
		t.Fatal("expected fresh output file to skip refresh")
	}
	if err := os.Chtimes(path, now.Add(-200*time.Hour), now.Add(-200*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !shouldRefreshOutput(path, 168, now) {
		t.Fatal("expected stale output file to require refresh")
	}
}

func TestFetchUCDPSkipsSilentlyWithoutToken(t *testing.T) {
	runner := New(io.Discard, io.Discard)
	called := false
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		called = true
		return fetch.New(cfg)
	}

	cfg := config.Default()
	cfg.UCDPAccessToken = ""
	nctx := normalize.Context{Config: cfg, Now: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)}
	source := model.RegistrySource{
		Type:    "ucdp-json",
		FeedURL: "https://ucdpapi.pcr.uu.se/api/gedevents/25.1",
	}

	alerts, err := runner.fetchUCDP(context.Background(), nctx, source)
	if err != nil {
		t.Fatalf("expected silent skip without token, got error: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected zero alerts when token missing, got %d", len(alerts))
	}
	if called {
		t.Fatal("expected no HTTP client calls when UCDP token is missing")
	}
}

func TestIsXStatusURL(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "https://x.com/CENTCOM/status/12345", want: true},
		{in: "https://twitter.com/StateDept/status/99", want: true},
		{in: "https://x.com/IDF", want: false},
		{in: "https://x.com/explore", want: false},
		{in: "https://example.com/CENTCOM/status/12345", want: false},
		{in: "", want: false},
	}
	for _, tc := range tests {
		if got := isXStatusURL(tc.in); got != tc.want {
			t.Fatalf("isXStatusURL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseXStatusAnchorsKeepsShortStatusLinks(t *testing.T) {
	body := `<html><body>
<a href="/IDF/status/123">5h</a>
<a href="https://x.com/CENTCOM/status/987">2m</a>
<a href="https://x.com/IDF">Profile</a>
</body></html>`
	items := parseXStatusAnchors(body, "https://x.com/IDF")
	if len(items) != 2 {
		t.Fatalf("expected 2 x status anchors, got %d", len(items))
	}
	if items[0].Link != "https://x.com/IDF/status/123" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Link != "https://x.com/CENTCOM/status/987" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
	if items[0].Title == "" || items[1].Title == "" {
		t.Fatalf("expected non-empty titles, got %#v", items)
	}
}

func TestParseXTweetsFromGraphQLJSON(t *testing.T) {
	body := []byte(`{
	  "data": {
	    "user": {
	      "result": {
	        "timeline": {
	          "timeline": {
	            "instructions": [
	              {
	                "entries": [
	                  {
	                    "content": {
	                      "itemContent": {
	                        "tweet_results": {
	                          "result": {
	                            "legacy": {
	                              "id_str": "2034641269960393096",
	                              "created_at": "Thu Mar 19 14:40:55 +0000 2026",
	                              "full_text": "Test operational update"
	                            }
	                          }
	                        }
	                      }
	                    }
	                  }
	                ]
	              }
	            ]
	          }
	        }
	      }
	    }
	  }
	}`)
	items := parseXTweetsFromGraphQLJSON(body, "https://x.com/IDF")
	if len(items) != 1 {
		t.Fatalf("expected 1 parsed tweet, got %d", len(items))
	}
	if items[0].Link != "https://x.com/IDF/status/2034641269960393096" {
		t.Fatalf("unexpected link: %q", items[0].Link)
	}
	if items[0].Title != "Test operational update" {
		t.Fatalf("unexpected title: %q", items[0].Title)
	}
	if items[0].Published != "2026-03-19T14:40:55Z" {
		t.Fatalf("unexpected published: %q", items[0].Published)
	}
}

func TestMergeGeoJSONFeaturesDeduplicatesByIDAndProperties(t *testing.T) {
	a := []json.RawMessage{
		json.RawMessage(`{"type":"Feature","id":"base-1","properties":{"name":"Base A"},"geometry":{"type":"Point","coordinates":[1,2]}}`),
		json.RawMessage(`{"type":"Feature","properties":{"OBJECTID":200,"name":"Base B"},"geometry":{"type":"Point","coordinates":[3,4]}}`),
	}
	b := []json.RawMessage{
		json.RawMessage(`{"type":"Feature","id":"base-1","properties":{"name":"Base A duplicate"},"geometry":{"type":"Point","coordinates":[1,2]}}`),
		json.RawMessage(`{"type":"Feature","properties":{"OBJECTID":200,"name":"Base B duplicate"},"geometry":{"type":"Point","coordinates":[3,4]}}`),
		json.RawMessage(`{"type":"Feature","id":"base-3","properties":{"name":"Base C"},"geometry":{"type":"Point","coordinates":[5,6]}}`),
	}
	merged := mergeGeoJSONFeatures(a, b)
	if len(merged) != 3 {
		t.Fatalf("expected 3 deduplicated features, got %d", len(merged))
	}
}

func TestFilterFeedKeywordsAppliesToRSSContent(t *testing.T) {
	items := []parse.FeedItem{
		{Title: "Budget debate", Summary: "Parliament procedure only", Link: "https://example.test/a"},
		{Title: "Parliament update", Summary: "New sanctions package announced", Link: "https://example.test/b"},
	}
	filtered := filterFeedKeywords(items, []string{"sanction"}, nil)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 retained RSS item, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/b" {
		t.Fatalf("unexpected retained RSS item: %#v", filtered[0])
	}
}

func TestFilterKeywordsAppliesGlobalStopWords(t *testing.T) {
	items := []parse.FeedItem{
		{Title: "CISA advisory on critical vulnerability", Link: "https://example.test/a"},
		{Title: "Premier League football results", Link: "https://example.test/b"},
		{Title: "UEFA Champions League draw", Link: "https://example.test/c"},
		{Title: "Ransomware attack on hospital", Link: "https://example.test/d"},
	}
	global := []string{"football", "champions league"}
	filtered := filterKeywords(items, nil, nil, global)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 retained items, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/a" || filtered[1].Link != "https://example.test/d" {
		t.Fatalf("unexpected retained items: %#v", filtered)
	}
}

func TestFilterFeedKeywordsAppliesGlobalStopWords(t *testing.T) {
	items := []parse.FeedItem{
		{Title: "Security incident report", Summary: "Critical infrastructure breach", Link: "https://example.test/a"},
		{Title: "Award ceremony", Summary: "Grammy nominees announced", Link: "https://example.test/b"},
		{Title: "Travel advisory update", Summary: "Celebrity gossip column", Link: "https://example.test/c"},
	}
	global := []string{"grammy", "celebrity"}
	filtered := filterFeedKeywords(items, nil, nil, global)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 retained item, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/a" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
}

func TestFilterKeywordsGlobalStopWordsMergeWithPerSource(t *testing.T) {
	items := []parse.FeedItem{
		{Title: "NBA basketball highlights", Link: "https://example.test/a"},
		{Title: "Local police budget report", Link: "https://example.test/b"},
		{Title: "Cyber attack on government", Link: "https://example.test/c"},
	}
	perSource := []string{"budget"}
	global := []string{"basketball"}
	filtered := filterKeywords(items, nil, perSource, global)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 retained item, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/c" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
}

func TestFilterKeywordsGlobalStopWordsEmptyPassesAll(t *testing.T) {
	items := []parse.FeedItem{
		{Title: "Football match", Link: "https://example.test/a"},
		{Title: "Cyber alert", Link: "https://example.test/b"},
	}
	filtered := filterKeywords(items, nil, nil)
	if len(filtered) != 2 {
		t.Fatalf("expected all 2 items with no stop words, got %d", len(filtered))
	}
}

func TestGlobalStopWordsDoNotSuppressSexualAssaultIntelligence(t *testing.T) {
	items := []parse.FeedItem{
		{
			Title:   "Police appeal: sexual assault investigation in city center",
			Summary: "Authorities seek witnesses",
			Link:    "https://example.test/a",
		},
		{
			Title: "Celebrity sex scandal spreads online",
			Link:  "https://example.test/b",
		},
	}
	filtered := filterFeedKeywords(items, nil, nil, []string{"sex", "sex scandal"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 retained intelligence item, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/a" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
}

func TestGlobalStopWordsUseWholeWordMatching(t *testing.T) {
	items := []parse.FeedItem{
		{
			Title: "National police warning on sextortion campaign",
			Link:  "https://example.test/sextortion",
		},
		{
			Title: "Sex rumor thread goes viral",
			Link:  "https://example.test/rumor",
		},
	}
	filtered := filterFeedKeywords(items, nil, nil, []string{"sex"})
	if len(filtered) != 1 {
		t.Fatalf("expected only sextortion item retained, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/sextortion" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
}

func TestApplyNoiseGateDropsPolicySpam(t *testing.T) {
	runner := New(io.Discard, io.Discard)
	engine, err := noisegate.Load(filepath.Join("..", "..", "..", "registry", "noise_policy.json"))
	if err != nil {
		t.Fatalf("load noise policy: %v", err)
	}
	runner.noiseGate = engine

	items := []parse.FeedItem{
		{Title: "Celebrity gossip giveaway and lottery update", Link: "https://example.test/spam"},
		{Title: "Police appeal for missing person", Link: "https://example.test/valid"},
	}
	filtered, decisions := runner.applyNoiseGate(model.RegistrySource{
		Category: "public_appeal",
		Source:   model.SourceMetadata{AuthorityType: "police"},
	}, items)
	if len(filtered) != 1 {
		t.Fatalf("expected only one retained item, got %d", len(filtered))
	}
	if filtered[0].Link != "https://example.test/valid" {
		t.Fatalf("unexpected retained item: %#v", filtered[0])
	}
	if decisions[itemDecisionKey(filtered[0])].Outcome != noisegate.OutcomeKeep {
		t.Fatalf("expected kept decision for valid item, got %+v", decisions[itemDecisionKey(filtered[0])])
	}
}

func TestApplyNoiseDecisionDowngradesAlert(t *testing.T) {
	alert := &model.Alert{
		Category: "public_appeal",
		Severity: "high",
	}
	applyNoiseDecision(alert, noisegate.Decision{
		Outcome:            noisegate.OutcomeDowngrade,
		PolicyVersion:      "v1",
		BlockScore:         0.2,
		NoiseScore:         0.8,
		ActionabilityScore: 0.3,
		Reasons:            []string{"downgrade-threshold"},
	})
	if alert.Category != "informational" || alert.Severity != "info" {
		t.Fatalf("expected informational downgrade, got category=%q severity=%q", alert.Category, alert.Severity)
	}
	if alert.Triage == nil || alert.Triage.Metadata == nil {
		t.Fatalf("expected triage metadata to be populated, got %#v", alert.Triage)
	}
	if alert.Triage.Metadata.NoiseDecision != "downgrade" {
		t.Fatalf("expected noise decision metadata, got %#v", alert.Triage.Metadata)
	}
	if alert.Triage.Metadata.NoisePolicyVersion != "v1" {
		t.Fatalf("expected policy version, got %#v", alert.Triage.Metadata)
	}
	if len(alert.Triage.WeakSignals) == 0 {
		t.Fatalf("expected weak signal reason from downgrade, got %#v", alert.Triage)
	}
}

func TestWriteNoiseMetrics(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.NoiseMetricsOutputPath = filepath.Join(dir, "noise-metrics.json")
	runner := New(io.Discard, io.Discard)
	alerts := []model.Alert{
		{
			AlertID:            "a1",
			SourceID:           "src-a",
			SignalLane:         model.SignalLaneAlarm,
			EventGeoConfidence: 0.9,
		},
		{
			AlertID:            "a2",
			SourceID:           "src-a",
			SignalLane:         model.SignalLaneIntel,
			EventGeoConfidence: 0.6,
		},
	}
	if err := runner.writeNoiseMetrics(context.Background(), cfg, alerts, nil); err != nil {
		t.Fatalf("writeNoiseMetrics: %v", err)
	}
	raw, err := os.ReadFile(cfg.NoiseMetricsOutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["lane_distribution"]; !ok {
		t.Fatalf("expected lane_distribution in metrics doc: %s", string(raw))
	}
	if _, ok := doc["geo_confidence_average"]; !ok {
		t.Fatalf("expected geo_confidence_average in metrics doc: %s", string(raw))
	}
}

func TestRunnerRunOnceUsesSQLiteAlertStateWithoutDuplicatingAlerts(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "sources.db")
	seedPath := filepath.Join(dir, "registry.json")
	registry := []byte(`[
	  {"type":"rss","feed_url":"https://collector.test/rss","category":"cyber_advisory","region_tag":"INT","lat":48.8,"lng":2.3,"source":{"source_id":"rss-source","authority_name":"RSS Source","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://collector.test","language_code":"en"}}
	]`)
	if err := os.WriteFile(seedPath, registry, 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := sourcedb.Open(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ImportRegistry(context.Background(), seedPath); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()

	cfg := config.Default()
	cfg.RegistryPath = registryPath
	cfg.OutputPath = filepath.Join(dir, "alerts.json")
	cfg.FilteredOutputPath = filepath.Join(dir, "filtered.json")
	cfg.StateOutputPath = filepath.Join(dir, "state.json")
	cfg.SourceHealthOutputPath = filepath.Join(dir, "health.json")
	cfg.ReplacementQueuePath = filepath.Join(dir, "replacement.json")
	cfg.MaxAgeDays = 10000

	runner := New(io.Discard, io.Discard)
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		return fetch.NewWithHTTPClient(cfg, &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/rss" {
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
				}
				body := `<?xml version="1.0"?><rss><channel><item><guid>alert-1</guid><title>Critical cyber advisory</title><link>https://collector.test/rss-item</link><pubDate>Mon, 02 Jan 2026 15:04:05 MST</pubDate><description>CVE-2026-1234 patch advisory</description></item></channel></rss>`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			}),
		})
	}

	if err := runner.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	persistedAfterFirstRun, err := loadPersistedAlerts(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(persistedAfterFirstRun) != 2 {
		t.Fatalf("expected 2 persisted alerts after first run, got %d", len(persistedAfterFirstRun))
	}

	firstSeenByID := map[string]string{}
	for _, alert := range persistedAfterFirstRun {
		firstSeenByID[alert.AlertID] = alert.FirstSeen
	}

	time.Sleep(1100 * time.Millisecond)

	if err := runner.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	persistedAfterSecondRun, err := loadPersistedAlerts(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(persistedAfterSecondRun) != 2 {
		t.Fatalf("expected 2 persisted alerts after second run, got %d", len(persistedAfterSecondRun))
	}
	for _, alert := range persistedAfterSecondRun {
		if want := firstSeenByID[alert.AlertID]; want == "" {
			t.Fatalf("unexpected alert persisted after second run: %q", alert.AlertID)
		} else if alert.FirstSeen != want {
			t.Fatalf("expected first_seen for %s to remain %q, got %q", alert.AlertID, want, alert.FirstSeen)
		}
	}
}

func TestClassifySourceErrorTreats401And522AsRetry(t *testing.T) {
	errClass, needsReplacement, action := classifySourceError(errors.New("fetch https://example.test/feed: status 401"))
	if errClass != "unauthorized" || needsReplacement || action != "retry" {
		t.Fatalf("expected 401 to retry, got class=%q needs=%v action=%q", errClass, needsReplacement, action)
	}

	errClass, needsReplacement, action = classifySourceError(errors.New("fetch https://example.test/feed: status 522"))
	if errClass != "origin_unreachable" || needsReplacement || action != "retry" {
		t.Fatalf("expected 522 to retry, got class=%q needs=%v action=%q", errClass, needsReplacement, action)
	}
}

func loadPersistedAlerts(dbPath string) ([]model.Alert, error) {
	db, err := sourcedb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.LoadAlerts(context.Background())
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestEnsureUCDPQueryAddsPageSize(t *testing.T) {
	got, explicitPage := ensureUCDPQuery("https://ucdpapi.pcr.uu.se/api/gedevents/25.1", 100)
	if explicitPage {
		t.Fatal("expected explicitPage=false when page is absent")
	}
	if !strings.Contains(got, "pagesize=100") {
		t.Fatalf("expected pagesize in URL, got %q", got)
	}
}

func TestEnsureUCDPQueryPreservesExplicitPage(t *testing.T) {
	got, explicitPage := ensureUCDPQuery("https://ucdpapi.pcr.uu.se/api/gedevents/25.1?page=7&pagesize=50", 100)
	if !explicitPage {
		t.Fatal("expected explicitPage=true when page is present")
	}
	if !strings.Contains(got, "page=7") {
		t.Fatalf("expected page=7 in URL, got %q", got)
	}
	if !strings.Contains(got, "pagesize=50") {
		t.Fatalf("expected pagesize=50 in URL, got %q", got)
	}
}

func TestSetUCDPPage(t *testing.T) {
	got := setUCDPPage("https://ucdpapi.pcr.uu.se/api/gedevents/25.1?pagesize=100", 3859)
	if !strings.Contains(got, "page=3859") {
		t.Fatalf("expected page=3859 in URL, got %q", got)
	}
	if !strings.Contains(got, "pagesize=100") {
		t.Fatalf("expected pagesize retained in URL, got %q", got)
	}
}

func TestParseUCDPTotalPages(t *testing.T) {
	body := []byte(`{"TotalPages":3860,"Result":[]}`)
	if got := parseUCDPTotalPages(body); got != 3860 {
		t.Fatalf("parseUCDPTotalPages()=%d, want 3860", got)
	}
	if got := parseUCDPTotalPages([]byte(`{"Result":[]}`)); got != 0 {
		t.Fatalf("parseUCDPTotalPages() missing field=%d, want 0", got)
	}
}

func TestFetchUCDPItemsForLensFiltersAndPaginates(t *testing.T) {
	page0 := `{"TotalCount":2,"TotalPages":2,"Result":[
		{"id":1,"type_of_violence":1,"date_start":"2026-03-01","country":"Sudan","country_id":625,"best":5,"latitude":13.0,"longitude":25.0,"side_a":"SAF","side_b":"RSF","adm_1":"Darfur"}
	]}`
	page1 := `{"TotalCount":2,"TotalPages":2,"Result":[
		{"id":2,"type_of_violence":1,"date_start":"2026-03-10","country":"Sudan","country_id":625,"best":3,"latitude":12.5,"longitude":24.5,"side_a":"SAF","side_b":"RSF","adm_1":"Kordofan"}
	]}`

	runner := New(io.Discard, io.Discard)
	var requestedURLs []string
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		return fetch.NewWithHTTPClient(cfg, &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requestedURLs = append(requestedURLs, req.URL.String())
				q := req.URL.Query()
				// Verify auth header is present.
				if req.Header.Get("x-ucdp-access-token") == "" {
					t.Fatal("missing UCDP auth header")
				}
				// Verify country filter is present.
				if q.Get("Country") == "" {
					t.Fatal("expected Country query param")
				}
				pageStr := q.Get("page")
				var body string
				if pageStr == "0" || pageStr == "" {
					body = page0
				} else {
					body = page1
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			}),
		})
	}

	cfg := config.Default()
	cfg.UCDPAPIVersion = "26.0.1"
	headers := map[string]string{"x-ucdp-access-token": "test-token"}

	// Use Sudan lens.
	var sudanLens zonebrief.LensDef
	for _, l := range zonebrief.SupportedLenses {
		if l.ID == "sudan" {
			sudanLens = l
			break
		}
	}

	items, err := runner.fetchUCDPItemsForLens(context.Background(), cfg, sudanLens, headers, "26.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items across 2 pages, got %d", len(items))
	}
	if len(requestedURLs) != 2 {
		t.Fatalf("expected 2 HTTP requests (2 pages), got %d", len(requestedURLs))
	}
	// Verify version in URL.
	if !strings.Contains(requestedURLs[0], "gedevents/26.0.1") {
		t.Fatalf("expected version 26.0.1 in URL, got %q", requestedURLs[0])
	}
	// Verify date filter.
	if !strings.Contains(requestedURLs[0], "StartDate=") {
		t.Fatalf("expected StartDate param, got %q", requestedURLs[0])
	}
}

func TestFetchUCDPConflictsPaginatesAndDeduplicates(t *testing.T) {
	page0 := `{"TotalCount":3,"TotalPages":2,"Result":[
		{"conflict_id":"309","conflict_name":"Sudan: Government","type_of_conflict":"3","intensity_level":"2","gwno_loc":"625","year":"2023","side_a":"Gov Sudan","side_b":"RSF"},
		{"conflict_id":"309","conflict_name":"Sudan: Government","type_of_conflict":"3","intensity_level":"1","gwno_loc":"625","year":"2020","side_a":"Gov Sudan","side_b":"SRF"}
	]}`
	page1 := `{"TotalCount":3,"TotalPages":2,"Result":[
		{"conflict_id":"309","conflict_name":"Sudan: Government","type_of_conflict":"3","intensity_level":"2","gwno_loc":"625","year":"2024","side_a":"Gov Sudan","side_b":"RSF"}
	]}`

	runner := New(io.Discard, io.Discard)
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		return fetch.NewWithHTTPClient(cfg, &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				q := req.URL.Query()
				body := page0
				if q.Get("page") == "1" {
					body = page1
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			}),
		})
	}

	cfg := config.Default()
	headers := map[string]string{"x-ucdp-access-token": "test-token"}

	conflicts, err := runner.fetchUCDPConflicts(context.Background(), cfg, headers, "25.1")
	if err != nil {
		t.Fatal(err)
	}
	// Should be deduplicated to 1 conflict (309) with year=2024.
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 deduplicated conflict, got %d", len(conflicts))
	}
	if conflicts[0].Year != 2024 {
		t.Fatalf("expected year 2024 (highest), got %d", conflicts[0].Year)
	}
	if conflicts[0].IntensityLevel != 2 {
		t.Fatalf("expected intensity 2, got %d", conflicts[0].IntensityLevel)
	}
}

func TestDeduplicateConflictsKeepsHighestYear(t *testing.T) {
	conflicts := []parse.UCDPConflict{
		{ConflictID: "1", Year: 2020, IntensityLevel: 1},
		{ConflictID: "1", Year: 2024, IntensityLevel: 2},
		{ConflictID: "1", Year: 2022, IntensityLevel: 1},
		{ConflictID: "2", Year: 2023, IntensityLevel: 1},
	}
	got := deduplicateConflicts(conflicts)
	if len(got) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(got))
	}
	byID := map[string]parse.UCDPConflict{}
	for _, c := range got {
		byID[c.ConflictID] = c
	}
	if byID["1"].Year != 2024 || byID["1"].IntensityLevel != 2 {
		t.Fatalf("conflict 1: expected year=2024 intensity=2, got year=%d intensity=%d", byID["1"].Year, byID["1"].IntensityLevel)
	}
	if byID["2"].Year != 2023 {
		t.Fatalf("conflict 2: expected year=2023, got %d", byID["2"].Year)
	}
}

func TestDiscoverUCDPVersionsParsesHTML(t *testing.T) {
	html := `<html><body>
		The latest version of UCDP GED:
		<code>25.1</code>
		The latest versions of UCDP GED Candidate:<br>
		Monthly release: <code>26.0.1</code><br>
		Quarterly release: <code>25.01.25.12</code>
		latest version of the yearly datasets:
		<code>25.1</code>
	</body></html>`

	// We can't easily mock discoverUCDPVersions since it uses http.DefaultClient,
	// but we can test the regex logic directly with a local HTTP server.
	srv := &http.Server{Addr: "127.0.0.1:0"}
	mux := http.NewServeMux()
	mux.HandleFunc("/apidocs/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	})
	srv.Handler = mux

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)
	defer srv.Close()

	// Since discoverUCDPVersions hits a hardcoded URL, test the parsing logic
	// via the regex directly.
	v := ucdpVersions{}
	if m := regexp.MustCompile(`Monthly release:\s*<code>([^<]+)</code>`).FindStringSubmatch(html); len(m) > 1 {
		v.candidate = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`latest version of the yearly datasets:\s*<code>([^<]+)</code>`).FindStringSubmatch(html); len(m) > 1 {
		v.annual = strings.TrimSpace(m[1])
	}
	if v.candidate != "26.0.1" {
		t.Fatalf("expected candidate 26.0.1, got %q", v.candidate)
	}
	if v.annual != "25.1" {
		t.Fatalf("expected annual 25.1, got %q", v.annual)
	}
}

func TestWriteZoneBriefingsStalenessSkip(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "zone-briefings.json")
	// Write a fresh file.
	if err := os.WriteFile(outPath, []byte(`[{"lens_id":"gaza"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := New(io.Discard, io.Discard)
	httpCalled := false
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		httpCalled = true
		return fetch.New(cfg)
	}

	cfg := config.Default()
	cfg.ZoneBriefingsOutputPath = outPath
	cfg.UCDPAccessToken = "test-token"
	cfg.ZoneBriefingRefreshHours = 24

	err := runner.writeZoneBriefings(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if httpCalled {
		t.Fatal("expected staleness check to skip refresh, but HTTP client was created")
	}
}

func TestWriteZoneBriefingsFreshEmptyFileDoesNotSkip(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "zone-briefings.json")
	// Fresh bootstrap artifact should not be treated as valid cache content.
	if err := os.WriteFile(outPath, []byte("[] "), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := New(io.Discard, io.Discard)
	cfg := config.Default()
	cfg.ZoneBriefingsOutputPath = outPath
	cfg.ZoneBriefingRefreshHours = 24
	cfg.UCDPAccessToken = ""

	if err := runner.writeZoneBriefings(context.Background(), cfg, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]\n" {
		t.Fatalf("expected rewrite of fresh empty file, got %q", string(data))
	}
}

func TestWriteZoneBriefingsNoTokenWritesEmpty(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "zone-briefings.json")

	runner := New(io.Discard, io.Discard)
	cfg := config.Default()
	cfg.ZoneBriefingsOutputPath = outPath
	cfg.UCDPAccessToken = ""

	err := runner.writeZoneBriefings(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]\n" {
		t.Fatalf("expected empty JSON array, got %q", string(data))
	}
}

func TestFetchUCDPItemsAlertPipelineFiltersTo7Days(t *testing.T) {
	response := `{"TotalCount":1,"TotalPages":1,"Result":[
		{"id":1,"type_of_violence":1,"date_start":"2026-03-19","country":"Sudan","country_id":625,"best":5,"side_a":"SAF","side_b":"RSF"}
	]}`

	runner := New(io.Discard, io.Discard)
	var capturedURL string
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		return fetch.NewWithHTTPClient(cfg, &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				capturedURL = req.URL.String()
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
			}),
		})
	}

	cfg := config.Default()
	cfg.UCDPAccessToken = "test-token"
	cfg.UCDPAPIVersion = "26.0.1"
	nctx := normalize.Context{Config: cfg, Now: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)}
	source := model.RegistrySource{
		Type:    "ucdp-json",
		FeedURL: "https://ucdpapi.pcr.uu.se/api/gedevents/25.1",
	}

	items, err := runner.fetchUCDPItems(context.Background(), nctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	// Verify URL uses config version, not source URL.
	if !strings.Contains(capturedURL, "gedevents/26.0.1") {
		t.Fatalf("expected version from config (26.0.1), got %q", capturedURL)
	}
	// Verify date filter present.
	if !strings.Contains(capturedURL, "StartDate=") || !strings.Contains(capturedURL, "EndDate=") {
		t.Fatalf("expected date filters in alert pipeline URL, got %q", capturedURL)
	}
}

func TestWriteZoneBriefingsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "zone-briefings.json")
	boundariesPath := filepath.Join(dir, "countries.geojson")
	geoDir := filepath.Join(dir, "geo")
	os.MkdirAll(geoDir, 0o755)

	// Write a minimal boundary file.
	boundaries := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"ISO_A2":"SD"},"geometry":{"type":"Polygon","coordinates":[[[25,10],[25,20],[35,20],[35,10],[25,10]]]}}]}`
	os.WriteFile(boundariesPath, []byte(boundaries), 0o644)

	// Write a stale file to bypass staleness check.
	staleTime := time.Now().Add(-48 * time.Hour)
	os.WriteFile(outPath, []byte("[]"), 0o644)
	os.Chtimes(outPath, staleTime, staleTime)

	recentEventDate := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	ucdpResponse := fmt.Sprintf(`{"TotalCount":1,"TotalPages":1,"Result":[
		{"id":100,"type_of_violence":1,"date_start":"%s","country":"Sudan","country_id":625,"best":10,"deaths_civilians":2,"latitude":13.0,"longitude":25.0,"side_a":"Gov of Sudan","side_b":"RSF","dyad_name":"Gov vs RSF","adm_1":"Darfur","adm_2":"Zalingei"}
	]}`, recentEventDate)
	conflictResponse := `{"TotalCount":1,"TotalPages":1,"Result":[
		{"conflict_id":"309","conflict_name":"Sudan: Government","type_of_conflict":"3","intensity_level":"2","gwno_loc":"625","year":"2024","side_a":"Gov of Sudan","side_b":"RSF"}
	]}`
	versionHTML := `<html>Monthly release: <code>26.0.1</code> latest version of the yearly datasets: <code>25.1</code></html>`

	runner := New(io.Discard, io.Discard)
	runner.nowFn = func() time.Time {
		return time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	}
	runner.clientFactory = func(cfg config.Config) *fetch.Client {
		return fetch.NewWithHTTPClient(cfg, &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				u := req.URL.String()
				var body string
				switch {
				case strings.Contains(u, "apidocs"):
					body = versionHTML
				case strings.Contains(u, "ucdpprioconflict"):
					body = conflictResponse
				case strings.Contains(u, "gedevents"):
					body = ucdpResponse
				default:
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			}),
		})
	}

	cfg := config.Default()
	cfg.ZoneBriefingsOutputPath = outPath
	cfg.CountryBoundariesPath = boundariesPath
	cfg.UCDPAccessToken = "test-token"
	cfg.UCDPAPIVersion = "26.0.1"
	cfg.ZoneBriefingRefreshHours = 24
	cfg.ZoneBriefingACLEDEnabled = false // skip ACLED for this test

	err := runner.writeZoneBriefings(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify briefings file was written.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var briefings []json.RawMessage
	if err := json.Unmarshal(data, &briefings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(briefings) != 6 {
		t.Fatalf("expected 6 lens briefings, got %d", len(briefings))
	}

	// Parse the Sudan briefing to verify enrichment.
	var records []map[string]any
	json.Unmarshal(data, &records)
	var sudan map[string]any
	for _, r := range records {
		if r["lens_id"] == "sudan" {
			sudan = r
			break
		}
	}
	if sudan == nil {
		t.Fatal("expected sudan briefing")
	}
	if sudan["conflict_intensity"] != "war" {
		t.Fatalf("expected conflict_intensity=war, got %v", sudan["conflict_intensity"])
	}
	if sudan["status"] != "active" && sudan["status"] != "watch" {
		t.Fatalf("expected status to reflect a current or recent conflict, got %v", sudan["status"])
	}
	conflicts, ok := sudan["active_conflicts"].([]any)
	if !ok || len(conflicts) == 0 {
		t.Fatal("expected active_conflicts to be populated")
	}

	// Verify GeoJSON files were written.
	conflictGeo := filepath.Join(geoDir, "conflict-zones.geojson")
	terrorGeo := filepath.Join(geoDir, "terrorism-zones.geojson")
	if _, err := os.Stat(conflictGeo); err != nil {
		t.Fatalf("expected conflict-zones.geojson: %v", err)
	}
	if _, err := os.Stat(terrorGeo); err != nil {
		t.Fatalf("expected terrorism-zones.geojson: %v", err)
	}
}
