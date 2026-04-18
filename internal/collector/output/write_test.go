// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/model"
)

func TestWriteOutputs(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.OutputPath = filepath.Join(dir, "alerts.json")
	cfg.FilteredOutputPath = filepath.Join(dir, "filtered.json")
	cfg.StateOutputPath = filepath.Join(dir, "state.json")
	cfg.SourceHealthOutputPath = filepath.Join(dir, "health.json")
	cfg.ZoneBriefingsOutputPath = filepath.Join(dir, "zone-briefings.json")
	cfg.ReplacementQueuePath = filepath.Join(dir, "replacement.json")

	err := Write(cfg, []model.Alert{{AlertID: "a"}}, []model.Alert{{AlertID: "b"}}, []model.Alert{{AlertID: "c"}}, []model.SourceHealthEntry{{SourceID: "s", Status: "ok"}}, model.DuplicateAudit{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cfg.OutputPath, cfg.FilteredOutputPath, cfg.StateOutputPath, cfg.SourceHealthOutputPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected output file %s: %v", path, err)
		}
	}

	if err := WriteZoneBriefings(cfg.ZoneBriefingsOutputPath, []model.ZoneBriefingRecord{{LensID: "gaza", Title: "Gaza", Source: "UCDP GED"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.ZoneBriefingsOutputPath); err != nil {
		t.Fatalf("expected output file %s: %v", cfg.ZoneBriefingsOutputPath, err)
	}
}
