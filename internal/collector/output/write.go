// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
)

func Write(cfg config.Config, active []model.Alert, filtered []model.Alert, state []model.Alert, sourceHealth []model.SourceHealthEntry, duplicateAudit model.DuplicateAudit, replacementQueue []model.SourceReplacementCandidate) error {
	return WriteWithTotal(cfg, active, filtered, state, sourceHealth, duplicateAudit, replacementQueue, 0)
}

// WriteWithTotal is like Write but accepts an explicit totalRegistrySources
// override. When > 0, total_sources in source-health.json reflects the full
// registry count rather than len(sourceHealth) — keeping the UI stable during
// progress snapshots mid-sweep.
func WriteWithTotal(cfg config.Config, active []model.Alert, filtered []model.Alert, state []model.Alert, sourceHealth []model.SourceHealthEntry, duplicateAudit model.DuplicateAudit, replacementQueue []model.SourceReplacementCandidate, totalRegistrySources int) error {
	paths := []string{cfg.OutputPath, cfg.FilteredOutputPath, cfg.StateOutputPath, cfg.SourceHealthOutputPath}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	}
	if err := writeJSON(cfg.OutputPath, active); err != nil {
		return err
	}
	if err := writeJSON(cfg.FilteredOutputPath, filtered); err != nil {
		return err
	}
	if err := writeJSON(cfg.StateOutputPath, state); err != nil {
		return err
	}
	totalSources := len(sourceHealth)
	if totalRegistrySources > totalSources {
		totalSources = totalRegistrySources
	}
	doc := model.SourceHealthDocument{
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
		CriticalSourcePrefixes:  cfg.CriticalSourcePrefixes,
		FailOnCriticalSourceGap: cfg.FailOnCriticalSourceGap,
		TotalSources:            totalSources,
		SourcesOK:               countStatus(sourceHealth, "ok"),
		SourcesError:            countStatus(sourceHealth, "error"),
		DuplicateAudit:          duplicateAudit,
		ReplacementQueue:        replacementQueue,
		Sources:                 sourceHealth,
	}
	if doc.ReplacementQueue == nil {
		doc.ReplacementQueue = []model.SourceReplacementCandidate{}
	}
	return writeJSON(cfg.SourceHealthOutputPath, doc)
}

func WriteZoneBriefings(path string, briefings []model.ZoneBriefingRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeJSON(path, briefings)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func countStatus(entries []model.SourceHealthEntry, status string) int {
	total := 0
	for _, entry := range entries {
		if entry.Status == status {
			total++
		}
	}
	return total
}
