// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesOutputs(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "registry.json")
	content := `[
	  {"type":"rss","feed_url":"https://invalid.example/feed","category":"cyber_advisory","source":{"source_id":"test","authority_name":"Test","country":"France","country_code":"FR","region":"Europe","authority_type":"cert","base_url":"https://invalid.example"}}
	]`
	if err := os.WriteFile(registryPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{
		"--registry", registryPath,
		"--output", filepath.Join(dir, "alerts.json"),
		"--filtered-output", filepath.Join(dir, "filtered.json"),
		"--state-output", filepath.Join(dir, "state.json"),
		"--source-health-output", filepath.Join(dir, "health.json"),
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"alerts.json", "filtered.json", "state.json", "health.json"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to be written: %v", path, err)
		}
	}
}
