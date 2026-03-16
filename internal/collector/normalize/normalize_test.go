package normalize

import (
	"testing"

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
