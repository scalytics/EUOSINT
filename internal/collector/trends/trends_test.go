// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package trends

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/scalytics/euosint/internal/collector/model"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestExtractSignificantTerms(t *testing.T) {
	terms := extractSignificantTerms("Ransomware attack disrupts hospital systems across EU")
	want := map[string]bool{"ransomware": true, "attack": true, "disrupts": true, "hospital": true, "systems": true, "across": true}
	for _, term := range terms {
		if !want[term] {
			t.Errorf("unexpected term %q", term)
		}
	}
	if len(terms) == 0 {
		t.Fatal("expected some terms")
	}
	// "EU" should be filtered (too short)
	for _, term := range terms {
		if term == "eu" {
			t.Error("expected 'eu' to be filtered (too short)")
		}
	}
}

func TestRecordAndDetectSpikes(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	d := New(db)
	if err := d.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// Simulate 7 days of baseline with 1-2 "ransomware" alerts per day.
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	for i := 7; i > 0; i-- {
		day := now.AddDate(0, 0, -i)
		alerts := []model.Alert{
			{Title: "Ransomware advisory issued", Category: "cyber_advisory", RegionTag: "EU", Severity: "high"},
		}
		if err := d.Record(ctx, alerts, day); err != nil {
			t.Fatal(err)
		}
	}

	// Today: spike with 5 ransomware alerts.
	todayAlerts := []model.Alert{
		{Title: "Ransomware attack on hospital network", Category: "cyber_advisory", RegionTag: "EU", Severity: "critical"},
		{Title: "Ransomware gang targets critical infrastructure", Category: "cyber_advisory", RegionTag: "EU", Severity: "critical"},
		{Title: "New ransomware variant detected in EU systems", Category: "cyber_advisory", RegionTag: "EU", Severity: "high"},
		{Title: "Ransomware incident response guide updated", Category: "cyber_advisory", RegionTag: "EU", Severity: "high"},
		{Title: "Major ransomware campaign linked to state actor", Category: "cyber_advisory", RegionTag: "EU", Severity: "critical"},
	}
	if err := d.Record(ctx, todayAlerts, now); err != nil {
		t.Fatal(err)
	}

	spikes, err := d.DetectSpikes(ctx, now, 7, 3.0, 3)
	if err != nil {
		t.Fatal(err)
	}

	// "ransomware" should spike (5 today vs ~1/day average).
	found := false
	for _, s := range spikes {
		if s.Term == "ransomware" {
			found = true
			if s.TodayCount != 5 {
				t.Errorf("expected today_count=5, got %d", s.TodayCount)
			}
			if s.Ratio < 3 {
				t.Errorf("expected ratio >= 3, got %.1f", s.Ratio)
			}
		}
	}
	if !found {
		t.Error("expected ransomware spike to be detected")
	}
}

func TestBuildHints(t *testing.T) {
	spikes := []Spike{
		{Term: "ransomware", Category: "cyber_advisory", Region: "EU", TodayCount: 10, AvgCount: 2, Ratio: 5},
		{Term: "hospital", Category: "cyber_advisory", Region: "EU", TodayCount: 5, AvgCount: 1, Ratio: 5},
		{Term: "trafficking", Category: "public_appeal", Region: "EU", TodayCount: 4, AvgCount: 1, Ratio: 4},
	}
	hints := BuildHints(spikes)
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints (2 category+region groups), got %d", len(hints))
	}
}

func TestHintsToCandidates(t *testing.T) {
	hints := []DiscoveryHint{
		{Terms: []string{"ransomware", "hospital"}, Category: "cyber_advisory", Region: "EU", Reason: "ransomware 5x above avg"},
	}
	candidates := HintsToCandidates(hints)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	c := candidates[0]
	if c.Category != "cyber_advisory" {
		t.Errorf("expected category cyber_advisory, got %q", c.Category)
	}
	if c.AuthorityType != "cert" {
		t.Errorf("expected authority_type cert, got %q", c.AuthorityType)
	}
	if c.Notes == "" {
		t.Error("expected notes with trend reason")
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	d := New(db)
	if err := d.Init(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	// Record data 60 days ago.
	old := now.AddDate(0, 0, -60)
	alerts := []model.Alert{
		{Title: "Old ransomware alert", Category: "cyber_advisory", RegionTag: "EU", Severity: "high"},
	}
	if err := d.Record(ctx, alerts, old); err != nil {
		t.Fatal(err)
	}

	// Prune with 30 day retention.
	if err := d.Prune(ctx, now, 30); err != nil {
		t.Fatal(err)
	}

	// Verify old data is gone.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM term_trends`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after prune, got %d", count)
	}
}

func TestAnnotateSpikesWithSamples(t *testing.T) {
	spikes := []Spike{
		{Term: "ransomware", Category: "cyber_advisory"},
	}
	alerts := []model.Alert{
		{Title: "Ransomware attack on hospital network"},
	}
	AnnotateSpikesWithSamples(spikes, alerts)
	if spikes[0].SampleTitle != "Ransomware attack on hospital network" {
		t.Errorf("expected sample title to be set, got %q", spikes[0].SampleTitle)
	}
}

func TestInfoAlertsSkipped(t *testing.T) {
	terms := map[string]bool{}
	alerts := []model.Alert{
		{Title: "Ransomware attack", Category: "cyber_advisory", Severity: "info"},
		{Title: "Workshop on cybersecurity", Category: "informational", Severity: "medium"},
	}
	ctx := context.Background()
	db := testDB(t)
	d := New(db)
	if err := d.Init(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	if err := d.Record(ctx, alerts, now); err != nil {
		t.Fatal(err)
	}
	// Should have no terms recorded.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM term_trends`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows for info-only alerts, got %d", count)
	}
	_ = terms
}
