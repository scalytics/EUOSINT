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
