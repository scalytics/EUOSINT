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
