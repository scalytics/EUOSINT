// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
)

func TestWriteOutputs(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.OutputPath = filepath.Join(dir, "alerts.json")
	cfg.FilteredOutputPath = filepath.Join(dir, "filtered.json")
	cfg.StateOutputPath = filepath.Join(dir, "state.json")
	cfg.SourceHealthOutputPath = filepath.Join(dir, "health.json")

	err := Write(cfg, []model.Alert{{AlertID: "a"}}, []model.Alert{{AlertID: "b"}}, []model.Alert{{AlertID: "c"}}, []model.SourceHealthEntry{{SourceID: "s", Status: "ok"}}, model.DuplicateAudit{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cfg.OutputPath, cfg.FilteredOutputPath, cfg.StateOutputPath, cfg.SourceHealthOutputPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected output file %s: %v", path, err)
		}
	}
}
