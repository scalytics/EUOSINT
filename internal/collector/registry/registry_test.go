// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/sourcedb"
)

func TestLoadRegistryDeduplicatesAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	content := `[
	  {"type":"rss","feed_url":"https://one.example/feed","category":"cyber_advisory","source":{"source_id":"dup","authority_name":"One","country":"France","country_code":"fr","region":"Europe","authority_type":"cert","base_url":"https://one.example"}},
	  {"type":"rss","feed_url":"https://two.example/feed","category":"cyber_advisory","source":{"source_id":"dup","authority_name":"Two"}},
	  {"type":"html-list","feed_url":"https://three.example/list","category":"wanted_suspect","source":{"source_id":"three","authority_name":"Three"}}
	]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if sources[0].Source.SourceID != "dup" {
		t.Fatalf("unexpected source ordering %#v", sources)
	}
	if sources[0].Source.CountryCode != "FR" {
		t.Fatalf("expected normalized country code, got %q", sources[0].Source.CountryCode)
	}
}

func TestLoadRegistryFromSQLite(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "registry.json")
	dbPath := filepath.Join(dir, "sources.db")
	content := `[
	  {"type":"rss","feed_url":"https://one.example/feed","category":"cyber_advisory","source":{"source_id":"one-feed","authority_name":"Agency One","country":"France","country_code":"fr","region":"Europe","authority_type":"cert","base_url":"https://one.example"}}
	]`
	if err := os.WriteFile(jsonPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sourcedb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ImportRegistry(t.Context(), jsonPath); err != nil {
		t.Fatal(err)
	}

	sources, err := Load(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source from sqlite registry, got %d", len(sources))
	}
	if sources[0].Source.CountryCode != "FR" {
		t.Fatalf("expected normalized country code, got %q", sources[0].Source.CountryCode)
	}
}

func TestLoadRegistrySkipsKnownDeadFeedURLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	content := `[
	  {"type":"rss","feed_url":"https://www.actionfraud.police.uk/rss","category":"fraud_alert","source":{"source_id":"action-fraud-uk","authority_name":"Action Fraud UK"}},
	  {"type":"rss","feed_url":"https://example.test/live","category":"cyber_advisory","source":{"source_id":"live-source","authority_name":"Live Source"}}
	]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source after dead URL filtering, got %d", len(sources))
	}
	if sources[0].Source.SourceID != "live-source" {
		t.Fatalf("unexpected source after dead URL filtering: %q", sources[0].Source.SourceID)
	}
}
