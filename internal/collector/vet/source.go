// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package vet

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/parse"
)

type Sample struct {
	Title   string   `json:"title"`
	Link    string   `json:"link"`
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type Input struct {
	AuthorityName string   `json:"authority_name"`
	AuthorityType string   `json:"authority_type"`
	Category      string   `json:"category"`
	Country       string   `json:"country"`
	CountryCode   string   `json:"country_code"`
	URL           string   `json:"url"`
	BaseURL       string   `json:"base_url"`
	FeedType      string   `json:"feed_type"`
	Samples       []Sample `json:"samples"`
}

type Verdict struct {
	Approve              bool      `json:"approve"`
	PromotionStatus      string    `json:"promotion_status"`
	Category             string    `json:"category,omitempty"`
	LanguageCode         string    `json:"language_code,omitempty"`
	Level                string    `json:"level"`
	MissionTags          []string  `json:"mission_tags"`
	SourceQuality        flexFloat `json:"source_quality"`
	OperationalRelevance flexFloat `json:"operational_relevance"`
	Reason               string    `json:"reason"`
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*f = 0
		return nil
	}

	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = flexFloat(num)
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return err
	}
	asString = strings.TrimSpace(asString)
	if asString == "" {
		*f = 0
		return nil
	}
	switch strings.ToLower(asString) {
	case "none", "n/a", "null", "unknown":
		*f = 0
		return nil
	case "very low":
		*f = 0.1
		return nil
	case "low":
		*f = 0.25
		return nil
	case "medium":
		*f = 0.5
		return nil
	case "high":
		*f = 0.8
		return nil
	case "very high":
		*f = 0.95
		return nil
	}
	if strings.HasSuffix(asString, "%") {
		asString = strings.TrimSuffix(asString, "%")
		num, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
		if err != nil {
			return err
		}
		*f = flexFloat(num / 100)
		return nil
	}
	num, err := strconv.ParseFloat(asString, 64)
	if err != nil {
		return err
	}
	*f = flexFloat(num)
	return nil
}

type Vetter struct {
	client *Client
	model  string
}

func New(cfg config.Config) *Vetter {
	return &Vetter{client: NewClient(cfg), model: cfg.VettingModel}
}

func (v *Vetter) Evaluate(ctx context.Context, input Input) (Verdict, error) {
	if reason, reject := deterministicReject(input); reject {
		return Verdict{
			Approve:              false,
			PromotionStatus:      "rejected",
			Level:                "local",
			MissionTags:          nil,
			SourceQuality:        0.1,
			OperationalRelevance: 0.1,
			Reason:               reason,
		}, nil
	}

	payload, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return Verdict{}, fmt.Errorf("marshal source vetting input: %w", err)
	}

	content, err := v.client.Complete(ctx, []Message{
		{
			Role: "system",
			Content: `You vet intelligence source candidates for an OSINT dashboard.

Approve only operationally relevant sources that publish actionable intelligence — wanted/missing persons, public appeals, cyber advisories, vulnerability disclosures, humanitarian security, conflict monitoring, disease outbreaks, environmental disasters, fraud alerts, terrorism, travel warnings, emergency management, or public-safety intelligence.

Reject: generic PR, speeches, institutional updates, marketing, newsletters, or low-signal content.
Local/municipal law-enforcement sources are allowed when official and operationally actionable (these are used for country-scoped views, not global defaults).

Valid categories (pick the best match):
- cyber_advisory: vulnerability disclosures, patch advisories, threat intel from CERTs
- wanted_suspect: arrest warrants, wanted person notices
- missing_person: missing persons, AMBER alerts
- public_appeal: police witness calls, identification requests, crime tips
- fraud_alert: financial crime, scam warnings, sanctions, AML
- intelligence_report: strategic assessments, geopolitical analysis
- travel_warning: government travel advisories, consular warnings
- conflict_monitoring: armed conflict tracking, ceasefire violations, peace processes
- humanitarian_security: aid worker safety, access restrictions, crisis zones
- humanitarian_tasking: humanitarian missions, disaster response deployments
- health_emergency: disease outbreaks, pandemic updates, biosecurity
- disease_outbreak: epidemics, zoonotic diseases, outbreak surveillance
- environmental_disaster: earthquakes, oil spills, floods, wildfires, nuclear incidents, volcanic activity
- public_safety: civil protection, natural disaster warnings, emergency notifications
- emergency_management: disaster declarations, evacuation orders, crisis coordination
- terrorism_tip: counter-terrorism alerts, extremism threat assessments
- private_sector: corporate security, supply chain disruptions
- informational: general information, educational content

Also detect the primary content language from the sample titles/summaries and return it as an ISO 639-1 code (e.g. "en", "fr", "de", "is", "hu", "ar", "ja"). Use "en" if the content is in English.

Return strict JSON only with keys: approve, promotion_status, category, language_code, level, mission_tags, source_quality, operational_relevance, reason.`,
		},
		{
			Role:    "user",
			Content: "Evaluate this discovered source and return JSON with keys approve, promotion_status, category, language_code, level, mission_tags, source_quality, operational_relevance, reason.\n\n" + string(payload),
		},
	})
	if err != nil {
		return Verdict{}, err
	}

	verdict, err := decodeVerdict(content)
	if err != nil {
		return Verdict{}, err
	}
	verdict.normalize()
	return verdict, nil
}

func SamplesFromFeedItems(items []parse.FeedItem, limit int) []Sample {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	samples := make([]Sample, 0, limit)
	for _, item := range items[:limit] {
		samples = append(samples, Sample{
			Title:   strings.TrimSpace(item.Title),
			Link:    strings.TrimSpace(item.Link),
			Summary: strings.TrimSpace(item.Summary),
			Tags:    append([]string(nil), item.Tags...),
		})
	}
	return samples
}

var jsonBlockRe = regexp.MustCompile(`(?s)\{.*\}`)

func decodeVerdict(content string) (Verdict, error) {
	content = strings.TrimSpace(content)
	if match := jsonBlockRe.FindString(content); match != "" {
		content = match
	}
	var verdict Verdict
	if err := json.Unmarshal([]byte(content), &verdict); err != nil {
		return Verdict{}, fmt.Errorf("decode source vetting verdict: %w", err)
	}
	return verdict, nil
}

var validCategories = map[string]bool{
	"cyber_advisory": true, "wanted_suspect": true, "missing_person": true,
	"public_appeal": true, "fraud_alert": true, "intelligence_report": true,
	"travel_warning": true, "conflict_monitoring": true, "humanitarian_security": true,
	"humanitarian_tasking": true, "health_emergency": true, "disease_outbreak": true,
	"environmental_disaster": true, "public_safety": true, "emergency_management": true,
	"terrorism_tip": true, "private_sector": true, "informational": true,
}

func (v *Verdict) normalize() {
	v.PromotionStatus = strings.ToLower(strings.TrimSpace(v.PromotionStatus))
	switch v.PromotionStatus {
	case "active", "validated", "rejected":
	default:
		if v.Approve {
			v.PromotionStatus = "active"
		} else {
			v.PromotionStatus = "rejected"
		}
	}
	v.Category = strings.ToLower(strings.TrimSpace(v.Category))
	if !validCategories[v.Category] {
		v.Category = ""
	}
	v.LanguageCode = strings.ToLower(strings.TrimSpace(v.LanguageCode))
	v.Level = strings.ToLower(strings.TrimSpace(v.Level))
	switch v.Level {
	case "international", "supranational", "federal", "national", "regional", "local":
	default:
		v.Level = "national"
	}
	v.SourceQuality = flexFloat(clamp01(float64(v.SourceQuality)))
	v.OperationalRelevance = flexFloat(clamp01(float64(v.OperationalRelevance)))
	if !v.Approve {
		v.PromotionStatus = "rejected"
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func deterministicReject(input Input) (string, bool) {
	if len(input.Samples) == 0 {
		return "deterministic reject: no sample items to assess", true
	}
	return "", false
}
