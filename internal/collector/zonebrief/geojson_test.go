package zonebrief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
)

func TestBuildConflictZonesGeoJSONIncludesInactive(t *testing.T) {
	data := BuildConflictZonesGeoJSON([]model.ZoneBriefingRecord{
		{LensID: "gaza", Title: "Gaza", Status: "active", UpdatedAt: "2026-03-20T00:00:00Z", ConflictStartDate: "2003-02-12T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
		{LensID: "ukraine", Title: "Ukraine South", Status: "inactive", UpdatedAt: "2026-03-20T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
	})
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) != 2 {
		t.Fatalf("expected 2 features (active + inactive), got %d", len(parsed.Features))
	}
	if parsed.Features[0].Properties["lens_id"] == "gaza" && parsed.Features[0].Properties["since"] != "2003" {
		t.Fatalf("expected conflict start year from conflict_start_date, got %#v", parsed.Features[0].Properties["since"])
	}
}

func TestBuildConflictZonesGeoJSONFromBoundariesUsesCountryFeatures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "countries.geojson")
	body := `{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "properties": {"ISO_A2": "UA", "name": "Ukraine"},
      "geometry": {"type": "Polygon", "coordinates": [[[30,44],[30,45],[31,45],[31,44],[30,44]]]}
    }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := BuildConflictZonesGeoJSONFromBoundaries([]model.ZoneBriefingRecord{
		{
			LensID:        "ukraine",
			Title:         "Ukraine South",
			Source:        "UCDP GED",
			SourceURL:     "https://ucdp.uu.se/country/369",
			Status:        "active",
			UpdatedAt:     "2026-03-20T00:00:00Z",
			CountryIDs:    []string{"369"},
			CountryLabels: []string{"Ukraine"},
			Violence:      model.ZoneBriefingViolence{Primary: "State-based conflict"},
		},
	}, path)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
			Geometry   struct {
				Type string `json:"type"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) == 0 {
		t.Fatal("expected at least one feature from country boundaries")
	}
	if parsed.Features[0].Geometry.Type != "Polygon" {
		t.Fatalf("expected polygon geometry, got %q", parsed.Features[0].Geometry.Type)
	}
	if parsed.Features[0].Properties["country_source_url"] != "https://ucdp.uu.se/country/369" {
		t.Fatalf("expected country source URL property, got %#v", parsed.Features[0].Properties["country_source_url"])
	}
}

func TestBuildConflictZonesGeoJSONFromBoundariesUsesPrimaryOverlayCountries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "countries.geojson")
	body := `{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "properties": {"ISO_A2": "PS", "name": "Palestine"},
      "geometry": {"type": "Polygon", "coordinates": [[[34.2,31.2],[34.2,31.6],[34.7,31.6],[34.7,31.2],[34.2,31.2]]]}
    },
    {
      "type": "Feature",
      "properties": {"ISO_A2": "IL", "name": "Israel"},
      "geometry": {"type": "Polygon", "coordinates": [[[34.7,31.2],[34.7,31.8],[35.2,31.8],[35.2,31.2],[34.7,31.2]]]}
    }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := BuildConflictZonesGeoJSONFromBoundaries([]model.ZoneBriefingRecord{
		{
			LensID:        "gaza",
			Title:         "Gaza",
			Source:        "UCDP GED",
			SourceURL:     "https://ucdp.uu.se/country/666",
			Status:        "active",
			UpdatedAt:     "2026-03-20T00:00:00Z",
			CountryIDs:    []string{"666"},
			CountryLabels: []string{"Palestine"},
			Violence:      model.ZoneBriefingViolence{Primary: "State-based conflict"},
		},
	}, path)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) != 1 {
		t.Fatalf("expected only primary overlay country feature, got %d", len(parsed.Features))
	}
	if parsed.Features[0].Properties["country_code"] != "PS" {
		t.Fatalf("expected Gaza overlay country PS, got %#v", parsed.Features[0].Properties["country_code"])
	}
}

func TestFilterZonesGeoJSONByLens(t *testing.T) {
	data := BuildConflictZonesGeoJSON([]model.ZoneBriefingRecord{
		{LensID: "gaza", Title: "Gaza", Status: "active", UpdatedAt: "2026-03-20T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
		{LensID: "ukraine", Title: "Ukraine South", Status: "active", UpdatedAt: "2026-03-20T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
	})
	filtered := FilterZonesGeoJSONByLens(data, "gaza")
	body, err := json.Marshal(filtered)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) != 1 {
		t.Fatalf("expected 1 gaza feature, got %d", len(parsed.Features))
	}
	if parsed.Features[0].Properties["lens_id"] != "gaza" {
		t.Fatalf("expected filtered lens_id gaza, got %#v", parsed.Features[0].Properties["lens_id"])
	}
}

func TestBuildConflictFootprintsGeoJSONUsesSingleLensFootprint(t *testing.T) {
	data := BuildConflictFootprintsGeoJSON([]model.ZoneBriefingRecord{
		{
			LensID:    "gaza",
			Title:     "Gaza",
			Status:    "active",
			UpdatedAt: "2026-03-20T00:00:00Z",
			Violence:  model.ZoneBriefingViolence{Primary: "State-based conflict"},
		},
		{
			LensID:    "ukraine",
			Title:     "Ukraine South",
			Status:    "watch",
			UpdatedAt: "2026-03-20T00:00:00Z",
			Violence:  model.ZoneBriefingViolence{Primary: "State-based conflict"},
		},
	}, []parse.UCDPItem{
		{FeedItem: parse.FeedItem{Lat: 31.42, Lng: 34.33}, CountryCode: "PS", WherePrecision: 1},
		{FeedItem: parse.FeedItem{Lat: 31.47, Lng: 34.42}, CountryCode: "PS", WherePrecision: 2},
		{FeedItem: parse.FeedItem{Lat: 0, Lng: 0}, CountryCode: "PS", WherePrecision: 1},         // dropped
		{FeedItem: parse.FeedItem{Lat: 31.40, Lng: 34.36}, CountryCode: "PS", WherePrecision: 5}, // dropped
	})
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
			Geometry   struct {
				Type string `json:"type"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) != 1 {
		t.Fatalf("expected only hotspot-backed footprint features, got %d", len(parsed.Features))
	}
	if parsed.Features[0].Geometry.Type != "Polygon" {
		t.Fatalf("expected polygon footprint geometry, got %q", parsed.Features[0].Geometry.Type)
	}
	if parsed.Features[0].Properties["geometry_role"] != "footprint" {
		t.Fatalf("expected geometry_role footprint, got %#v", parsed.Features[0].Properties["geometry_role"])
	}
}

func TestBuildConflictFootprintsGeoJSONFiltersOutsideLensBounds(t *testing.T) {
	data := BuildConflictFootprintsGeoJSON([]model.ZoneBriefingRecord{
		{
			LensID:    "ukraine",
			Title:     "Ukraine South",
			Status:    "active",
			UpdatedAt: "2026-03-20T00:00:00Z",
			Violence:  model.ZoneBriefingViolence{Primary: "State-based conflict"},
		},
	}, []parse.UCDPItem{
		{FeedItem: parse.FeedItem{Lat: 47.5, Lng: 35.1}, CountryCode: "369", WherePrecision: 1}, // in lens bounds
		{FeedItem: parse.FeedItem{Lat: 61.0, Lng: 90.0}, CountryCode: "365", WherePrecision: 1}, // outside bounds
	})

	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
			Geometry   struct {
				Type string `json:"type"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) != 1 {
		t.Fatalf("expected 1 lens footprint feature from bounded event points, got %d", len(parsed.Features))
	}
	if parsed.Features[0].Properties["lens_id"] != "ukraine" {
		t.Fatalf("expected lens_id ukraine, got %#v", parsed.Features[0].Properties["lens_id"])
	}
}
