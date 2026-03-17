// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
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

func TestLocalCrimeDownranked(t *testing.T) {
	cfg := config.Default()
	ctx := Context{Config: cfg, Now: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)}
	meta := model.RegistrySource{
		Type:     "rss",
		Category: "public_appeal",
		Source: model.SourceMetadata{
			SourceID:      "pj-pt",
			AuthorityName: "Polícia Judiciária",
			Country:       "Portugal",
			CountryCode:   "PT",
			Region:        "Europe",
			AuthorityType: "police",
		},
	}

	// Local crime: police raid on a mortuary — no cross-border significance.
	localItem := parse.FeedItem{
		Title:     "Operação Rigor Mortis – PJ realiza buscas em casa mortuária e em domicílios",
		Link:      "https://www.policiajudiciaria.pt/operacao-rigor-mortis/",
		Published: "2026-03-15T10:00:00Z",
		Summary:   "A Polícia Judiciária realizou buscas em casa mortuária. Autopsy fraud investigation.",
	}
	localAlert := RSSItem(ctx, meta, localItem)
	if localAlert == nil {
		t.Fatal("expected local crime alert to be normalized")
	}
	if localAlert.Triage.RelevanceScore >= cfg.IncidentRelevanceThreshold {
		t.Fatalf("expected local crime to be below threshold, got %.3f (threshold %.3f)",
			localAlert.Triage.RelevanceScore, cfg.IncidentRelevanceThreshold)
	}

	// Cross-border crime: Europol joint operation — should stay above threshold.
	crossBorderItem := parse.FeedItem{
		Title:     "Operação conjunta PJ-Europol — rede transnacional de tráfico desmantelada",
		Link:      "https://www.policiajudiciaria.pt/operacao-europol/",
		Published: "2026-03-15T10:00:00Z",
		Summary:   "Joint operation with Europol dismantled cross-border trafficking network. Drug seizure of 500 kg cocaine.",
	}
	crossBorderAlert := RSSItem(ctx, meta, crossBorderItem)
	if crossBorderAlert == nil {
		t.Fatal("expected cross-border alert to be normalized")
	}
	if crossBorderAlert.Triage.RelevanceScore < localAlert.Triage.RelevanceScore {
		t.Fatalf("expected cross-border alert (%.3f) to score higher than local crime (%.3f)",
			crossBorderAlert.Triage.RelevanceScore, localAlert.Triage.RelevanceScore)
	}
}

func TestJitterRadiusKMIsPrecisionAware(t *testing.T) {
	cityMin, cityMax := jitterRadiusKM("city-db")
	countryMin, countryMax := jitterRadiusKM("country-text")
	if cityMax >= countryMin {
		t.Fatalf("expected city jitter to be tighter than country jitter, got city %.1f-%.1f km vs country %.1f-%.1f km", cityMin, cityMax, countryMin, countryMax)
	}
	if cityMax > 2 {
		t.Fatalf("expected city-db jitter to stay very tight, got max %.1f km", cityMax)
	}
}

func TestRSSItemUsesSummaryForCityPlacement(t *testing.T) {
	cfg := config.Default()
	ctx := Context{
		Config: cfg,
		Now:    time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC),
		Geocoder: NewGeocoder(&mockCityLookup{cities: map[string]CityLookupResult{
			"Valletta|MT": {Name: "Valletta", CountryCode: "MT", Lat: 35.90, Lng: 14.51, Population: 6400},
		}}, nil),
	}
	meta := model.RegistrySource{
		Type:     "rss",
		Category: "public_safety",
		Source: model.SourceMetadata{
			SourceID:      "malta-civil",
			AuthorityName: "Malta Civil Protection",
			Country:       "Malta",
			CountryCode:   "MT",
			Region:        "Europe",
			AuthorityType: "public_safety_program",
			BaseURL:       "https://example.test",
		},
	}
	item := parse.FeedItem{
		Title:     "Incident update",
		Summary:   "Emergency crews dispatched in Valletta harbour district",
		Link:      "https://example.test/incident",
		Published: "2026-03-16T10:00:00Z",
	}
	alert := RSSItem(ctx, meta, item)
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Lat < 35.7 || alert.Lat > 36.1 || alert.Lng < 14.3 || alert.Lng > 14.7 {
		t.Fatalf("expected alert to stay near Valletta, got (%f, %f)", alert.Lat, alert.Lng)
	}
}
