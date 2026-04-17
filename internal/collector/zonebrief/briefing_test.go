package zonebrief

import (
	"testing"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/parse"
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

	briefs := Build(items, nil, nil, now)
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
		if brief.Metrics.FatalitiesTotal != 12 {
			t.Fatalf("expected 12 total fatalities, got %d", brief.Metrics.FatalitiesTotal)
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
	sudanLens := SupportedLenses[1] // sudan
	if sudanLens.ID != "sudan" {
		t.Fatalf("expected sudan lens at index 1, got %q", sudanLens.ID)
	}

	item := parse.UCDPItem{
		FeedItem:    parse.FeedItem{Lat: 0, Lng: 0},
		CountryCode: "625",
	}
	if !matchesLens(sudanLens, item) {
		t.Fatal("expected numeric gwno '625' to match Sudan lens via ISO2 conversion")
	}

	item.CountryCode = "365" // Russia
	if matchesLens(sudanLens, item) {
		t.Fatal("gwno '365' (Russia) should not match Sudan lens")
	}
}

func TestBuildWithNumericCountryCodes(t *testing.T) {
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

	briefs := Build(items, nil, nil, now)
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

func TestBuildFiltersPlaceholderActors(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	items := []parse.UCDPItem{
		{
			FeedItem:     parse.FeedItem{Published: "2026-03-18"},
			ViolenceType: "State-based conflict",
			Fatalities:   7,
			Country:      "Ukraine",
			CountryCode:  "369",
			SideA:        "Government of Ukraine",
			SideB:        "XXX369",
		},
	}

	briefs := Build(items, nil, nil, now)
	for _, brief := range briefs {
		if brief.LensID != "ukraine" {
			continue
		}
		for _, actor := range brief.Actors {
			if actor == "XXX369" || actor == "XXX666" {
				t.Fatalf("placeholder actor leaked into briefing actors: %q", actor)
			}
		}
		return
	}
	t.Fatal("expected ukraine briefing in output")
}

func TestMatchConflictsToLens(t *testing.T) {
	conflicts := []parse.UCDPConflict{
		{ConflictID: "1", ConflictName: "Sudan internal", GWNoLoc: "625", IntensityLevel: 2, TypeOfConflict: "3"},
		{ConflictID: "2", ConflictName: "Ukraine-Russia", GWNoLoc: "369", IntensityLevel: 2, TypeOfConflict: "4"},
		{ConflictID: "3", ConflictName: "Unrelated", GWNoLoc: "999", IntensityLevel: 1, TypeOfConflict: "3"},
	}
	sudanLens := SupportedLenses[1]
	matched := matchConflictsToLens(sudanLens, conflicts)
	if len(matched) != 1 || matched[0].ConflictID != "1" {
		t.Fatalf("expected 1 Sudan conflict match, got %d", len(matched))
	}
}

func TestEnrichWithConflicts(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	items := []parse.UCDPItem{
		{
			FeedItem:     parse.FeedItem{Published: "2026-03-19"},
			ViolenceType: "State-based conflict",
			Fatalities:   10,
			Country:      "Sudan",
			CountryCode:  "625",
		},
	}
	conflicts := []parse.UCDPConflict{
		{ConflictID: "1", ConflictName: "Sudan civil war", GWNoLoc: "625", IntensityLevel: 2, TypeOfConflict: "3", Year: 2026, StartDate: "2003-02-12"},
	}
	briefs := Build(items, conflicts, nil, now)
	var found bool
	for _, brief := range briefs {
		if brief.LensID != "sudan" {
			continue
		}
		found = true
		if brief.ConflictIntensity != "war" {
			t.Fatalf("expected conflict_intensity 'war', got %q", brief.ConflictIntensity)
		}
		if len(brief.ActiveConflicts) != 1 {
			t.Fatalf("expected 1 active conflict, got %d", len(brief.ActiveConflicts))
		}
		if brief.ConflictStartDate != "2003-02-12T00:00:00Z" {
			t.Fatalf("expected normalized conflict start date, got %q", brief.ConflictStartDate)
		}
	}
	if !found {
		t.Fatal("expected sudan briefing")
	}
}

func TestACLEDStatusFallback(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	// No UCDP events -> status would be "inactive"
	acledItems := []parse.ACLEDItem{
		{
			FeedItem:   parse.FeedItem{Published: "2026-03-19", Title: "Clash in Darfur", Lat: 13.0, Lng: 25.0},
			EventType:  "Battles",
			Fatalities: 5,
			Country:    "Sudan",
			ISO3:       "SDN",
		},
	}
	briefs := Build(nil, nil, acledItems, now)
	var found bool
	for _, brief := range briefs {
		if brief.LensID != "sudan" {
			continue
		}
		found = true
		if brief.Status != "active" {
			t.Fatalf("expected status 'active' from ACLED fallback, got %q", brief.Status)
		}
		if brief.ACLEDRecency == nil {
			t.Fatal("expected ACLED recency data")
		}
		if brief.ACLEDRecency.Events7D != 1 {
			t.Fatalf("expected 1 ACLED event in 7d, got %d", brief.ACLEDRecency.Events7D)
		}
	}
	if !found {
		t.Fatal("expected sudan briefing")
	}
}
