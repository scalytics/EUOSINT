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

// Cursors tracks the resume page for paginated sources that accumulate.
type Cursors map[string]int // sourceID → next page to fetch

func ReadCursors(path string) Cursors {
	data, err := os.ReadFile(path)
	if err != nil {
		return Cursors{}
	}
	var c Cursors
	if err := json.Unmarshal(data, &c); err != nil {
		return Cursors{}
	}
	return c
}

func WriteCursors(path string, c Cursors) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SourceBackoffEntry tracks consecutive fetch failures for a source.
type SourceBackoffEntry struct {
	ConsecutiveFailures int       `json:"consecutive_failures"`
	SkipUntil           time.Time `json:"skip_until"`
	LastError           string    `json:"last_error,omitempty"`
}

// SourceBackoff maps source IDs to their backoff state.
type SourceBackoff map[string]SourceBackoffEntry

// backoffDurations defines exponential backoff tiers after N consecutive failures.
// After 3 failures: 1h → 6h → 24h → 7d (capped).
var backoffDurations = []time.Duration{
	0,              // 0 failures: no backoff
	0,              // 1 failure: no backoff
	0,              // 2 failures: no backoff
	1 * time.Hour,  // 3 failures
	6 * time.Hour,  // 4 failures
	24 * time.Hour, // 5 failures
	7 * 24 * time.Hour, // 6+ failures
}

func ReadSourceBackoff(path string) SourceBackoff {
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceBackoff{}
	}
	var b SourceBackoff
	if err := json.Unmarshal(data, &b); err != nil {
		return SourceBackoff{}
	}
	return b
}

func WriteSourceBackoff(path string, b SourceBackoff) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ShouldSkip returns true if the source should be skipped in this cycle.
func (b SourceBackoff) ShouldSkip(sourceID string, now time.Time) bool {
	entry, ok := b[sourceID]
	if !ok {
		return false
	}
	return entry.ConsecutiveFailures >= 3 && now.Before(entry.SkipUntil)
}

// RecordFailure increments the consecutive failure count and sets the
// next skip_until time based on exponential backoff.
func (b SourceBackoff) RecordFailure(sourceID string, now time.Time, errMsg string) {
	entry := b[sourceID]
	entry.ConsecutiveFailures++
	entry.LastError = errMsg
	tier := entry.ConsecutiveFailures
	if tier >= len(backoffDurations) {
		tier = len(backoffDurations) - 1
	}
	entry.SkipUntil = now.Add(backoffDurations[tier])
	b[sourceID] = entry
}

// RecordSuccess resets the backoff state for a source.
func (b SourceBackoff) RecordSuccess(sourceID string) {
	delete(b, sourceID)
}

// Reconcile merges current fetch results with previous state.
// accumulateSources lists source IDs where alerts carry forward across runs
// (paginated APIs like Interpol where each run only fetches a window).
func Reconcile(cfg config.Config, active []model.Alert, filtered []model.Alert, previous []model.Alert, now time.Time, accumulateSources map[string]bool) ([]model.Alert, []model.Alert, []model.Alert) {
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
		// Accumulating sources: carry forward active alerts not in this batch.
		if accumulateSources[prev.SourceID] && prev.Status == "active" {
			currentActive = append(currentActive, prev)
			presentByID[prev.AlertID] = struct{}{}
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
