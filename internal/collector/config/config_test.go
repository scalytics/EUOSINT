// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.OutputPath == "" || cfg.RegistryPath == "" {
		t.Fatalf("default config should populate output and registry paths: %#v", cfg)
	}
	if cfg.MaxPerSource <= 0 {
		t.Fatalf("unexpected max per source %d", cfg.MaxPerSource)
	}
}

func TestLoadStopWordsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stop_words.json")
	content := `{"stop_words":["football","celebrity","grammy"]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	words := loadStopWords(path)
	if len(words) != 3 {
		t.Fatalf("expected 3 stop words, got %d: %v", len(words), words)
	}
	expected := map[string]bool{"football": true, "celebrity": true, "grammy": true}
	for _, w := range words {
		if !expected[w] {
			t.Fatalf("unexpected stop word: %q", w)
		}
	}
}

func TestLoadStopWordsMissingFileReturnsNil(t *testing.T) {
	words := loadStopWords("/nonexistent/path/stop_words.json")
	if words != nil {
		t.Fatalf("expected nil for missing file, got %v", words)
	}
}

func TestLoadStopWordsEmptyPath(t *testing.T) {
	words := loadStopWords("")
	if words != nil {
		t.Fatalf("expected nil for empty path, got %v", words)
	}
}

func TestLoadStopWordsShippedDefault(t *testing.T) {
	// Verify the shipped registry/stop_words.json loads correctly.
	path := filepath.Join("..", "..", "..", "registry", "stop_words.json")
	words := loadStopWords(path)
	if len(words) == 0 {
		t.Fatal("shipped stop_words.json should contain at least one term")
	}
	found := false
	for _, w := range words {
		if w == "football" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("shipped stop_words.json should contain 'football'")
	}
}

func TestDefaultNoisePolicyPath(t *testing.T) {
	cfg := Default()
	if cfg.NoisePolicyPath != "registry/noise_policy.json" {
		t.Fatalf("unexpected default noise policy path %q", cfg.NoisePolicyPath)
	}
}

func TestNoisePolicyPathFromEnv(t *testing.T) {
	t.Setenv("NOISE_POLICY_PATH", "/tmp/noise_policy.json")
	cfg := FromEnv()
	if cfg.NoisePolicyPath != "/tmp/noise_policy.json" {
		t.Fatalf("expected NOISE_POLICY_PATH override, got %q", cfg.NoisePolicyPath)
	}
}
