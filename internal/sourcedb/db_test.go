// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package sourcedb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/model"
)

func TestImportAndExportRegistry(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	dbPath := filepath.Join(dir, "sources.db")
	content := `[
	  {"type":"rss","feed_url":"https://one.example/feed","category":"cyber_advisory","region_tag":"EU","source":{"source_id":"agency-one-feed","authority_name":"Agency One","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://one.example"}},
	  {"type":"rss","feed_url":"https://one.example/alerts","category":"public_appeal","region_tag":"EU","source":{"source_id":"agency-one-alerts","authority_name":"Agency One","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://one.example"}}
	]`
	if err := os.WriteFile(registryPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.ImportRegistry(context.Background(), registryPath); err != nil {
		t.Fatal(err)
	}
	sources, err := db.LoadActiveSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 active sources, got %d", len(sources))
	}

	exportPath := filepath.Join(dir, "exported.json")
	if err := db.ExportRegistry(context.Background(), exportPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("expected exported registry file: %v", err)
	}
}

func TestDeactivateSourcesRemovesThemFromActiveLoad(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	dbPath := filepath.Join(dir, "sources.db")
	content := `[
	  {"type":"rss","feed_url":"https://one.example/feed","category":"cyber_advisory","source":{"source_id":"agency-one-feed","authority_name":"Agency One","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://one.example"}}
	]`
	if err := os.WriteFile(registryPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ImportRegistry(context.Background(), registryPath); err != nil {
		t.Fatal(err)
	}
	if err := db.DeactivateSources(context.Background(), map[string]string{
		"agency-one-feed": "fetch https://one.example/feed: status 404",
	}); err != nil {
		t.Fatal(err)
	}
	sources, err := db.LoadActiveSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("expected deactivated source to be removed from active load, got %d", len(sources))
	}
}

func TestScopeAndCurationMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	dbPath := filepath.Join(dir, "sources.db")
	content := `[
	  {"type":"rss","feed_url":"https://example.test/feed","category":"wanted_suspect","source_quality":0.97,"promotion_status":"active","source":{"source_id":"europol","authority_name":"Europol","country":"Netherlands","country_code":"NL","region":"Europe","authority_type":"police","base_url":"https://www.europol.europa.eu","scope":"supranational","level":"supranational","mission_tags":["organized_crime","wanted_suspect"],"operational_relevance":0.98,"is_curated":true,"is_high_value":true,"language_code":"en"}}
	]`
	if err := os.WriteFile(registryPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ImportRegistry(context.Background(), registryPath); err != nil {
		t.Fatal(err)
	}

	sources, err := db.LoadActiveSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 active source, got %d", len(sources))
	}
	if sources[0].Source.Scope != "supranational" {
		t.Fatalf("expected scope to round-trip, got %q", sources[0].Source.Scope)
	}
	if !sources[0].Source.IsCurated || !sources[0].Source.IsHighValue {
		t.Fatalf("expected curated/high-value flags to round-trip: %#v", sources[0].Source)
	}
	if sources[0].Source.LanguageCode != "en" {
		t.Fatalf("expected language code to round-trip, got %q", sources[0].Source.LanguageCode)
	}
	if sources[0].Source.Level != "supranational" {
		t.Fatalf("expected level to round-trip, got %q", sources[0].Source.Level)
	}
	if sources[0].PromotionStatus != "active" {
		t.Fatalf("expected promotion_status to round-trip, got %q", sources[0].PromotionStatus)
	}
	if sources[0].SourceQuality != 0.97 {
		t.Fatalf("expected source_quality to round-trip, got %v", sources[0].SourceQuality)
	}
	if len(sources[0].Source.MissionTags) != 2 {
		t.Fatalf("expected mission tags to round-trip, got %#v", sources[0].Source.MissionTags)
	}
}

func TestMergeRegistryAddsCuratedSeedWithoutReplacingExistingSources(t *testing.T) {
	dir := t.TempDir()
	baseRegistryPath := filepath.Join(dir, "base.json")
	seedRegistryPath := filepath.Join(dir, "seed.json")
	dbPath := filepath.Join(dir, "sources.db")

	base := `[
	  {"type":"rss","feed_url":"https://example.test/base","category":"cyber_advisory","source":{"source_id":"base-source","authority_name":"Base Source","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://example.test"}}
	]`
	seed := `[
	  {"type":"rss","feed_url":"https://example.test/seed","category":"wanted_suspect","source":{"source_id":"seed-source","authority_name":"Seed Source","country":"United States","country_code":"US","region":"North America","authority_type":"police","base_url":"https://example.test","scope":"national","is_curated":true,"is_high_value":true,"language_code":"en"}}
	]`
	if err := os.WriteFile(baseRegistryPath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seedRegistryPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ImportRegistry(context.Background(), baseRegistryPath); err != nil {
		t.Fatal(err)
	}
	if err := db.MergeRegistry(context.Background(), seedRegistryPath); err != nil {
		t.Fatal(err)
	}

	sources, err := db.LoadActiveSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected merged source count 2, got %d", len(sources))
	}
}

func TestSaveAndLoadAlertsReplacesMaterializedStateWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	firstSeen := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	lastSeen := time.Date(2026, 3, 16, 10, 15, 0, 0, time.UTC).Format(time.RFC3339)
	alerts := []model.Alert{
		{
			AlertID:      "alpha",
			SourceID:     "source-one",
			Title:        "Alpha alert",
			CanonicalURL: "https://example.test/a",
			FirstSeen:    firstSeen,
			LastSeen:     lastSeen,
			Status:       "active",
			Category:     "cyber_advisory",
			Severity:     "high",
			RegionTag:    "EU",
			Source: model.SourceMetadata{
				SourceID:      "source-one",
				AuthorityName: "Source One",
				Country:       "France",
				CountryCode:   "FR",
				Region:        "Europe",
				AuthorityType: "cert",
				BaseURL:       "https://example.test",
			},
		},
	}
	if err := db.SaveAlerts(context.Background(), alerts); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAlerts(context.Background(), alerts); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.LoadAlerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected exactly 1 persisted alert, got %d", len(loaded))
	}
	if loaded[0].AlertID != "alpha" {
		t.Fatalf("expected alert alpha, got %q", loaded[0].AlertID)
	}
	if loaded[0].FirstSeen != firstSeen {
		t.Fatalf("expected first_seen %q, got %q", firstSeen, loaded[0].FirstSeen)
	}
}

func TestUpsertSourceCandidatesDeduplicatesByURL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inputs := []CandidateInput{
		{
			DiscoveredURL: "https://example.test/feed",
			DiscoveredVia: "first.org",
			Status:        "candidate",
			Category:      "cyber_advisory",
			AuthorityType: "cert",
			Country:       "France",
			CountryCode:   "FR",
			Notes:         "Agency One",
		},
		{
			DiscoveredURL: "https://example.test/feed",
			DiscoveredVia: "replacement-queue",
			Status:        "candidate",
			Category:      "public_appeal",
			AuthorityType: "police",
			Country:       "France",
			CountryCode:   "FR",
			Notes:         "Agency One Revised",
		},
	}
	if err := db.UpsertSourceCandidates(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}

	row := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*), discovered_via, category, authority_type, notes FROM source_candidates WHERE discovered_url = ?`, "https://example.test/feed")
	var count int
	var discoveredVia, category, authorityType, notes string
	if err := row.Scan(&count, &discoveredVia, &category, &authorityType, &notes); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 candidate row, got %d", count)
	}
	if discoveredVia != "replacement-queue" || category != "public_appeal" || authorityType != "police" || notes != "Agency One Revised" {
		t.Fatalf("unexpected candidate row values: via=%q category=%q authority=%q notes=%q", discoveredVia, category, authorityType, notes)
	}
}
