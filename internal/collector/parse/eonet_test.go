// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"testing"
)

func TestParseEONET(t *testing.T) {
	doc := EONETResponse{
		Events: []EONETEvent{
			{
				ID:    "EONET_1234",
				Title: "Wildfire - California, US",
				Link:  "https://eonet.gsfc.nasa.gov/api/v3/events/EONET_1234",
				Categories: []EONETCategory{
					{ID: "wildfires", Title: "Wildfires"},
				},
				Geometry: []EONETGeometry{
					{Date: "2026-03-15T00:00:00Z", Type: "Point", Coordinates: []float64{-118.5, 34.0}},
					{Date: "2026-03-17T00:00:00Z", Type: "Point", Coordinates: []float64{-118.6, 34.1}},
				},
			},
			{
				ID:    "EONET_5678",
				Title: "Eruption of Mt. Etna",
				Categories: []EONETCategory{
					{ID: "volcanoes", Title: "Volcanoes"},
				},
				Sources: []EONETSource{
					{ID: "SIVolcano", URL: "https://volcano.si.edu/volcano.cfm?vn=211060"},
				},
				Geometry: []EONETGeometry{
					{Date: "2026-03-16T12:00:00Z", Type: "Point", Coordinates: []float64{15.0, 37.75}},
				},
			},
		},
	}
	body, _ := json.Marshal(doc)

	items, err := ParseEONET(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Wildfire should use latest geometry point.
	fire := items[0]
	if fire.Lat != 34.1 || fire.Lng != -118.6 {
		t.Errorf("expected latest coords (34.1, -118.6), got (%f, %f)", fire.Lat, fire.Lng)
	}
	if fire.Published != "2026-03-17T00:00:00Z" {
		t.Errorf("expected latest date, got %q", fire.Published)
	}
	if fire.CategoryID != "wildfires" {
		t.Errorf("expected category=wildfires, got %q", fire.CategoryID)
	}

	// Volcano should use source URL as link.
	volcano := items[1]
	if volcano.Link != "https://volcano.si.edu/volcano.cfm?vn=211060" {
		t.Errorf("expected source URL as link, got %q", volcano.Link)
	}
}

func TestEONETSeverity(t *testing.T) {
	tests := []struct {
		catID string
		want  string
	}{
		{"volcanoes", "high"},
		{"wildfires", "medium"},
		{"floods", "high"},
		{"landslides", "medium"},
		{"unknown", "medium"},
	}
	for _, tt := range tests {
		got := EONETSeverity(tt.catID)
		if got != tt.want {
			t.Errorf("EONETSeverity(%q) = %q, want %q", tt.catID, got, tt.want)
		}
	}
}
