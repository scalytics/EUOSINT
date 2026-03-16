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

func Write(cfg config.Config, active []model.Alert, filtered []model.Alert, state []model.Alert, sourceHealth []model.SourceHealthEntry, duplicateAudit model.DuplicateAudit) error {
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
	doc := model.SourceHealthDocument{
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
		CriticalSourcePrefixes:  cfg.CriticalSourcePrefixes,
		FailOnCriticalSourceGap: cfg.FailOnCriticalSourceGap,
		TotalSources:            len(sourceHealth),
		SourcesOK:               countStatus(sourceHealth, "ok"),
		SourcesError:            countStatus(sourceHealth, "error"),
		DuplicateAudit:          duplicateAudit,
		Sources:                 sourceHealth,
	}
	return writeJSON(cfg.SourceHealthOutputPath, doc)
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
