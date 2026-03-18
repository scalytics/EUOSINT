// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseFBIWanted(t *testing.T) {
	body := []byte(`{
		"total": 1137,
		"page": 1,
		"items": [
			{
				"uid": "abc123",
				"title": "JOHN DOE",
				"description": "<p>Conspiracy to commit wire fraud</p>",
				"url": "https://www.fbi.gov/wanted/fugitive/john-doe",
				"nationality": "American",
				"place_of_birth": "New York, New York",
				"aliases": ["JD", "Johnny"],
				"subjects": ["Cyber's Most Wanted"],
				"person_classification": "Main",
				"poster_classification": "default",
				"sex": "Male",
				"warning_message": "SHOULD BE CONSIDERED ARMED AND DANGEROUS",
				"reward_text": "Up to $100,000",
				"publication": "2025-01-15T00:00:00",
				"modified": "2026-03-01T12:00:00",
				"images": [{"thumb": "https://www.fbi.gov/image/thumb.jpg"}]
			},
			{
				"uid": "def456",
				"title": "",
				"url": ""
			}
		]
	}`)

	items, total, err := ParseFBIWanted(body)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1137 {
		t.Fatalf("expected total 1137, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (empty title skipped), got %d", len(items))
	}

	item := items[0]
	if item.Title != "JOHN DOE" {
		t.Fatalf("expected title 'JOHN DOE', got %q", item.Title)
	}
	if item.Link != "https://www.fbi.gov/wanted/fugitive/john-doe" {
		t.Fatalf("expected FBI link, got %q", item.Link)
	}
	if item.Published != "2026-03-01T12:00:00" {
		t.Fatalf("expected modified date, got %q", item.Published)
	}
	// Summary should contain stripped HTML description.
	if item.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	// Tags should include subjects, classification, sex, armed-dangerous.
	foundArmed := false
	for _, tag := range item.Tags {
		if tag == "armed-dangerous" {
			foundArmed = true
		}
	}
	if !foundArmed {
		t.Fatalf("expected armed-dangerous tag, got %v", item.Tags)
	}
}

func TestParseFBIWantedEmpty(t *testing.T) {
	body := []byte(`{"total": 0, "page": 1, "items": []}`)
	items, total, err := ParseFBIWanted(body)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}
