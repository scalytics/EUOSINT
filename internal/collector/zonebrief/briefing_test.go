package zonebrief

import (
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/parse"
)

func TestBuildGeneratesLensBriefings(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	items := []parse.UCDPItem{
		{
			FeedItem:       parse.FeedItem{Published: "2026-03-19", Lat: 31.4, Lng: 34.3},
			ViolenceType:   "State-based conflict",
			Fatalities:     12,
			CivilianDeaths: 4,
			Country:        "Palestine",
			CountryCode:    "PS",
			SideA:          "Actor A",
			SideB:          "Actor B",
			DyadName:       "Actor A vs Actor B",
			Admin1:         "Gaza",
			Admin2:         "Rafah",
			WherePrecision: 2,
			DatePrecision:  1,
			EventClarity:   1,
		},
	}

	briefs := Build(items, now)
	if len(briefs) == 0 {
		t.Fatal("expected zone briefings")
	}
	var gazaFound bool
	for _, brief := range briefs {
		if brief.LensID != "gaza" {
			continue
		}
		gazaFound = true
		if brief.Metrics.Events7D != 1 {
			t.Fatalf("expected 1 event in 7d, got %d", brief.Metrics.Events7D)
		}
		if brief.Metrics.FatalitiesBest30D != 12 {
			t.Fatalf("expected 12 fatalities, got %d", brief.Metrics.FatalitiesBest30D)
		}
		if brief.SourceURL != "https://ucdp.uu.se/country/666" {
			t.Fatalf("expected deterministic UCDP source URL, got %q", brief.SourceURL)
		}
		if len(brief.ActorSummary.TopActors) == 0 || brief.ActorSummary.TopActors[0] != "Actor A" {
			t.Fatalf("unexpected top actors: %#v", brief.ActorSummary.TopActors)
		}
	}
	if !gazaFound {
		t.Fatal("expected gaza briefing in output")
	}
}

func TestMatchesLensWithNumericGWNO(t *testing.T) {
	// UCDP API returns numeric GW country codes (e.g. "625" for Sudan),
	// not ISO2 codes. Verify that matchesLens converts them correctly.
	sudanLens := supportedLenses[1] // sudan
	if sudanLens.ID != "sudan" {
		t.Fatalf("expected sudan lens at index 1, got %q", sudanLens.ID)
	}

	// Event with gwno "625" (Sudan) but coords outside bounds — must match by country code.
	item := parse.UCDPItem{
		FeedItem:    parse.FeedItem{Lat: 0, Lng: 0},
		CountryCode: "625",
	}
	if !matchesLens(sudanLens, item) {
		t.Fatal("expected numeric gwno '625' to match Sudan lens via ISO2 conversion")
	}

	// Verify an unrelated country code does not match.
	item.CountryCode = "365" // Russia
	if matchesLens(sudanLens, item) {
		t.Fatal("gwno '365' (Russia) should not match Sudan lens")
	}
}

func TestBuildWithNumericCountryCodes(t *testing.T) {
	// Simulate real UCDP data with numeric gwno country codes and no coords.
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	items := []parse.UCDPItem{
		{
			FeedItem:     parse.FeedItem{Published: "2026-03-18"},
			ViolenceType: "Non-state conflict",
			Fatalities:   5,
			Country:      "Sudan",
			CountryCode:  "625",
			SideA:        "RSF",
			SideB:        "SAF",
		},
	}

	briefs := Build(items, now)
	var found bool
	for _, brief := range briefs {
		if brief.LensID != "sudan" {
			continue
		}
		found = true
		if brief.Metrics.Events7D != 1 {
			t.Fatalf("expected 1 event in 7d for Sudan, got %d", brief.Metrics.Events7D)
		}
		if len(brief.CountryLabels) == 0 || brief.CountryLabels[0] != "Sudan" {
			t.Fatalf("expected country label 'Sudan', got %v", brief.CountryLabels)
		}
	}
	if !found {
		t.Fatal("expected sudan briefing in output")
	}
}
