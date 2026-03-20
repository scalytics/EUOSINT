// Copyright 2025 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseUCDPResultEnvelope(t *testing.T) {
	body := []byte(`{
	  "Result": [
	    {
	      "id": "123",
	      "date_start": "2026-03-18",
	      "country": "Syria",
	      "region": "Middle East",
	      "type_of_violence": "1",
	      "side_a": "Government",
	      "side_b": "Opposition",
	      "best": 12,
	      "latitude": 35.1,
	      "longitude": 36.7
	    }
	  ]
	}`)
	items, err := ParseUCDP(body)
	if err != nil {
		t.Fatalf("ParseUCDP returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ViolenceType != "State-based conflict" {
		t.Fatalf("unexpected violence type: %q", items[0].ViolenceType)
	}
	if items[0].Fatalities != 12 {
		t.Fatalf("unexpected fatalities: %d", items[0].Fatalities)
	}
	if items[0].Country != "Syria" {
		t.Fatalf("unexpected country: %q", items[0].Country)
	}
	if items[0].Lat == 0 || items[0].Lng == 0 {
		t.Fatalf("expected coordinates, got (%f, %f)", items[0].Lat, items[0].Lng)
	}
}

func TestParseUCDPResultsEnvelope(t *testing.T) {
	body := []byte(`{
	  "results": [
	    {
	      "event_id": "x1",
	      "date_start": "2026-01-01",
	      "country_name": "Ukraine",
	      "type_of_violence_text": "One-sided violence",
	      "fatalities_best": "3"
	    }
	  ]
	}`)
	items, err := ParseUCDP(body)
	if err != nil {
		t.Fatalf("ParseUCDP returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Fatalities != 3 {
		t.Fatalf("unexpected fatalities: %d", items[0].Fatalities)
	}
	if items[0].Published != "2026-01-01" {
		t.Fatalf("unexpected published date: %q", items[0].Published)
	}
}
