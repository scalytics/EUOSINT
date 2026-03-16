// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
)

func TestDeduplicatePrefersHigherScore(t *testing.T) {
	alerts := []model.Alert{
		{Title: "A", CanonicalURL: "https://x", Triage: &model.Triage{RelevanceScore: 0.2}},
		{Title: "A", CanonicalURL: "https://x", Triage: &model.Triage{RelevanceScore: 0.8}},
	}
	deduped, _ := Deduplicate(alerts)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(deduped))
	}
	if deduped[0].Triage.RelevanceScore != 0.8 {
		t.Fatalf("expected highest score to win, got %.3f", deduped[0].Triage.RelevanceScore)
	}
}

func TestFilterActiveUsesMissingPersonThreshold(t *testing.T) {
	cfg := config.Default()
	cfg.IncidentRelevanceThreshold = 0.5
	cfg.MissingPersonRelevanceThreshold = 0.1
	alerts := []model.Alert{
		{Category: "missing_person", Triage: &model.Triage{RelevanceScore: 0.2}},
		{Category: "cyber_advisory", Triage: &model.Triage{RelevanceScore: 0.2}},
	}
	active, filtered := FilterActive(cfg, alerts)
	if len(active) != 1 || active[0].Category != "missing_person" {
		t.Fatalf("unexpected active alerts %#v", active)
	}
	if len(filtered) != 1 || filtered[0].Category != "cyber_advisory" {
		t.Fatalf("unexpected filtered alerts %#v", filtered)
	}
}

func TestInterpolAlertUsesNoticeCountryAndStableID(t *testing.T) {
	ctx := Context{Config: config.Default(), Now: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)}
	meta := model.RegistrySource{
		Type:      "interpol-yellow-json",
		Category:  "missing_person",
		RegionTag: "INT",
		Source: model.SourceMetadata{
			SourceID:      "interpol-yellow",
			AuthorityName: "INTERPOL Yellow Notices",
			Country:       "France",
			CountryCode:   "FR",
			Region:        "International",
			AuthorityType: "police",
			BaseURL:       "https://www.interpol.int",
		},
	}
	alert := InterpolAlert(ctx, meta, "2026-17351", "INTERPOL Yellow Notice: Jane Doe", "https://www.interpol.int/How-we-work/Notices/Yellow-Notices/View-Yellow-Notices#2026-17351", "DE", "INTERPOL Paris", []string{"DE"})
	if alert == nil {
		t.Fatal("expected interpol alert")
	}
	if alert.AlertID != "interpol-yellow:2026-17351" {
		t.Fatalf("expected stable interpol alert id, got %q", alert.AlertID)
	}
	if alert.Source.CountryCode != "DE" || alert.Source.Country != "Germany" {
		t.Fatalf("expected country mapping to Germany, got %#v", alert.Source)
	}
	if alert.Source.AuthorityName != "INTERPOL Yellow Notices" {
		t.Fatalf("expected source authority to remain INTERPOL, got %#v", alert.Source)
	}
}
