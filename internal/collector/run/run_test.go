// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/dictionary"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/sourcedb"
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
