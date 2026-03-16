package config

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.OutputPath == "" || cfg.RegistryPath == "" {
		t.Fatalf("default config should populate output and registry paths: %#v", cfg)
	}
	if cfg.MaxPerSource <= 0 {
		t.Fatalf("unexpected max per source %d", cfg.MaxPerSource)
	}
}
