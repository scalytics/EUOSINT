package registry

import (
	"os"
	"path/filepath"
	"testing"
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
