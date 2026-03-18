// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseTelegram(t *testing.T) {
	html := `<html><body>
<div class="tgme_widget_message_wrap" data-post="testchan/101">
  <div class="tgme_widget_message">
    <div class="tgme_widget_message_bubble">
      <div class="tgme_widget_message_text" dir="auto">Breaking: Missile strike reported in Kharkiv region, multiple explosions heard</div>
      <div class="tgme_widget_message_info"><time datetime="2026-03-15T10:30:00+00:00"></time></div>
    </div>
  </div>
</div>
<div class="tgme_widget_message_wrap" data-post="testchan/102">
  <div class="tgme_widget_message">
    <div class="tgme_widget_message_bubble">
      <div class="tgme_widget_message_text" dir="auto">Short</div>
      <div class="tgme_widget_message_info"><time datetime="2026-03-15T11:00:00+00:00"></time></div>
    </div>
  </div>
</div>
<div class="tgme_widget_message_wrap" data-post="testchan/103">
  <div class="tgme_widget_message">
    <div class="tgme_widget_message_bubble">
      <div class="tgme_widget_message_text" dir="auto">Ukrainian forces advance near Bakhmut, liberating two villages according to General Staff</div>
      <div class="tgme_widget_message_info"><time datetime="2026-03-15T12:00:00+00:00"></time></div>
    </div>
  </div>
</div>
</body></html>`

	items := ParseTelegram(html, "testchan")
	// "Short" should be filtered (< 8 chars)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].Link != "https://t.me/testchan/101" {
		t.Errorf("link = %q, want https://t.me/testchan/101", items[0].Link)
	}
	if items[0].Published != "2026-03-15T10:30:00+00:00" {
		t.Errorf("published = %q", items[0].Published)
	}
	if items[0].Title == "" {
		t.Error("title should not be empty")
	}

	if items[1].Link != "https://t.me/testchan/103" {
		t.Errorf("second link = %q", items[1].Link)
	}
}

func TestParseTelegramFallback(t *testing.T) {
	html := `<html><body>
<div class="tgme_widget_message_text" dir="auto">Heavy fighting reported near Donetsk airport perimeter today</div>
<div class="tgme_widget_message_text" dir="auto">Air raid sirens across multiple oblasts this morning</div>
</body></html>`

	items := ParseTelegram(html, "somechan")
	if len(items) != 2 {
		t.Fatalf("expected 2 items from fallback, got %d", len(items))
	}
	if items[0].Title == "" {
		t.Error("expected non-empty title")
	}
}
