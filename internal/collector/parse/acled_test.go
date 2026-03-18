// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"testing"
)

func TestParseACLED(t *testing.T) {
	resp := ACLEDResponse{
		Status: 200,
		Count:  2,
		Data: []ACLEDEvent{
			{
				DataID:       json.Number("12345"),
				EventDate:    "2026-03-15",
				EventType:    "Battles",
				SubEventType: "Armed clash",
				Actor1:       "Military Forces of Russia",
				Actor2:       "Military Forces of Ukraine",
				Country:      "Ukraine",
				ISO3:         "UKR",
				Region:       "Europe",
				Admin1:       "Donetsk",
				Location:     "Bakhmut",
				Latitude:     "48.5953",
				Longitude:    "38.0003",
				Notes:        "Clashes reported near Bakhmut between Russian and Ukrainian forces.",
				Fatalities:   json.Number("5"),
				Source:       "Ukrainian General Staff",
			},
			{
				DataID:       json.Number("12346"),
				EventDate:    "2026-03-15",
				EventType:    "Protests",
				SubEventType: "Peaceful protest",
				Actor1:       "Protesters (Georgia)",
				Country:      "Georgia",
				ISO3:         "GEO",
				Region:       "Europe",
				Admin1:       "Tbilisi",
				Location:     "Tbilisi",
				Latitude:     "41.7151",
				Longitude:    "44.8271",
				Notes:        "Protesters gathered outside parliament building.",
				Fatalities:   json.Number("0"),
				Source:       "Civil.ge",
			},
		},
	}
	body, _ := json.Marshal(resp)

	items, total, err := ParseACLED(body)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// First item: battle with fatalities.
	battle := items[0]
	if battle.EventType != "Battles" {
		t.Errorf("expected EventType=Battles, got %q", battle.EventType)
	}
	if battle.Fatalities != 5 {
		t.Errorf("expected Fatalities=5, got %d", battle.Fatalities)
	}
	if battle.Lat == 0 || battle.Lng == 0 {
		t.Error("expected non-zero coordinates")
	}
	if battle.Title == "" {
		t.Error("expected non-empty title")
	}

	// Second item: protest, no fatalities.
	protest := items[1]
	if protest.Fatalities != 0 {
		t.Errorf("expected Fatalities=0, got %d", protest.Fatalities)
	}
}

func TestACLEDEventCategory(t *testing.T) {
	tests := []struct {
		eventType string
		want      string
	}{
		{"Battles", "conflict_monitoring"},
		{"Explosions/Remote violence", "conflict_monitoring"},
		{"Violence against civilians", "conflict_monitoring"},
		{"Protests", "public_safety"},
		{"Riots", "public_safety"},
		{"Strategic developments", "intelligence_report"},
	}
	for _, tt := range tests {
		got := ACLEDEventCategory(tt.eventType)
		if got != tt.want {
			t.Errorf("ACLEDEventCategory(%q) = %q, want %q", tt.eventType, got, tt.want)
		}
	}
}

func TestACLEDEventSeverity(t *testing.T) {
	tests := []struct {
		eventType  string
		fatalities int
		want       string
	}{
		{"Battles", 0, "high"},
		{"Battles", 15, "critical"},
		{"Protests", 0, "medium"},
		{"Protests", 3, "high"},
	}
	for _, tt := range tests {
		got := ACLEDEventSeverity(tt.eventType, tt.fatalities)
		if got != tt.want {
			t.Errorf("ACLEDEventSeverity(%q, %d) = %q, want %q", tt.eventType, tt.fatalities, got, tt.want)
		}
	}
}
