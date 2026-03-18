// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
)

func TestReconcileCarriesForwardAndRemoves(t *testing.T) {
	cfg := config.Default()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	active := []model.Alert{{AlertID: "a", FirstSeen: now.Add(-time.Hour).Format(time.RFC3339), LastSeen: now.Format(time.RFC3339)}}
	filtered := []model.Alert{{AlertID: "b", FirstSeen: now.Add(-2 * time.Hour).Format(time.RFC3339)}}
	previous := []model.Alert{
		{AlertID: "a", FirstSeen: now.Add(-24 * time.Hour).Format(time.RFC3339), Status: "active", LastSeen: now.Add(-time.Hour).Format(time.RFC3339)},
		{AlertID: "c", FirstSeen: now.Add(-6 * time.Hour).Format(time.RFC3339), Status: "active", LastSeen: now.Add(-6 * time.Hour).Format(time.RFC3339)},
		{AlertID: "d", FirstSeen: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339), Status: "active", LastSeen: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339)},
		{AlertID: "e", FirstSeen: now.Add(-45 * 24 * time.Hour).Format(time.RFC3339), Status: "active", LastSeen: now.Add(-45 * 24 * time.Hour).Format(time.RFC3339)},
	}

	currentActive, currentFiltered, fullState := Reconcile(cfg, active, filtered, previous, now, nil)
	if currentActive[0].FirstSeen != previous[0].FirstSeen {
		t.Fatalf("expected first_seen to carry forward, got %q", currentActive[0].FirstSeen)
	}
	if currentFiltered[0].Status != "filtered" {
		t.Fatalf("expected filtered status, got %q", currentFiltered[0].Status)
	}
	foundCCooldown := false
	foundDStale := false
	foundEArchived := false
	for _, alert := range fullState {
		if alert.AlertID == "c" && alert.Status == "cooldown" {
			foundCCooldown = true
		}
		if alert.AlertID == "d" && alert.Status == "stale" {
			foundDStale = true
		}
		if alert.AlertID == "e" && alert.Status == "archived" {
			foundEArchived = true
		}
	}
	if !foundCCooldown {
		t.Fatalf("expected recent missing alert 'c' to move to cooldown in state %#v", fullState)
	}
	if !foundDStale {
		t.Fatalf("expected missing alert 'd' to move to stale in state %#v", fullState)
	}
	if !foundEArchived {
		t.Fatalf("expected older missing alert 'e' to move to archived in state %#v", fullState)
	}
}

func TestReconcileAccumulateCarriesForward(t *testing.T) {
	cfg := config.Default()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	// This run only fetches alert "a" (new page).
	active := []model.Alert{{AlertID: "a", SourceID: "interpol-red"}}
	// Previous state has "b" from an earlier run — should carry forward.
	previous := []model.Alert{
		{AlertID: "b", SourceID: "interpol-red", Status: "active", FirstSeen: "2026-01-01T00:00:00Z", LastSeen: "2026-01-01T12:00:00Z"},
		{AlertID: "c", SourceID: "other-source", Status: "active", FirstSeen: "2025-12-01T00:00:00Z", LastSeen: "2025-12-01T12:00:00Z"},
	}
	acc := map[string]bool{"interpol-red": true}

	currentActive, _, fullState := Reconcile(cfg, active, nil, previous, now, acc)

	foundB := false
	for _, a := range currentActive {
		if a.AlertID == "b" {
			foundB = true
		}
	}
	if !foundB {
		t.Fatalf("expected accumulated alert 'b' in currentActive, got %v", currentActive)
	}

	foundCArchived := false
	for _, a := range fullState {
		if a.AlertID == "c" && a.Status == "archived" {
			foundCArchived = true
		}
	}
	if !foundCArchived {
		t.Fatalf("expected non-accumulating alert 'c' to become archived in fullState")
	}
}
