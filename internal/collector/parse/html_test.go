// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseHTMLAnchors(t *testing.T) {
	body := `<html><body><a href="/wanted/1">Wanted Person</a><a href="#skip">Skip</a><a href="/wanted/1">Duplicate</a></body></html>`
	items := ParseHTMLAnchors(body, "https://agency.example.org/news")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Link != "https://agency.example.org/wanted/1" {
		t.Fatalf("unexpected link %q", items[0].Link)
	}
}

func TestParseHTMLAnchorsSkipsTemplateAndFilterNoise(t *testing.T) {
	body := `<html><body>
<a href="/projects/1">${item.title} ${item.url}</a>
<a href="/projects/2">Reset Filters</a>
<a href="/projects/3">Disaster Response in Sudan</a>
</body></html>`
	items := ParseHTMLAnchors(body, "https://agency.example.org/news")
	if len(items) != 1 {
		t.Fatalf("expected 1 item after filtering noise, got %d", len(items))
	}
	if items[0].Title != "Disaster Response in Sudan" {
		t.Fatalf("unexpected title %q", items[0].Title)
	}
}
