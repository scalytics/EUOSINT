// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"testing"
)

func TestParseUSGSGeoJSON(t *testing.T) {
	doc := USGSGeoJSON{
		Features: []USGSFeature{
			{
				ID: "us7000abc1",
				Properties: USGSProperties{
					Mag:     6.2,
					Place:   "45km NNE of Hualien City, Taiwan",
					Time:    1710720000000, // 2024-03-18T00:00:00Z
					URL:     "https://earthquake.usgs.gov/earthquakes/eventpage/us7000abc1",
					Title:   "M 6.2 - 45km NNE of Hualien City, Taiwan",
					Alert:   "yellow",
					Tsunami: 1,
					Type:    "earthquake",
				},
				Geometry: USGSGeometry{
					Coordinates: []float64{121.75, 24.15, 15.0},
				},
			},
			{
				ID: "us7000abc2",
				Properties: USGSProperties{
					Mag:   4.8,
					Place: "120km S of Arica, Chile",
					Time:  1710720060000,
					Title: "M 4.8 - 120km S of Arica, Chile",
					Type:  "earthquake",
				},
				Geometry: USGSGeometry{
					Coordinates: []float64{-70.33, -19.60, 100.0},
				},
			},
		},
	}
	body, _ := json.Marshal(doc)

	items, err := ParseUSGSGeoJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	eq := items[0]
	if eq.Magnitude != 6.2 {
		t.Errorf("expected mag=6.2, got %f", eq.Magnitude)
	}
	if !eq.Tsunami {
		t.Error("expected tsunami=true")
	}
	if eq.Lat != 24.15 || eq.Lng != 121.75 {
		t.Errorf("expected lat=24.15 lng=121.75, got %f %f", eq.Lat, eq.Lng)
	}
	if eq.Published == "" {
		t.Error("expected non-empty published")
	}

	small := items[1]
	if small.Tsunami {
		t.Error("expected tsunami=false")
	}
	if small.Magnitude != 4.8 {
		t.Errorf("expected mag=4.8, got %f", small.Magnitude)
	}
}

func TestUSGSSeverity(t *testing.T) {
	tests := []struct {
		mag   float64
		alert string
		want  string
	}{
		{7.5, "", "critical"},
		{6.2, "yellow", "critical"},
		{5.5, "", "high"},
		{4.5, "", "medium"},
		{4.5, "red", "critical"},
		{5.0, "orange", "critical"},
	}
	for _, tt := range tests {
		got := USGSSeverity(tt.mag, tt.alert)
		if got != tt.want {
			t.Errorf("USGSSeverity(%.1f, %q) = %q, want %q", tt.mag, tt.alert, got, tt.want)
		}
	}
}
