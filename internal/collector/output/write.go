// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
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
	paths := []string{cfg.OutputPath, cfg.FilteredOutputPath, cfg.StateOutputPath, cfg.SourceHealthOutputPath, cfg.ReplacementQueuePath}
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
	doc := model.SourceHealthDocument{
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
		CriticalSourcePrefixes:  cfg.CriticalSourcePrefixes,
		FailOnCriticalSourceGap: cfg.FailOnCriticalSourceGap,
		TotalSources:            len(sourceHealth),
		SourcesOK:               countStatus(sourceHealth, "ok"),
		SourcesError:            countStatus(sourceHealth, "error"),
		DuplicateAudit:          duplicateAudit,
		ReplacementQueue:        replacementQueue,
		Sources:                 sourceHealth,
	}
	if doc.ReplacementQueue == nil {
		doc.ReplacementQueue = []model.SourceReplacementCandidate{}
	}
	if err := writeJSON(cfg.SourceHealthOutputPath, doc); err != nil {
		return err
	}
	queueDoc := model.SourceReplacementDocument{
		GeneratedAt: doc.GeneratedAt,
		Sources:     replacementQueue,
	}
	if queueDoc.Sources == nil {
		queueDoc.Sources = []model.SourceReplacementCandidate{}
	}
	return writeJSON(cfg.ReplacementQueuePath, queueDoc)
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
