// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseFeedRSS(t *testing.T) {
	xml := `<?xml version="1.0"?><rss><channel><item><title>Alert One</title><link>https://example.com/1</link><pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate><description>Body</description><category>crime</category></item></channel></rss>`
	items := ParseFeed(xml)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Alert One" {
		t.Fatalf("unexpected title %q", items[0].Title)
	}
	if items[0].Link != "https://example.com/1" {
		t.Fatalf("unexpected link %q", items[0].Link)
	}
	if items[0].Summary != "Body" {
		t.Fatalf("unexpected summary %q", items[0].Summary)
	}
}

func TestParseFeedAtom(t *testing.T) {
	xml := `<?xml version="1.0"?><feed><entry><title>Entry One</title><link rel="alternate" href="https://example.com/a"/><updated>2026-01-02T03:04:05Z</updated><author><name>Ops</name></author><summary>Summary</summary><category term="cyber"/></entry></feed>`
	items := ParseFeed(xml)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Author != "Ops" {
		t.Fatalf("unexpected author %q", items[0].Author)
	}
	if items[0].Link != "https://example.com/a" {
		t.Fatalf("unexpected link %q", items[0].Link)
	}
	if len(items[0].Tags) != 1 || items[0].Tags[0] != "cyber" {
		t.Fatalf("unexpected tags %#v", items[0].Tags)
	}
}
