// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
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
		{AlertID: "c", FirstSeen: now.Add(-24 * time.Hour).Format(time.RFC3339), Status: "active", LastSeen: now.Add(-time.Hour).Format(time.RFC3339)},
	}

	currentActive, currentFiltered, fullState := Reconcile(cfg, active, filtered, previous, now)
	if currentActive[0].FirstSeen != previous[0].FirstSeen {
		t.Fatalf("expected first_seen to carry forward, got %q", currentActive[0].FirstSeen)
	}
	if currentFiltered[0].Status != "filtered" {
		t.Fatalf("expected filtered status, got %q", currentFiltered[0].Status)
	}
	foundRemoved := false
	for _, alert := range fullState {
		if alert.AlertID == "c" && alert.Status == "removed" {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Fatalf("expected removed alert in state %#v", fullState)
	}
}
