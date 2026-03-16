// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"encoding/json"
	"os"
	"sort"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
)

func Read(path string) []model.Alert {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var alerts []model.Alert
	if err := json.Unmarshal(data, &alerts); err != nil {
		return nil
	}
	return alerts
}

func Reconcile(cfg config.Config, active []model.Alert, filtered []model.Alert, previous []model.Alert, now time.Time) ([]model.Alert, []model.Alert, []model.Alert) {
	nowISO := now.UTC().Format(time.RFC3339)
	retentionCutoff := now.Add(-time.Duration(cfg.RemovedRetentionDays) * 24 * time.Hour)
	previousByID := map[string]model.Alert{}
	presentByID := map[string]struct{}{}
	for _, alert := range previous {
		previousByID[alert.AlertID] = alert
	}
	for _, alert := range append(append([]model.Alert{}, active...), filtered...) {
		presentByID[alert.AlertID] = struct{}{}
	}

	currentActive := make([]model.Alert, 0, len(active))
	for _, alert := range active {
		if prev, ok := previousByID[alert.AlertID]; ok && prev.FirstSeen != "" {
			alert.FirstSeen = prev.FirstSeen
		}
		alert.Status = "active"
		alert.LastSeen = nowISO
		currentActive = append(currentActive, alert)
	}

	currentFiltered := make([]model.Alert, 0, len(filtered))
	for _, alert := range filtered {
		if prev, ok := previousByID[alert.AlertID]; ok && prev.FirstSeen != "" {
			alert.FirstSeen = prev.FirstSeen
		}
		alert.Status = "filtered"
		alert.LastSeen = nowISO
		currentFiltered = append(currentFiltered, alert)
	}

	removed := []model.Alert{}
	for _, prev := range previous {
		if _, ok := presentByID[prev.AlertID]; ok {
			continue
		}
		if prev.Status == "removed" {
			lastSeen, err := time.Parse(time.RFC3339, prev.LastSeen)
			if err == nil && !lastSeen.Before(retentionCutoff) {
				removed = append(removed, prev)
			}
			continue
		}
		if prev.Status == "filtered" {
			continue
		}
		prev.Status = "removed"
		prev.LastSeen = nowISO
		removed = append(removed, prev)
	}

	fullState := append(append(append([]model.Alert{}, currentActive...), currentFiltered...), removed...)
	sort.Slice(fullState, func(i, j int) bool { return fullState[i].LastSeen > fullState[j].LastSeen })
	return currentActive, currentFiltered, fullState
}
