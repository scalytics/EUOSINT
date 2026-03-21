package zonebrief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/collector/model"
)

func TestBuildConflictZonesGeoJSONIncludesInactive(t *testing.T) {
	data := BuildConflictZonesGeoJSON([]model.ZoneBriefingRecord{
		{LensID: "gaza", Title: "Gaza", Status: "active", UpdatedAt: "2026-03-20T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
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
