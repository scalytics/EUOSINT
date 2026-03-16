// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/collector/model"
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
			AlertID:     "a1",
			SourceID:    "europol",
			Status:      "active",
			Title:       "Europol dismantles major drug trafficking network",
			CanonicalURL: "https://europol.europa.eu/a1",
			Category:    "public_appeal",
			Severity:    "high",
			RegionTag:   "EU",
			Source: model.SourceMetadata{
				SourceID:      "europol",
				AuthorityName: "Europol",
				Country:       "Netherlands",
				CountryCode:   "NL",
				Region:        "Europe",
			},
		},
		{
			AlertID:     "a2",
			SourceID:    "fbi-wanted",
			Status:      "active",
			Title:       "FBI Most Wanted: Cyber fugitive identified",
			CanonicalURL: "https://fbi.gov/a2",
			Category:    "wanted_suspect",
			Severity:    "critical",
			RegionTag:   "US",
			Source: model.SourceMetadata{
				SourceID:      "fbi-wanted",
				AuthorityName: "FBI",
				Country:       "United States",
				CountryCode:   "US",
				Region:        "North America",
			},
		},
		{
			AlertID:     "a3",
			SourceID:    "cert-ua",
			Status:      "active",
			Title:       "CERT-UA reports new malware campaign targeting energy sector",
			CanonicalURL: "https://cert.gov.ua/a3",
			Category:    "cyber_advisory",
			Severity:    "high",
			RegionTag:   "UA",
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
