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

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
	"github.com/scalytics/euosint/internal/collector/model"
)

func TestRunnerRunOnceWritesOutputs(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	registry := []byte(`[
	  {"type":"rss","feed_url":"https://collector.test/rss","category":"cyber_advisory","region_tag":"INT","lat":48.8,"lng":2.3,"source":{"source_id":"rss-source","authority_name":"RSS Source","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://collector.test"}},
	  {"type":"html-list","feed_url":"https://collector.test/html","category":"wanted_suspect","region_tag":"FR","lat":48.8,"lng":2.3,"include_keywords":["wanted"],"source":{"source_id":"html-source","authority_name":"HTML Source","country":"France","country_code":"FR","region":"Europe","authority_type":"police","base_url":"https://collector.test"}},
	  {"type":"kev-json","feed_url":"https://collector.test/kev","category":"cyber_advisory","region_tag":"US","lat":38.8,"lng":-77.0,"source":{"source_id":"kev-source","authority_name":"KEV Source","country":"United States","country_code":"US","region":"North America","authority_type":"cert","base_url":"https://www.cisa.gov"}},
	  {"type":"interpol-red-json","feed_url":"https://collector.test/interpol","category":"wanted_suspect","region_tag":"INT","lat":45.7,"lng":4.8,"source":{"source_id":"interpol-red","authority_name":"Interpol Red","country":"France","country_code":"FR","region":"International","authority_type":"police","base_url":"https://www.interpol.int"}}
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
	if health.TotalSources != 4 {
		t.Fatalf("expected 4 sources in health document, got %d", health.TotalSources)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
