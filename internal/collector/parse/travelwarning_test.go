// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"testing"
)

func TestParseGermanAATravelWarnings(t *testing.T) {
	body := []byte(`{
		"1": {
			"title": "Afghanistan - Reisewarnung",
			"country": "Afghanistan",
			"warning": "Do not travel to Afghanistan.",
			"severity": "Reisewarnung",
			"lastChanged": "2026-01-15",
			"url": "https://www.auswaertiges-amt.de/de/aussenpolitik/laender/afghanistan-node/afghanistansicherheit/204692"
		},
		"2": {
			"title": "France - Exercise normal safety precautions",
			"country": "France",
			"warning": "No specific warnings.",
			"severity": "",
			"lastChanged": "2026-02-01",
			"url": "https://www.auswaertiges-amt.de/de/aussenpolitik/laender/frankreich-node"
		}
	}`)
	items, err := ParseGermanAATravelWarnings(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	foundAfghan := false
	for _, item := range items {
		if item.Title == "Afghanistan - Reisewarnung" {
			foundAfghan = true
			if item.Summary != "Do not travel to Afghanistan." {
				t.Errorf("unexpected summary: %s", item.Summary)
			}
			if item.Published != "2026-01-15" {
				t.Errorf("unexpected published: %s", item.Published)
			}
		}
	}
	if !foundAfghan {
		t.Error("did not find Afghanistan entry")
	}
}

func TestParseGermanAATravelWarningsEnvelope(t *testing.T) {
	body := []byte(`{"response": {
		"10": {
			"title": "Test Country",
			"country": "Test",
			"warning": "Be careful.",
			"lastChanged": "2026-03-01"
		}
	}}`)
	items, err := ParseGermanAATravelWarnings(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Test Country" {
		t.Errorf("unexpected title: %s", items[0].Title)
	}
}

func TestParseFCDOAtom(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
	<feed xmlns="http://www.w3.org/2005/Atom">
	  <title>FCDO Travel Advice</title>
	  <entry>
	    <title>Afghanistan travel advice</title>
	    <link rel="alternate" href="https://www.gov.uk/foreign-travel-advice/afghanistan"/>
	    <published>2026-01-10T12:00:00Z</published>
	    <summary>FCDO advises against all travel to Afghanistan.</summary>
	  </entry>
	</feed>`)
	items, err := ParseFCDOAtom(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Afghanistan travel advice" {
		t.Errorf("unexpected title: %s", items[0].Title)
	}
	if items[0].Link != "https://www.gov.uk/foreign-travel-advice/afghanistan" {
		t.Errorf("unexpected link: %s", items[0].Link)
	}
}
