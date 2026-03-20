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
	if cfg.StructuredDiscoveryIntervalHours != 168 {
		t.Fatalf("unexpected structured discovery interval default %d", cfg.StructuredDiscoveryIntervalHours)
	}
	if !cfg.DiscoverSocialEnabled {
		t.Fatal("expected social discovery to be enabled by default")
	}
	if cfg.XFetchPauseMS <= 0 {
		t.Fatalf("expected X fetch pause default > 0, got %d", cfg.XFetchPauseMS)
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
	if cfg.NoisePolicyBPath != "" || cfg.NoisePolicyBPercent != 0 {
		t.Fatalf("unexpected noise policy B defaults: path=%q percent=%d", cfg.NoisePolicyBPath, cfg.NoisePolicyBPercent)
	}
	if cfg.NoiseMetricsOutputPath != "public/noise-metrics.json" {
		t.Fatalf("unexpected default noise metrics path %q", cfg.NoiseMetricsOutputPath)
	}
	if cfg.ZoneBriefingsOutputPath != "public/zone-briefings.json" {
		t.Fatalf("unexpected default zone briefings path %q", cfg.ZoneBriefingsOutputPath)
	}
	if cfg.CountryBoundariesPath != "registry/geo/countries-adm0.geojson" {
		t.Fatalf("unexpected default country boundaries path %q", cfg.CountryBoundariesPath)
	}
}

func TestNoisePolicyPathFromEnv(t *testing.T) {
	t.Setenv("NOISE_POLICY_PATH", "/tmp/noise_policy.json")
	t.Setenv("NOISE_POLICY_B_PATH", "/tmp/noise_policy_b.json")
	t.Setenv("NOISE_POLICY_B_PERCENT", "25")
	t.Setenv("NOISE_METRICS_OUTPUT_PATH", "/tmp/noise_metrics.json")
	t.Setenv("ZONE_BRIEFINGS_OUTPUT_PATH", "/tmp/zone_briefings.json")
	t.Setenv("COUNTRY_BOUNDARIES_PATH", "/tmp/countries-adm0.geojson")
	cfg := FromEnv()
	if cfg.NoisePolicyPath != "/tmp/noise_policy.json" {
		t.Fatalf("expected NOISE_POLICY_PATH override, got %q", cfg.NoisePolicyPath)
	}
	if cfg.NoisePolicyBPath != "/tmp/noise_policy_b.json" {
		t.Fatalf("expected NOISE_POLICY_B_PATH override, got %q", cfg.NoisePolicyBPath)
	}
	if cfg.NoisePolicyBPercent != 25 {
		t.Fatalf("expected NOISE_POLICY_B_PERCENT override, got %d", cfg.NoisePolicyBPercent)
	}
	if cfg.NoiseMetricsOutputPath != "/tmp/noise_metrics.json" {
		t.Fatalf("expected NOISE_METRICS_OUTPUT_PATH override, got %q", cfg.NoiseMetricsOutputPath)
	}
	if cfg.ZoneBriefingsOutputPath != "/tmp/zone_briefings.json" {
		t.Fatalf("expected ZONE_BRIEFINGS_OUTPUT_PATH override, got %q", cfg.ZoneBriefingsOutputPath)
	}
	if cfg.CountryBoundariesPath != "/tmp/countries-adm0.geojson" {
		t.Fatalf("expected COUNTRY_BOUNDARIES_PATH override, got %q", cfg.CountryBoundariesPath)
	}
}
