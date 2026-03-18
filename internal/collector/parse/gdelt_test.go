// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"testing"
)

func TestParseGDELT(t *testing.T) {
	doc := GDELTResponse{
		Articles: []GDELTArticle{
			{
				URL:           "https://example.com/article1",
				Title:         "Military tensions rise in Eastern Europe",
				Seendate:      "20260318T120000Z",
				Domain:        "example.com",
				Language:      "English",
				SourceCountry: "United States",
			},
			{
				URL:           "https://example.com/article2",
				Title:         "New sanctions imposed on Russia",
				Seendate:      "20260317T080000Z",
				Domain:        "reuters.com",
				SourceCountry: "United Kingdom",
			},
			{
				// Duplicate URL — should be deduped.
				URL:           "https://example.com/article1",
				Title:         "Military tensions rise in Eastern Europe (duplicate)",
				Seendate:      "20260318T130000Z",
				Domain:        "mirror.com",
				SourceCountry: "United States",
			},
		},
	}
	body, _ := json.Marshal(doc)

	items, err := ParseGDELT(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (deduped), got %d", len(items))
	}

	if items[0].Published != "2026-03-18T12:00:00Z" {
		t.Errorf("expected RFC3339 date, got %q", items[0].Published)
	}
	if items[0].Author != "example.com" {
		t.Errorf("expected author=example.com, got %q", items[0].Author)
	}
}

func TestNormalizeGDELTDate(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"20260318T120000Z", "2026-03-18T12:00:00Z"},
		{"20260101", "2026-01-01"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeGDELTDate(tt.in)
		if got != tt.want {
			t.Errorf("normalizeGDELTDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGDELTCountryISO2(t *testing.T) {
	tests := []struct {
		country string
		want    string
	}{
		{"United States", "US"},
		{"united kingdom", "GB"},
		{"Unknown", ""},
	}
	for _, tt := range tests {
		got := GDELTCountryISO2(tt.country)
		if got != tt.want {
			t.Errorf("GDELTCountryISO2(%q) = %q, want %q", tt.country, got, tt.want)
		}
	}
}
