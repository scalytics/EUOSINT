// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/trends"
	"github.com/scalytics/euosint/internal/sourcedb"
)

func testDB(t *testing.T) *sourcedb.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sourcedb.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedAlerts(t *testing.T, db *sourcedb.DB) {
	t.Helper()
	alerts := []model.Alert{
		{
			AlertID:      "a1",
			SourceID:     "europol",
			Status:       "active",
			Title:        "Europol dismantles major drug trafficking network",
			CanonicalURL: "https://europol.europa.eu/a1",
			Category:     "public_appeal",
			Severity:     "high",
			RegionTag:    "EU",
			Source: model.SourceMetadata{
				SourceID:      "europol",
				AuthorityName: "Europol",
				Country:       "Netherlands",
				CountryCode:   "NL",
				Region:        "Europe",
			},
		},
		{
			AlertID:      "a2",
			SourceID:     "fbi-wanted",
			Status:       "active",
			Title:        "FBI Most Wanted: Cyber fugitive identified",
			CanonicalURL: "https://fbi.gov/a2",
			Category:     "wanted_suspect",
			Severity:     "critical",
			RegionTag:    "US",
			Source: model.SourceMetadata{
				SourceID:      "fbi-wanted",
				AuthorityName: "FBI",
				Country:       "United States",
				CountryCode:   "US",
				Region:        "North America",
			},
		},
		{
			AlertID:      "a3",
			SourceID:     "cert-ua",
			Status:       "active",
			Title:        "CERT-UA reports new malware campaign targeting energy sector",
			CanonicalURL: "https://cert.gov.ua/a3",
			Category:     "cyber_advisory",
			Severity:     "high",
			RegionTag:    "UA",
			Source: model.SourceMetadata{
				SourceID:      "cert-ua",
				AuthorityName: "CERT-UA",
				Country:       "Ukraine",
				CountryCode:   "UA",
				Region:        "Europe",
			},
		},
	}
	if err := db.SaveAlerts(context.Background(), alerts); err != nil {
		t.Fatal(err)
	}
}

func TestSearchReturnsRankedResults(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	seedAlerts(t, db)

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	req := httptest.NewRequest("GET", "/api/search?q=drug+trafficking", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count   int           `json:"count"`
		Results []model.Alert `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected search results for 'drug trafficking'")
	}
	if resp.Results[0].AlertID != "a1" {
		t.Fatalf("expected Europol alert first, got %s", resp.Results[0].AlertID)
	}
}

func TestSearchWithCategoryFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	seedAlerts(t, db)

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	req := httptest.NewRequest("GET", "/api/search?category=cyber_advisory", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count   int           `json:"count"`
		Results []model.Alert `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if r.Category != "cyber_advisory" {
			t.Fatalf("expected only cyber_advisory, got %s", r.Category)
		}
	}
}

func TestSearchEmptyQueryReturns400(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSearchDefaultsToActiveStatus(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	alerts := []model.Alert{
		{
			AlertID:      "active-1",
			SourceID:     "src",
			Status:       "active",
			Title:        "Active cyber alert",
			CanonicalURL: "https://example.test/active",
			Category:     "cyber_advisory",
			Severity:     "high",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				Country:       "Germany",
				CountryCode:   "DE",
			},
		},
		{
			AlertID:      "filtered-1",
			SourceID:     "src",
			Status:       "filtered",
			Title:        "Filtered cyber alert",
			CanonicalURL: "https://example.test/filtered",
			Category:     "cyber_advisory",
			Severity:     "low",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				Country:       "Germany",
				CountryCode:   "DE",
			},
		},
		{
			AlertID:      "removed-1",
			SourceID:     "src",
			Status:       "removed",
			Title:        "Removed cyber alert",
			CanonicalURL: "https://example.test/removed",
			Category:     "cyber_advisory",
			Severity:     "medium",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				Country:       "Germany",
				CountryCode:   "DE",
			},
		},
	}
	if err := db.SaveAlerts(context.Background(), alerts); err != nil {
		t.Fatal(err)
	}

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler
	req := httptest.NewRequest("GET", "/api/search?category=cyber_advisory", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []model.Alert `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].AlertID != "active-1" {
		t.Fatalf("expected only active alert by default, got %#v", resp.Results)
	}
}

func TestSearchIncludeFilteredAndRemovedOptIn(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	alerts := []model.Alert{
		{
			AlertID:      "active-1",
			SourceID:     "src",
			Status:       "active",
			Title:        "Active alert",
			CanonicalURL: "https://example.test/active",
			Category:     "cyber_advisory",
			Severity:     "high",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				CountryCode:   "DE",
			},
		},
		{
			AlertID:      "filtered-1",
			SourceID:     "src",
			Status:       "filtered",
			Title:        "Filtered alert",
			CanonicalURL: "https://example.test/filtered",
			Category:     "cyber_advisory",
			Severity:     "low",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				CountryCode:   "DE",
			},
		},
		{
			AlertID:      "removed-1",
			SourceID:     "src",
			Status:       "removed",
			Title:        "Removed alert",
			CanonicalURL: "https://example.test/removed",
			Category:     "cyber_advisory",
			Severity:     "low",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				CountryCode:   "DE",
			},
		},
	}
	if err := db.SaveAlerts(context.Background(), alerts); err != nil {
		t.Fatal(err)
	}

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler
	req := httptest.NewRequest("GET", "/api/search?category=cyber_advisory&include_filtered=true&include_removed=true", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []model.Alert `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected active + filtered + removed, got %d", len(resp.Results))
	}
}

func TestSearchLaneFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	alerts := []model.Alert{
		{
			AlertID:      "alarm-1",
			SourceID:     "src",
			Status:       "active",
			Title:        "Alarm lane alert",
			CanonicalURL: "https://example.test/alarm",
			Category:     "missing_person",
			Severity:     "critical",
			SignalLane:   model.SignalLaneAlarm,
			RegionTag:    "EU",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				CountryCode:   "DE",
			},
		},
		{
			AlertID:      "intel-1",
			SourceID:     "src",
			Status:       "active",
			Title:        "Intel lane alert",
			CanonicalURL: "https://example.test/intel",
			Category:     "cyber_advisory",
			Severity:     "medium",
			SignalLane:   model.SignalLaneIntel,
			RegionTag:    "EU",
			FirstSeen:    now,
			LastSeen:     now,
			Source: model.SourceMetadata{
				SourceID:      "src",
				AuthorityName: "Source",
				CountryCode:   "DE",
			},
		},
	}
	if err := db.SaveAlerts(context.Background(), alerts); err != nil {
		t.Fatal(err)
	}

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler
	req := httptest.NewRequest("GET", "/api/search?region=EU&status=active&lane=intel", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []model.Alert `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].AlertID != "intel-1" {
		t.Fatalf("expected only intel lane results, got %#v", resp.Results)
	}
}

func TestRateLimitReturns429(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	seedAlerts(t, db)

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	// Exhaust the burst (30 requests).
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest("GET", "/api/search?q=europol", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			// Burst may be slightly less than 30 due to token consumption timing.
			// Reaching 429 early is fine — the limiter works.
			return
		}
	}

	// The 31st request should be rate limited.
	req := httptest.NewRequest("GET", "/api/search?q=europol", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestRateLimitSkipsHealth(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	// Even after many requests, health should never be rate limited.
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.RemoteAddr = "10.0.0.2:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("health request %d: expected 200, got %d", i, w.Code)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestNoiseFeedbackCreateAndStats(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	seedAlerts(t, db)

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	body := []byte(`{"alert_id":"a1","verdict":"false_positive","analyst":"ops","notes":"noise from broad source"}`)
	req := httptest.NewRequest("POST", "/api/noise-feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/noise-feedback/stats", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats struct {
		Total     int            `json:"total"`
		ByVerdict map[string]int `json:"by_verdict"`
	}
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 {
		t.Fatalf("expected 1 feedback record, got %d", stats.Total)
	}
	if stats.ByVerdict["false_positive"] != 1 {
		t.Fatalf("expected false_positive count 1, got %#v", stats.ByVerdict)
	}
}

func TestDigestEndpointReturnsCountryTerms(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	seedAlerts(t, db)

	// Record trends so the digest has data.
	detector := trends.New(db.RawDB())
	if err := detector.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	alerts := []model.Alert{
		{Title: "Drug trafficking network dismantled", Category: "public_appeal", RegionTag: "EU", Severity: "high",
			Source: model.SourceMetadata{CountryCode: "NL", AuthorityType: "police"}},
		{Title: "Ransomware attack on infrastructure", Category: "cyber_advisory", RegionTag: "EU", Severity: "critical",
			Source: model.SourceMetadata{CountryCode: "NL", AuthorityType: "cert"}},
	}
	if err := detector.Record(context.Background(), alerts, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	srv := New(db, ":0", os.Stderr)
	handler := srv.srv.Handler

	// Single country digest.
	req := httptest.NewRequest("GET", "/api/digest?cc=NL&days=7&limit=5", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		CountryCode string              `json:"country_code"`
		Terms       []trends.DigestTerm `json:"terms"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CountryCode != "NL" {
		t.Errorf("expected country_code NL, got %q", resp.CountryCode)
	}
	if len(resp.Terms) == 0 {
		t.Error("expected at least one digest term for NL")
	}

	// All-countries digest.
	req2 := httptest.NewRequest("GET", "/api/digest?days=7", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}
