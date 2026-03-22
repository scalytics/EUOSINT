package sourcedb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/collector/parse"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sources.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestUCDPEventKeyPrefersLink(t *testing.T) {
	item := parse.UCDPItem{}
	item.Link = "https://ucdp.uu.se/exploratory?id=123"
	item.Title = "ignored"
	if got := UCDPEventKey(item); got != item.Link {
		t.Fatalf("expected link key, got %q", got)
	}
}

func TestUCDPEventKeyFallbackComposition(t *testing.T) {
	item := parse.UCDPItem{}
	item.Title = "Event Title"
	item.Published = "2026-03-20"
	item.Lat = 12.34
	item.Lng = 56.78
	item.SideA = "A"
	item.SideB = "B"
	const want = "Event Title|2026-03-20|12.34000|56.78000|A|B"
	if got := UCDPEventKey(item); got != want {
		t.Fatalf("unexpected fallback event key: got %q want %q", got, want)
	}
}

func TestReplaceAndLoadUCDPLensEventsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	items := []parse.UCDPItem{
		{
			FeedItem: parse.FeedItem{
				Title:     "Older",
				Link:      "https://ucdp.uu.se/exploratory?id=old",
				Published: "2026-01-01",
				Summary:   "older summary",
				Tags:      []string{"a", "b"},
				Lat:       1.1,
				Lng:       2.2,
			},
			Country: "Ukraine", CountryCode: "UA", SideA: "A", SideB: "B", Fatalities: 2,
		},
		{
			FeedItem: parse.FeedItem{
				Title:     "Newer",
				Link:      "https://ucdp.uu.se/exploratory?id=new",
				Published: "2026-02-01",
				Summary:   "new summary",
				Tags:      []string{"x"},
				Lat:       3.3,
				Lng:       4.4,
			},
			Country: "Ukraine", CountryCode: "UA", SideA: "A2", SideB: "B2", Fatalities: 5,
		},
	}
	if err := db.ReplaceUCDPLensEvents(ctx, " ukraine ", items); err != nil {
		t.Fatalf("replace events: %v", err)
	}
	got, err := db.LoadUCDPLensEvents(ctx, "ukraine")
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].Title != "Newer" {
		t.Fatalf("expected descending order by published, got first title %q", got[0].Title)
	}
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "x" {
		t.Fatalf("expected tags to round-trip, got %#v", got[0].Tags)
	}

	if err := db.ReplaceUCDPLensEvents(ctx, "ukraine", items[:1]); err != nil {
		t.Fatalf("replace events second pass: %v", err)
	}
	got, err = db.LoadUCDPLensEvents(ctx, "ukraine")
	if err != nil {
		t.Fatalf("load events after replace: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Older" {
		t.Fatalf("replace should clear prior rows; got %#v", got)
	}
}

func TestUpsertUCDPLensEventsUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	item := parse.UCDPItem{
		FeedItem: parse.FeedItem{
			Title:     "Event",
			Link:      "https://ucdp.uu.se/exploratory?id=same",
			Published: "2026-03-01",
			Summary:   "v1",
		},
		Fatalities: 1,
	}
	if err := db.UpsertUCDPLensEvents(ctx, "gaza", []parse.UCDPItem{item}); err != nil {
		t.Fatalf("upsert initial: %v", err)
	}
	item.Summary = "v2"
	item.Fatalities = 7
	if err := db.UpsertUCDPLensEvents(ctx, "gaza", []parse.UCDPItem{item}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	got, err := db.LoadUCDPLensEvents(ctx, "gaza")
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one merged event, got %d", len(got))
	}
	if got[0].Summary != "v2" || got[0].Fatalities != 7 {
		t.Fatalf("upsert should update existing row, got summary=%q fatalities=%d", got[0].Summary, got[0].Fatalities)
	}
}

func TestUCDPLensStateRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if _, ok, err := db.GetUCDPLensState(ctx, "sudan"); err != nil || ok {
		t.Fatalf("expected missing state on empty db, ok=%v err=%v", ok, err)
	}

	in := UCDPLensState{
		LensID:     " sudan ",
		Version:    "26.1",
		StartDate:  "1989-01-01",
		EndDate:    "2026-03-01",
		HeadHash:   "abc",
		TotalPages: 12,
		EventCount: 345,
	}
	if err := db.UpsertUCDPLensState(ctx, in); err != nil {
		t.Fatalf("upsert state: %v", err)
	}
	got, ok, err := db.GetUCDPLensState(ctx, "sudan")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if !ok {
		t.Fatal("expected state to exist")
	}
	if got.LensID != "sudan" || got.Version != "26.1" || got.EventCount != 345 {
		t.Fatalf("unexpected state: %#v", got)
	}
	if got.RefreshedAt == "" {
		t.Fatal("expected refreshed_at auto-populated")
	}
}
