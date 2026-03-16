// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package vet

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/parse"
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
	Approve              bool     `json:"approve"`
	PromotionStatus      string   `json:"promotion_status"`
	Level                string   `json:"level"`
	MissionTags          []string `json:"mission_tags"`
	SourceQuality        float64  `json:"source_quality"`
	OperationalRelevance float64  `json:"operational_relevance"`
	Reason               string   `json:"reason"`
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
			Role:    "system",
			Content: "You vet intelligence source candidates. Approve only operationally relevant sources: supranational, federal, or national level sources that publish actionable intelligence, wanted/missing/public appeals, cyber advisories, humanitarian security, crisis, war, organized crime, fraud, terrorism, or public-safety intelligence. Reject generic PR, speeches, institutional updates, local police, municipal news, or low-signal information. Return strict JSON only.",
		},
		{
			Role:    "user",
			Content: "Evaluate this discovered source and return JSON with keys approve, promotion_status, level, mission_tags, source_quality, operational_relevance, reason.\n\n" + string(payload),
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
	v.Level = strings.ToLower(strings.TrimSpace(v.Level))
	switch v.Level {
	case "international", "supranational", "federal", "national", "regional", "local":
	default:
		v.Level = "national"
	}
	v.SourceQuality = clamp01(v.SourceQuality)
	v.OperationalRelevance = clamp01(v.OperationalRelevance)
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
	hay := strings.ToLower(strings.Join([]string{
		input.AuthorityName,
		input.AuthorityType,
		input.Category,
		input.URL,
		input.BaseURL,
	}, " "))
	for _, needle := range []string{
		"municipal", "municipality", "city of ", "county ", "sheriff", "police department", "local police",
	} {
		if strings.Contains(hay, needle) {
			return "deterministic reject: local or municipal source", true
		}
	}
	if len(input.Samples) == 0 {
		return "deterministic reject: no sample items to assess", true
	}
	return "", false
}
