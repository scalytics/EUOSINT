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
			FeedItem:       parse.FeedItem{Title: "State-based conflict in Palestine", Link: "https://ucdp.example/event-1", Published: "2026-03-19", Lat: 31.4, Lng: 34.3},
			ViolenceType:   "State-based conflict",
			Fatalities:     12,
			CivilianDeaths: 4,
			Country:        "Palestine",
			CountryID:      "999",
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
		if len(brief.RecentEvents) != 1 {
			t.Fatalf("expected one recent event, got %d", len(brief.RecentEvents))
		}
		if brief.RecentEvents[0].Title != "State-based conflict in Palestine" {
			t.Fatalf("unexpected recent event title %q", brief.RecentEvents[0].Title)
		}
	}
	if !gazaFound {
		t.Fatal("expected gaza briefing in output")
	}
}

func TestBuildRedSeaForcesSomaliaPiracyActor(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	items := []parse.UCDPItem{
		{
			FeedItem:       parse.FeedItem{Title: "State-based conflict in Yemen", Published: "2026-03-18", Lat: 15.2, Lng: 42.7},
			ViolenceType:   "State-based conflict",
			Fatalities:     3,
			CivilianDeaths: 1,
			Country:        "Yemen",
			CountryID:      "679",
			CountryCode:    "YE",
			SideA:          "Government of Yemen",
			SideB:          "Houthis",
		},
	}

	briefs := Build(items, now)
	for _, brief := range briefs {
		if brief.LensID != "red-sea" {
			continue
		}
		if len(brief.Actors) == 0 {
			t.Fatal("expected actors for red-sea briefing")
		}
		if brief.Actors[0] != "Somali pirate networks" {
			t.Fatalf("expected forced piracy actor first, got %#v", brief.Actors)
		}
		return
	}
	t.Fatal("expected red-sea briefing in output")
}

func TestBuildMetricsUseLatestAvailableWindow(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	items := []parse.UCDPItem{
		{
			FeedItem:       parse.FeedItem{Title: "State-based conflict in Gaza", Published: "2024-11-23", Lat: 31.45, Lng: 34.4},
			ViolenceType:   "State-based conflict",
			Fatalities:     20,
			CivilianDeaths: 8,
			Country:        "Palestine",
			CountryCode:    "PS",
			SideA:          "Actor A",
			SideB:          "Actor B",
		},
		{
			FeedItem:       parse.FeedItem{Title: "State-based conflict in Gaza", Published: "2024-11-18", Lat: 31.42, Lng: 34.36},
			ViolenceType:   "State-based conflict",
			Fatalities:     5,
			CivilianDeaths: 2,
			Country:        "Palestine",
			CountryCode:    "PS",
			SideA:          "Actor A",
			SideB:          "Actor C",
		},
	}

	briefs := Build(items, now)
	for _, brief := range briefs {
		if brief.LensID != "gaza" {
			continue
		}
		if brief.Metrics.Events30D != 2 {
			t.Fatalf("expected 2 events in latest-window 30d, got %d", brief.Metrics.Events30D)
		}
		if brief.Metrics.FatalitiesBest30D != 25 {
			t.Fatalf("expected 25 fatalities in latest-window 30d, got %d", brief.Metrics.FatalitiesBest30D)
		}
		if brief.Status != "inactive" {
			t.Fatalf("expected stale dataset status inactive, got %q", brief.Status)
		}
		return
	}
	t.Fatal("expected gaza briefing in output")
}
