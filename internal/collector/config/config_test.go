// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

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
