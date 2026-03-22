package sourcedb

import (
	"context"
	"testing"
)

func TestZoneBriefLLMRoundTripAndPreserveNonEmptyFields(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if _, ok, err := db.GetZoneBriefLLM(ctx, "625"); err != nil || ok {
		t.Fatalf("expected no row initially, ok=%v err=%v", ok, err)
	}

	in := ZoneBriefLLM{
		CountryID:         "625",
		Title:             "Sudan",
		HistoricalSummary: "Historic summary",
		CurrentAnalysis:   "Current analysis",
	}
	if err := db.UpsertZoneBriefLLM(ctx, in); err != nil {
		t.Fatalf("upsert zone brief llm: %v", err)
	}
	first, ok, err := db.GetZoneBriefLLM(ctx, "625")
	if err != nil {
		t.Fatalf("get zone brief llm first: %v", err)
	}
	if !ok {
		t.Fatal("expected zone brief llm row")
	}
	if first.HistoricalUpdatedAt == "" || first.AnalysisUpdatedAt == "" {
		t.Fatalf("expected auto timestamps, got %#v", first)
	}

	// Empty narrative fields must not erase previously persisted text.
	if err := db.UpsertZoneBriefLLM(ctx, ZoneBriefLLM{
		CountryID: "625",
		Title:     "Sudan Updated",
	}); err != nil {
		t.Fatalf("upsert title-only zone brief llm: %v", err)
	}
	second, ok, err := db.GetZoneBriefLLM(ctx, "625")
	if err != nil {
		t.Fatalf("get zone brief llm second: %v", err)
	}
	if !ok {
		t.Fatal("expected zone brief llm row on second fetch")
	}
	if second.Title != "Sudan Updated" {
		t.Fatalf("expected updated title, got %q", second.Title)
	}
	if second.HistoricalSummary != "Historic summary" || second.CurrentAnalysis != "Current analysis" {
		t.Fatalf("expected non-empty narratives preserved, got %#v", second)
	}
	if second.HistoricalUpdatedAt != first.HistoricalUpdatedAt || second.AnalysisUpdatedAt != first.AnalysisUpdatedAt {
		t.Fatalf("expected narrative timestamps preserved, first=%#v second=%#v", first, second)
	}
}

func TestZoneBriefLLMBlankCountryIsNoop(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.UpsertZoneBriefLLM(ctx, ZoneBriefLLM{CountryID: " "}); err != nil {
		t.Fatalf("blank country should noop, err=%v", err)
	}
	if _, ok, err := db.GetZoneBriefLLM(ctx, " "); err != nil || ok {
		t.Fatalf("blank get should miss, ok=%v err=%v", ok, err)
	}
}
