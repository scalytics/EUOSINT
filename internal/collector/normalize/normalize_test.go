// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
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
	if cityMax > 2 {
		t.Fatalf("expected city-db jitter to stay very tight, got max %.1f km", cityMax)
	}
	// National-level pins spread around the capital/registry point so
	// alerts from the same country don't stack on a single pixel.
	for _, src := range []string{"capital", "country-text"} {
		min, max := jitterRadiusKM(src)
		if min < 5 || max > 30 {
			t.Fatalf("expected %s jitter 5-30 km range, got %.1f-%.1f km", src, min, max)
		}
	}
	regMin, regMax := jitterRadiusKM("registry")
	if regMin < 2 || regMax > 20 {
		t.Fatalf("expected registry jitter 2-20 km range, got %.1f-%.1f km", regMin, regMax)
	}
	_ = cityMin // used above
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

func TestRSSItemSkipsDynamicGeocodingForCyberAdvisory(t *testing.T) {
	cfg := config.Default()
	ctx := Context{
		Config: cfg,
		Now:    time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC),
		Geocoder: NewGeocoder(&mockCityLookup{cities: map[string]CityLookupResult{
			"Berlin|MT": {Name: "Berlin", CountryCode: "DE", Lat: 52.52, Lng: 13.41, Population: 3700000},
			"Berlin":    {Name: "Berlin", CountryCode: "DE", Lat: 52.52, Lng: 13.41, Population: 3700000},
		}}, nil),
	}
	meta := model.RegistrySource{
		Type:     "rss",
		Category: "cyber_advisory",
		Source: model.SourceMetadata{
			SourceID:      "mt-cert",
			AuthorityName: "Malta CERT",
			Country:       "Malta",
			CountryCode:   "MT",
			Region:        "Europe",
			AuthorityType: "cert",
			BaseURL:       "https://example.test",
		},
	}
	item := parse.FeedItem{
		Title:     "Security advisory",
		Summary:   "Berlin malware campaign observed in enterprise endpoints",
		Link:      "https://example.test/advisory",
		Published: "2026-03-16T10:00:00Z",
	}
	alert := RSSItem(ctx, meta, item)
	if alert == nil {
		t.Fatal("expected alert")
	}
	// Cyber advisory should stay pinned to source country capital (Valletta),
	// not dynamic city matches from prose.
	if alert.Lat < 35.5 || alert.Lat > 36.5 || alert.Lng < 14.0 || alert.Lng > 15.0 {
		t.Fatalf("expected advisory to stay near Valletta, got (%f, %f)", alert.Lat, alert.Lng)
	}
}

func TestRSSItemNewsAggregatorPrefersExplicitCountryOverCityFalsePositive(t *testing.T) {
	cfg := config.Default()
	ctx := Context{
		Config: cfg,
		Now:    time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC),
		Geocoder: NewGeocoder(&mockCityLookup{cities: map[string]CityLookupResult{
			"York": {Name: "York", CountryCode: "GB", Lat: 53.96, Lng: -1.08, Population: 210000},
		}}, nil),
	}
	meta := model.RegistrySource{
		Type:      "rss",
		Category:  "conflict_monitoring",
		RegionTag: "EU",
		Source: model.SourceMetadata{
			SourceID:      "gnews-conflict-global",
			AuthorityName: "Google News — Armed Conflict",
			Country:       "International",
			CountryCode:   "INT",
			Region:        "International",
			AuthorityType: "news_aggregator",
			BaseURL:       "https://news.google.com",
		},
	}
	item := parse.FeedItem{
		Title:     "Activists attacked in Germany while posting hostage signs - New York Times",
		Summary:   "Incident reported in Germany with no UK location context.",
		Link:      "https://example.test/gnews-item",
		Published: "2026-03-20T10:00:00Z",
	}
	alert := RSSItem(ctx, meta, item)
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.EventCountryCode != "DE" {
		t.Fatalf("expected DE from explicit country text, got %q (%s)", alert.EventCountryCode, alert.EventCountry)
	}
	if alert.EventGeoSource != "capital" && alert.EventGeoSource != "country-text" {
		t.Fatalf("expected country-level geocode source, got %q", alert.EventGeoSource)
	}
}

func TestInformationalTitleClassification(t *testing.T) {
	tests := []struct {
		title string
		info  bool // true = should be classified as informational
	}{
		// IAEA informational content — should NOT be alerts.
		{"Colombia's Coastal and Marine Research Institute (INVEMAR) Designated as IAEA Collaborating Centre", true},
		{"IAEA Reviews Rwanda's Nuclear Power Infrastructure Development", true},
		{"IAEA convenes ZODIAC Week in Vienna to strengthen global defences against future pandemics", true},
		// Real incidents — should stay actionable.
		{"Nuclear incident at Zaporizhzhia power plant — radiation leak detected", false},
		{"Earthquake magnitude 6.2 strikes central Turkey", false},
		{"Ransomware attack disrupts hospital systems across three EU countries", false},
		{"INTERPOL Red Notice: wanted fugitive arrested in Spain", false},
		// More informational patterns.
		{"Annual cybersecurity awareness campaign launched", true},
		{"Workshop on maritime safety held in Lisbon", true},
		{"Training course on border security for West African officers", true},
		{"Partnership agreement signed between IAEA and University of Tokyo", true},
		{"Justice for Palestinian Women Demands End to Occupation, Reparations, Accountability, Experts Tell Rights Committee", true},
		// Periodic digest titles — should be informational, not alerts.
		{"UKMTO Weekly Piracy Report 15 – 21 November 2025", true},
		{"Monthly Cyber Threat Summary – October 2025", true},
		{"Quarterly Maritime Security Review Q3 2025", true},
		{"Daily Situation Update – Horn of Africa", true},
		{"Bi-Weekly Intelligence Briefing", true},
		{"Annual Piracy Overview 2024", true},
		// Institutional policy/strategy — informational, not alerts.
		{"Consumer investments priorities: strengthening trust, supporting investors", true},
		{"Our strategy for improving consumer confidence in financial markets", true},
		// Regulatory circulars / financial authority guidance — informational.
		{"Circular to Market Participants on Implementation of Guidelines on MiCA", true},
		{"ESMA Report on Trends, Risks and Vulnerabilities", true},
		{"Amendments to the Delegated Regulation on Taxonomy Reporting", true},
		{"Consultation Paper on Regulatory Technical Standards under MiFID II", true},
		{"Lapsing of Authorisation - ABC Investment Services Ltd", true},
		{"ESMA Guidelines on Suitability Requirements under MiFID II", true},
		{"EBA Opinion on De-risking and its Impact on Access to Financial Services", true},
		// But bare "report" or "weekly" alone should NOT match.
		{"Missing persons report filed with authorities", false},
		{"Reporting from the frontline: explosion in Beirut", false},
	}
	for _, tt := range tests {
		gotInfo := IsInformationalTitle(tt.title)
		gotActionable := IsActionableTitle(tt.title)
		if tt.info && !gotInfo {
			t.Errorf("expected informational for %q, but IsInformationalTitle=false", tt.title)
		}
		if !tt.info && gotInfo {
			t.Errorf("expected NOT informational for %q, but IsInformationalTitle=true", tt.title)
		}
		if tt.info && gotActionable {
			// Informational titles may still match actionable patterns (e.g. "pandemic"),
			// but inferSeverity should override to "info".
			sev := inferSeverity(tt.title, "medium")
			if sev != "info" {
				t.Errorf("informational title %q has inferSeverity=%q, want info", tt.title, sev)
			}
		}
	}
}

func TestInferSeverityInformationalOverride(t *testing.T) {
	// "pandemics" in title would normally match critical, but informational
	// pattern should override.
	sev := inferSeverity("IAEA convenes ZODIAC Week in Vienna to strengthen global defences against future pandemics", "medium")
	if sev != "info" {
		t.Fatalf("expected info severity for informational title with pandemic keyword, got %q", sev)
	}
	// Real pandemic outbreak should stay critical.
	sev = inferSeverity("WHO declares pandemic — global emergency response activated", "medium")
	if sev != "critical" {
		t.Fatalf("expected critical severity for real pandemic, got %q", sev)
	}
	sev = inferSeverity("Parliament declares war after cross-border armed attack", "medium")
	if sev != "critical" {
		t.Fatalf("expected critical severity for war declaration signal, got %q", sev)
	}
	sev = inferSeverity("FBI operation targets organized crime money laundering and terrorism financing network", "medium")
	if sev != "critical" {
		t.Fatalf("expected critical severity for 3-domain threat fusion title, got %q", sev)
	}
}

func TestApplySignalLanesSeparatesInfoIntelAlarm(t *testing.T) {
	cfg := config.Default()
	cfg.AlarmRelevanceThreshold = 0.72

	alerts := []model.Alert{
		{
			AlertID:  "info-1",
			Category: "informational",
			Severity: "info",
			Source: model.SourceMetadata{
				AuthorityType: "osint",
			},
		},
		{
			AlertID:  "alarm-1",
			Category: "missing_person",
			Severity: "high",
			Triage:   &model.Triage{RelevanceScore: 0.9},
			Source: model.SourceMetadata{
				AuthorityType: "police",
			},
		},
		{
			AlertID:  "intel-1",
			Category: "cyber_advisory",
			Severity: "medium",
			Triage:   &model.Triage{RelevanceScore: 0.4},
			Source: model.SourceMetadata{
				AuthorityType: "cert",
			},
		},
	}

	got := ApplySignalLanes(cfg, alerts)
	if got[0].SignalLane != model.SignalLaneInfo {
		t.Fatalf("expected informational alert in info lane, got %q", got[0].SignalLane)
	}
	if got[1].SignalLane != model.SignalLaneAlarm {
		t.Fatalf("expected missing_person high alert in alarm lane, got %q", got[1].SignalLane)
	}
	if got[2].SignalLane != model.SignalLaneIntel {
		t.Fatalf("expected cyber advisory in intel lane, got %q", got[2].SignalLane)
	}
}

func TestApplySignalLaneEscalatesStrategicLegislativeAlerts(t *testing.T) {
	cfg := config.Default()
	cfg.AlarmRelevanceThreshold = 0.72

	alert := model.Alert{
		AlertID:  "leg-1",
		Category: "legislative",
		Severity: "critical",
		Title:    "Parliament declares war after armed attack",
		Source: model.SourceMetadata{
			AuthorityType: "government",
		},
	}
	got := ApplySignalLanes(cfg, []model.Alert{alert})
	if len(got) != 1 {
		t.Fatalf("expected one alert, got %d", len(got))
	}
	if got[0].SignalLane != model.SignalLaneAlarm {
		t.Fatalf("expected strategic legislative alert in alarm lane, got %q", got[0].SignalLane)
	}
}

func TestThreatFusionMatchCount(t *testing.T) {
	count := threatFusionMatchCount("Authorities dismantle organized crime ring linked to money laundering and terrorism financing")
	if count < 3 {
		t.Fatalf("expected at least 3 fusion buckets, got %d", count)
	}
	if got := threatFusionMatchCount("Routine patrol update from local police"); got != 0 {
		t.Fatalf("expected 0 fusion buckets for routine text, got %d", got)
	}
}

func TestApplySignalLaneEscalatesThreatFusionFraud(t *testing.T) {
	cfg := config.Default()
	cfg.AlarmRelevanceThreshold = 0.72

	alert := model.Alert{
		AlertID:  "fraud-fusion-1",
		Category: "fraud_alert",
		Severity: "high",
		Title:    "Authorities expose organized crime money laundering scheme with terrorism financing links",
		Triage:   &model.Triage{RelevanceScore: 0.66},
		Source: model.SourceMetadata{
			AuthorityType: "regulatory",
		},
	}
	got := ApplySignalLanes(cfg, []model.Alert{alert})
	if len(got) != 1 {
		t.Fatalf("expected one alert, got %d", len(got))
	}
	if got[0].SignalLane != model.SignalLaneAlarm {
		t.Fatalf("expected threat-fusion fraud alert in alarm lane, got %q", got[0].SignalLane)
	}
}

func TestScoreBoostsThreatFusion(t *testing.T) {
	cfg := config.Default()
	meta := model.RegistrySource{
		Type:     "rss",
		Category: "fraud_alert",
		Source: model.SourceMetadata{
			SourceID:      "fbi-national-press",
			AuthorityName: "FBI National Press Releases",
			Country:       "United States",
			CountryCode:   "US",
			Region:        "North America",
			AuthorityType: "police",
			BaseURL:       "https://www.fbi.gov",
		},
	}
	ctx := Context{Config: cfg, Now: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)}
	plain := RSSItem(ctx, meta, parse.FeedItem{
		Title:     "Fraud enforcement update",
		Summary:   "Authorities announced charges in a fraud case.",
		Link:      "https://www.fbi.gov/plain",
		Published: "2026-03-18T10:00:00Z",
	})
	fusion := RSSItem(ctx, meta, parse.FeedItem{
		Title:     "Organized crime and money laundering network tied to terrorism financing dismantled",
		Summary:   "Investigators detail fraud, laundering, cartel logistics, and terror-financing links.",
		Link:      "https://www.fbi.gov/fusion",
		Published: "2026-03-18T10:00:00Z",
	})
	if plain == nil || fusion == nil {
		t.Fatal("expected both alerts to be normalized")
	}
	if fusion.Triage.RelevanceScore <= plain.Triage.RelevanceScore {
		t.Fatalf("expected fusion score (%.3f) > plain score (%.3f)", fusion.Triage.RelevanceScore, plain.Triage.RelevanceScore)
	}
}

func TestSanitizeGeoTextStripsPublisherAttribution(t *testing.T) {
	raw := "Iceland's Chief 'Lava Cooler' Is Bracing for the Next Eruption - The New York Times"
	got := sanitizeGeoText(raw)
	if got != "Iceland's Chief 'Lava Cooler' Is Bracing for the Next Eruption" {
		t.Fatalf("unexpected sanitized geo text: %q", got)
	}
}

func TestUCDPAlertSetsConflictCategory(t *testing.T) {
	cfg := config.Default()
	ctx := Context{Config: cfg, Now: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)}
	meta := model.RegistrySource{
		Type:     "ucdp-json",
		Category: "conflict_monitoring",
		Source: model.SourceMetadata{
			SourceID:      "ucdp-ged",
			AuthorityName: "UCDP Georeferenced Event Dataset",
			Country:       "Sweden",
			CountryCode:   "SE",
			Region:        "Europe",
			AuthorityType: "research",
			BaseURL:       "https://ucdp.uu.se",
		},
	}
	alert := UCDPAlert(ctx, meta, parse.UCDPItem{
		FeedItem: parse.FeedItem{
			Title:     "State-based conflict in Syria",
			Link:      "https://ucdp.uu.se/exploratory?id=123",
			Published: "2026-03-18",
			Summary:   "Type: State-based conflict. Fatalities: 14",
			Lat:       35.2,
			Lng:       36.8,
		},
		ViolenceType: "State-based conflict",
		Fatalities:   14,
		Country:      "Syria",
		Region:       "Middle East",
	})
	if alert == nil {
		t.Fatal("expected UCDP alert")
	}
	if alert.Category != "conflict_monitoring" {
		t.Fatalf("expected conflict_monitoring, got %q", alert.Category)
	}
	if alert.Severity != "high" {
		t.Fatalf("expected high severity, got %q", alert.Severity)
	}
}

func TestLegislativeInstitutionalStatementDowngradedToInformational(t *testing.T) {
	cfg := config.Default()
	ctx := Context{Config: cfg, Now: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)}
	meta := model.RegistrySource{
		Type:     "rss",
		Category: "legislative",
		Source: model.SourceMetadata{
			SourceID:      "un-rights-committee",
			AuthorityName: "UN Security Council Press",
			Country:       "China",
			CountryCode:   "CN",
			Region:        "International",
			AuthorityType: "government",
			BaseURL:       "https://example.test",
		},
	}
	item := parse.FeedItem{
		Title:     "Justice for Palestinian Women Demands End to Occupation, Reparations, Accountability, Experts Tell Rights Committee",
		Summary:   "Institutional hearing and statements by experts to committee members.",
		Link:      "https://example.test/statement",
		Published: "2026-03-18T10:00:00Z",
	}
	alert := RSSItem(ctx, meta, item)
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.Category != "informational" || alert.Severity != "info" {
		t.Fatalf("expected informational/info, got category=%q severity=%q", alert.Category, alert.Severity)
	}
}

func TestLegacyCategoryRemapOnAlertBuild(t *testing.T) {
	cfg := config.Default()
	ctx := Context{Config: cfg, Now: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)}

	mk := func(category string, title string) *model.Alert {
		meta := model.RegistrySource{
			Type:     "rss",
			Category: category,
			Source: model.SourceMetadata{
				SourceID:      "test-" + category,
				AuthorityName: "Test Source",
				Country:       "Germany",
				CountryCode:   "DE",
				Region:        "Europe",
				AuthorityType: "regulatory",
				BaseURL:       "https://example.test",
			},
		}
		return RSSItem(ctx, meta, parse.FeedItem{
			Title:     title,
			Summary:   "summary",
			Link:      "https://example.test/" + category,
			Published: "2026-03-18T10:00:00Z",
		})
	}

	a1 := mk("public_appeal", "Wanted in fraud operation")
	if a1 == nil || a1.Category != "public_safety" {
		t.Fatalf("expected public_appeal -> public_safety, got %#v", a1)
	}

	a2 := mk("intelligence_report", "Weekly intelligence briefing")
	if a2 == nil || a2.Category != "informational" || a2.Severity != "info" {
		t.Fatalf("expected intelligence_report -> informational/info, got %#v", a2)
	}

	a3 := mk("humanitarian_security", "Armed attack on aid convoy in border zone")
	if a3 == nil || a3.Category != "conflict_monitoring" {
		t.Fatalf("expected humanitarian_security -> conflict_monitoring, got %#v", a3)
	}
}

func TestInferSubcategoryHeuristics(t *testing.T) {
	tests := []struct {
		name     string
		category string
		text     string
		want     string
	}{
		{
			name:     "cyber vulnerability",
			category: "cyber_advisory",
			text:     "Critical CVE-2026-1234 vulnerability requires immediate patch",
			want:     "vulnerability",
		},
		{
			name:     "maritime piracy",
			category: "maritime_security",
			text:     "Oil tanker boarded by armed pirates in gulf transit route",
			want:     "piracy",
		},
		{
			name:     "environment earthquake",
			category: "environmental_disaster",
			text:     "Magnitude 6.2 earthquake triggers aftershocks",
			want:     "earthquake",
		},
		{
			name:     "legislative escalation",
			category: "legislative",
			text:     "Parliament approves declaration of war after invasion",
			want:     "strategic_escalation",
		},
		{
			name:     "informational policy",
			category: "informational",
			text:     "Committee approved new sanctions package and executive order",
			want:     "policy_update",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inferSubcategory(tc.category, tc.text)
			if got != tc.want {
				t.Fatalf("inferSubcategory(%q, %q) = %q, want %q", tc.category, tc.text, got, tc.want)
			}
		})
	}
}
