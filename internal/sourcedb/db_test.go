// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
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

func TestLoadActiveSourcesAutoMigratesOlderDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate an older DB that predates the hygiene columns added later.
	for _, stmt := range []string{
		`CREATE TABLE agencies (
			id TEXT PRIMARY KEY,
			authority_name TEXT NOT NULL,
			country TEXT NOT NULL DEFAULT '',
			country_code TEXT NOT NULL DEFAULT '',
			region TEXT NOT NULL DEFAULT '',
			authority_type TEXT NOT NULL DEFAULT '',
			base_url TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL DEFAULT 'national',
			jurisdiction_name TEXT NOT NULL DEFAULT '',
			parent_agency_id TEXT NOT NULL DEFAULT '',
			is_curated INTEGER NOT NULL DEFAULT 0,
			is_high_value INTEGER NOT NULL DEFAULT 0,
			language_code TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE sources (
			id TEXT PRIMARY KEY,
			agency_id TEXT NOT NULL,
			type TEXT NOT NULL,
			fetch_mode TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			follow_redirects INTEGER NOT NULL DEFAULT 1,
			feed_url TEXT NOT NULL,
			feed_urls_json TEXT NOT NULL DEFAULT '[]',
			category TEXT NOT NULL DEFAULT '',
			region_tag TEXT NOT NULL DEFAULT '',
			lat REAL NOT NULL DEFAULT 0,
			lng REAL NOT NULL DEFAULT 0,
			max_items INTEGER NOT NULL DEFAULT 20,
			include_keywords_json TEXT NOT NULL DEFAULT '[]',
			exclude_keywords_json TEXT NOT NULL DEFAULT '[]',
			reporting_label TEXT NOT NULL DEFAULT '',
			reporting_url TEXT NOT NULL DEFAULT '',
			reporting_phone TEXT NOT NULL DEFAULT '',
			reporting_notes TEXT NOT NULL DEFAULT '',
			last_http_status INTEGER NOT NULL DEFAULT 0,
			last_ok_at TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			last_error_class TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE source_categories (
			source_id TEXT NOT NULL,
			category TEXT NOT NULL,
			PRIMARY KEY (source_id, category)
		)`,
		`CREATE TABLE source_candidates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			discovered_url TEXT NOT NULL,
			discovered_via TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'candidate',
			language_code TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			authority_type TEXT NOT NULL DEFAULT '',
			country TEXT NOT NULL DEFAULT '',
			country_code TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			checked_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(discovered_url)
		)`,
		`CREATE TABLE agencies_fts (
			agency_id TEXT NOT NULL,
			name TEXT NOT NULL,
			aliases TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE agency_category_coverage (
			agency_id TEXT NOT NULL,
			category TEXT NOT NULL,
			PRIMARY KEY (agency_id, category)
		)`,
	} {
		if _, err := db.sql.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed old schema: %v", err)
		}
	}

	if _, err := db.sql.ExecContext(context.Background(), `
INSERT INTO agencies (id, authority_name, country, country_code, region, authority_type, base_url, scope)
VALUES ('agency-one', 'Agency One', 'France', 'FR', 'Europe', 'cert', 'https://one.example', 'national')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(context.Background(), `
INSERT INTO sources (id, agency_id, type, status, feed_url, category)
VALUES ('agency-one-feed', 'agency-one', 'rss', 'active', 'https://one.example/feed', 'cyber_advisory')`); err != nil {
		t.Fatal(err)
	}

	sources, err := db.LoadActiveSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 active source after auto-migration, got %d", len(sources))
	}
	if sources[0].PromotionStatus != "active" {
		t.Fatalf("expected promotion_status backfill to active, got %q", sources[0].PromotionStatus)
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

func TestMergeRegistryRejectsExistingSource(t *testing.T) {
	dir := t.TempDir()
	baseRegistryPath := filepath.Join(dir, "base.json")
	seedRegistryPath := filepath.Join(dir, "seed.json")
	dbPath := filepath.Join(dir, "sources.db")

	base := `[
	  {"type":"html-list","feed_url":"https://www.hotosm.org/projects/","category":"humanitarian_tasking","source":{"source_id":"hot-tasking","authority_name":"Humanitarian OpenStreetMap Team","country":"International","country_code":"INT","region":"International","authority_type":"public_safety_program","base_url":"https://www.hotosm.org"}}
	]`
	seed := `[
	  {"type":"html-list","feed_url":"https://www.hotosm.org/projects/","category":"humanitarian_tasking","promotion_status":"rejected","rejection_reason":"JS-rendered navigation page, not a stable incident/tasking feed","source":{"source_id":"hot-tasking","authority_name":"Humanitarian OpenStreetMap Team","country":"International","country_code":"INT","region":"International","authority_type":"public_safety_program","base_url":"https://www.hotosm.org"}}
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
	if len(sources) != 0 {
		t.Fatalf("expected rejected source to be removed from active load, got %d", len(sources))
	}

	var promotionStatus, rejectionReason string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT promotion_status, rejection_reason FROM sources WHERE id = 'hot-tasking'`).Scan(&promotionStatus, &rejectionReason); err != nil {
		t.Fatal(err)
	}
	if promotionStatus != "rejected" {
		t.Fatalf("expected promotion_status rejected, got %q", promotionStatus)
	}
	if rejectionReason == "" {
		t.Fatal("expected rejection_reason to be persisted")
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

func TestCorpusScoresDistinguishesDistinctiveFromCommon(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create a corpus with many similar alerts + one distinctive one.
	alerts := []model.Alert{
		{AlertID: "common-1", SourceID: "src", Status: "active", Title: "Security advisory update issued", Category: "cyber_advisory", Severity: "medium", FirstSeen: now, LastSeen: now, Source: model.SourceMetadata{AuthorityName: "CERT", Country: "Germany", CountryCode: "DE"}},
		{AlertID: "common-2", SourceID: "src", Status: "active", Title: "Security advisory update released", Category: "cyber_advisory", Severity: "medium", FirstSeen: now, LastSeen: now, Source: model.SourceMetadata{AuthorityName: "CERT", Country: "Germany", CountryCode: "DE"}},
		{AlertID: "common-3", SourceID: "src", Status: "active", Title: "Security advisory update published", Category: "cyber_advisory", Severity: "medium", FirstSeen: now, LastSeen: now, Source: model.SourceMetadata{AuthorityName: "CERT", Country: "Germany", CountryCode: "DE"}},
		{AlertID: "common-4", SourceID: "src", Status: "active", Title: "Security advisory update notification", Category: "cyber_advisory", Severity: "medium", FirstSeen: now, LastSeen: now, Source: model.SourceMetadata{AuthorityName: "CERT", Country: "Germany", CountryCode: "DE"}},
		{AlertID: "distinctive-1", SourceID: "src", Status: "active", Title: "Ransomware gang exploits zero-day vulnerability in critical infrastructure", Category: "cyber_advisory", Severity: "critical", FirstSeen: now, LastSeen: now, Source: model.SourceMetadata{AuthorityName: "CERT", Country: "Germany", CountryCode: "DE"}},
	}

	if err := db.SaveAlerts(ctx, alerts); err != nil {
		t.Fatal(err)
	}

	scores, err := db.CorpusScores(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The distinctive alert should score higher than the common ones.
	distinctiveScore, ok := scores["distinctive-1"]
	if !ok {
		t.Fatal("expected distinctive-1 to have a corpus score")
	}

	for _, id := range []string{"common-1", "common-2", "common-3", "common-4"} {
		commonScore, ok := scores[id]
		if !ok {
			continue // may not have scored if all terms are stopwords
		}
		if distinctiveScore < commonScore {
			t.Errorf("expected distinctive alert (%.3f) to score higher than %s (%.3f)",
				distinctiveScore, id, commonScore)
		}
	}
}
