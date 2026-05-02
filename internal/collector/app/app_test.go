// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		"--replacement-queue", filepath.Join(dir, "replacement.json"),
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"alerts.json", "filtered.json", "state.json", "health.json", "replacement.json"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to be written: %v", path, err)
		}
	}
}

func TestTimestampWriterPrefixesLines(t *testing.T) {
	var buf bytes.Buffer
	w := newTimestampWriter(&buf)
	if _, err := w.Write([]byte("line one\nline two\n")); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buf.String())
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 output lines, got %d: %q", len(lines), out)
	}
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("expected timestamp prefix, got %q", line)
		}
		if !strings.Contains(parts[0], "T") || !strings.HasSuffix(parts[0], "Z") {
			t.Fatalf("expected RFC3339 UTC timestamp prefix, got %q", parts[0])
		}
	}
}

func TestResetAgentOpsFilesRemovesDBAndSidecars(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentops.db")
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := resetAgentOpsFiles(dbPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
}
