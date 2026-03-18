// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
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

// DLQ (Dead Letter Queue) tracks sources that returned permanent errors
// (404, 410, DNS failure, etc.). Sources in the DLQ are skipped during
// normal collection and probed once per day to check if they've come back.
// Discovery uses the same file to find replacement feeds.
type DLQ struct {
	entries map[string]model.SourceReplacementCandidate
	mu      sync.RWMutex
}

// DLQRetryInterval is how often DLQ sources are re-probed (once per day).
const DLQRetryInterval = 7 * 24 * time.Hour

func ReadDLQ(path string) *DLQ {
	dlq := &DLQ{entries: make(map[string]model.SourceReplacementCandidate)}
	data, err := os.ReadFile(path)
	if err != nil {
		return dlq
	}
	var doc model.SourceReplacementDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return dlq
	}
	for _, entry := range doc.Sources {
		dlq.entries[entry.SourceID] = entry
	}
	return dlq
}

func (d *DLQ) Write(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	d.mu.RLock()
	sources := make([]model.SourceReplacementCandidate, 0, len(d.entries))
	for _, entry := range d.entries {
		sources = append(sources, entry)
	}
	d.mu.RUnlock()
	doc := model.SourceReplacementDocument{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Sources:     sources,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// ShouldSkip returns true if the source is in the DLQ and not yet due
// for its daily re-probe.
func (d *DLQ) ShouldSkip(sourceID string, now time.Time) bool {
	d.mu.RLock()
	entry, ok := d.entries[sourceID]
	d.mu.RUnlock()
	if !ok {
		return false
	}
	if entry.LastAttemptAt == "" {
		return true
	}
	lastAttempt, err := time.Parse(time.RFC3339, entry.LastAttemptAt)
	if err != nil {
		return true
	}
	return now.Sub(lastAttempt) < DLQRetryInterval
}

// DueForRetry returns true if the source is in the DLQ but its retry
// interval has elapsed so it should be probed this cycle.
func (d *DLQ) DueForRetry(sourceID string, now time.Time) bool {
	d.mu.RLock()
	entry, ok := d.entries[sourceID]
	d.mu.RUnlock()
	if !ok {
		return false
	}
	if entry.LastAttemptAt == "" {
		return false
	}
	lastAttempt, err := time.Parse(time.RFC3339, entry.LastAttemptAt)
	if err != nil {
		return false
	}
	return now.Sub(lastAttempt) >= DLQRetryInterval
}

// Add places a source into the DLQ.
func (d *DLQ) Add(entry model.SourceReplacementCandidate) {
	entry.LastAttemptAt = time.Now().UTC().Format(time.RFC3339)
	d.mu.Lock()
	d.entries[entry.SourceID] = entry
	d.mu.Unlock()
}

// Remove takes a source out of the DLQ (it came back).
func (d *DLQ) Remove(sourceID string) {
	d.mu.Lock()
	delete(d.entries, sourceID)
	d.mu.Unlock()
}

// UpdateAttempt refreshes LastAttemptAt after a failed re-probe.
func (d *DLQ) UpdateAttempt(sourceID string, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry, ok := d.entries[sourceID]; ok {
		entry.LastAttemptAt = now.UTC().Format(time.RFC3339)
		d.entries[sourceID] = entry
	}
}

// Len returns the number of sources in the DLQ.
func (d *DLQ) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}

// Entries returns all DLQ entries for reporting.
func (d *DLQ) Entries() []model.SourceReplacementCandidate {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]model.SourceReplacementCandidate, 0, len(d.entries))
	for _, entry := range d.entries {
		out = append(out, entry)
	}
	return out
}

// Has returns true if the source is in the DLQ.
func (d *DLQ) Has(sourceID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.entries[sourceID]
	return ok
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
