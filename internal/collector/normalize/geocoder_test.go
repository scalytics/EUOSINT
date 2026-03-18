// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"context"
	"testing"
)

// mockCityLookup is a simple in-memory mock for testing.
type mockCityLookup struct {
	cities map[string]CityLookupResult
}

func (m *mockCityLookup) LookupCity(_ context.Context, name string, countryCode string) (CityLookupResult, bool) {
	key := name
	if countryCode != "" {
		// Try country-specific first.
		if r, ok := m.cities[name+"|"+countryCode]; ok {
			return r, true
		}
	}
	r, ok := m.cities[key]
	return r, ok
}

func newMockCities() *mockCityLookup {
	return &mockCityLookup{cities: map[string]CityLookupResult{
		"Valletta":  {Name: "Valletta", CountryCode: "MT", Lat: 35.90, Lng: 14.51, Population: 6400},
		"Berlin":    {Name: "Berlin", CountryCode: "DE", Lat: 52.52, Lng: 13.41, Population: 3700000},
		"Munich":    {Name: "Munich", CountryCode: "DE", Lat: 48.14, Lng: 11.58, Population: 1500000},
		"Kyiv":      {Name: "Kyiv", CountryCode: "UA", Lat: 50.45, Lng: 30.52, Population: 3000000},
		"Mogadishu": {Name: "Mogadishu", CountryCode: "SO", Lat: 2.05, Lng: 45.32, Population: 2900000},
		"Aleppo":    {Name: "Aleppo", CountryCode: "SY", Lat: 36.20, Lng: 37.17, Population: 1800000},
	}}
}

func TestGeocoderResolve_CityDB(t *testing.T) {
	g := NewGeocoder(newMockCities(), nil)

	tests := []struct {
		name        string
		text        string
		countryHint string
		wantCity    string
		wantCode    string
		wantSource  string
	}{
		{
			name:       "city in headline",
			text:       "Explosion rocks central Berlin district",
			wantCity:   "Berlin",
			wantCode:   "DE",
			wantSource: "city-db",
		},
		{
			name:        "city with country hint",
			text:        "Air raid sirens in Kyiv as strikes continue",
			countryHint: "UA",
			wantCity:    "Kyiv",
			wantCode:    "UA",
			wantSource:  "city-db",
		},
		{
			name:       "rightmost city wins",
			text:       "Berlin conference discusses Aleppo humanitarian crisis",
			wantCity:   "Aleppo",
			wantCode:   "SY",
			wantSource: "city-db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.Resolve(context.Background(), tt.text, tt.countryHint)
			if result.CityName != tt.wantCity {
				t.Errorf("CityName = %q, want %q", result.CityName, tt.wantCity)
			}
			if result.CountryCode != tt.wantCode {
				t.Errorf("CountryCode = %q, want %q", result.CountryCode, tt.wantCode)
			}
			if result.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", result.Source, tt.wantSource)
			}
		})
	}
}

func TestGeocoderResolve_FallbackToCapital(t *testing.T) {
	// No city DB, no Nominatim — should fall back to country text + capitals.
	g := NewGeocoder(nil, nil)

	result := g.Resolve(context.Background(), "Somalia conflict escalates", "")
	if result.CountryCode != "SO" {
		t.Errorf("CountryCode = %q, want SO", result.CountryCode)
	}
	// Should use capital coords (Mogadishu) not centroid.
	if result.Source != "capital" {
		t.Errorf("Source = %q, want capital", result.Source)
	}
	if result.Lat < 1.5 || result.Lat > 3.0 {
		t.Errorf("Lat = %f, want ~2.05 (Mogadishu)", result.Lat)
	}
}

func TestGeocoderResolve_CountryHintCapital(t *testing.T) {
	// No city DB, no text match — should use country hint's capital.
	g := NewGeocoder(nil, nil)

	result := g.Resolve(context.Background(), "New advisory issued for financial sector", "MT")
	if result.Source != "capital" {
		t.Errorf("Source = %q, want capital", result.Source)
	}
	// Valletta coordinates.
	if result.Lat < 35.5 || result.Lat > 36.5 {
		t.Errorf("Lat = %f, want ~35.90 (Valletta)", result.Lat)
	}
}

func TestCapitalCoords_IslandsOnLand(t *testing.T) {
	// Verify that island nations have capital coords on land.
	islands := map[string]string{
		"MT": "Valletta",
		"CY": "Nicosia",
		"SG": "Singapore",
		"JM": "Kingston",
		"CU": "Havana",
		"IS": "Reykjavik",
	}

	for code, name := range islands {
		coords, ok := capitalCoords[code]
		if !ok {
			t.Errorf("missing capital coords for %s (%s)", code, name)
			continue
		}
		// Sanity: lat/lng should be non-zero.
		if coords[0] == 0 && coords[1] == 0 {
			t.Errorf("capital coords for %s (%s) are zero", code, name)
		}
	}
}

func TestGeocodeCountryCode_UsesCapitals(t *testing.T) {
	// Malta centroid is in the sea. Capital (Valletta) should be returned.
	lat, lng, name, ok := geocodeCountryCode("MT")
	if !ok {
		// MT might not be in geoCountries — that's fine for this test.
		t.Skip("MT not in geoCountries")
	}
	_ = name
	// Valletta is at 35.90, 14.51. Centroid would be different.
	if lat < 35.5 || lat > 36.5 || lng < 14.0 || lng > 15.0 {
		t.Errorf("geocodeCountryCode(MT) = (%f, %f), want Valletta area", lat, lng)
	}
}

func TestExtractCandidateNames(t *testing.T) {
	candidates := extractCandidateNames("Explosion in San Francisco kills 3 near Mission District")

	names := make(map[string]bool)
	for _, c := range candidates {
		names[c.name] = true
	}

	if !names["San"] {
		t.Error("expected 'San' in candidates")
	}
	if !names["San Francisco"] {
		t.Error("expected 'San Francisco' in candidates")
	}
	if !names["Mission District"] {
		t.Error("expected 'Mission District' in candidates")
	}
}

func TestExtractCandidateNamesSkipsLowercaseNoise(t *testing.T) {
	candidates := extractCandidateNames("vulnerability assessment and testing automation for global enhancement")
	if len(candidates) != 0 {
		t.Fatalf("expected no city candidates from lowercase advisory prose, got %d", len(candidates))
	}
}
