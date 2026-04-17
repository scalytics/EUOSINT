// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/model"
	"github.com/scalytics/kafSIEM/internal/collector/vet"
)

type searchCompleter interface {
	Complete(ctx context.Context, messages []vet.Message) (string, error)
}

type llmSearchResponse struct {
	URLs []struct {
		URL    string `json:"url"`
		Reason string `json:"reason"`
	} `json:"urls"`
}

var searchJSONBlockRe = regexp.MustCompile(`(?s)\{.*\}`)

func llmSearchCandidates(ctx context.Context, cfg config.Config, client searchCompleter, seeds []model.SourceCandidate) ([]model.SourceCandidate, error) {
	if !cfg.SearchDiscoveryEnabled || client == nil {
		return nil, nil
	}
	targets := selectSearchTargets(cfg, seeds)
	if len(targets) == 0 {
		return nil, nil
	}

	out := make([]model.SourceCandidate, 0, len(targets)*cfg.SearchDiscoveryMaxURLsPerTarget)
	var failures []string
	for _, target := range targets {
		found, err := searchCandidateTarget(ctx, client, cfg, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", firstNonEmpty(target.AuthorityName, target.URL), err))
			continue
		}
		out = append(out, found...)
	}
	if len(failures) > 0 {
		return out, fmt.Errorf("%s", strings.Join(failures, " | "))
	}
	return out, nil
}

func selectSearchTargets(cfg config.Config, seeds []model.SourceCandidate) []model.SourceCandidate {
	maxTargets := cfg.SearchDiscoveryMaxTargets
	if maxTargets <= 0 {
		return nil
	}
	ordered := append([]model.SourceCandidate(nil), seeds...)
	sort.SliceStable(ordered, func(i, j int) bool {
		pi := searchTargetPriority(ordered[i])
		pj := searchTargetPriority(ordered[j])
		if pi != pj {
			return pi < pj
		}
		return i < j
	})
	out := make([]model.SourceCandidate, 0, maxTargets)
	seen := map[string]struct{}{}
	for _, seed := range ordered {
		if !passesDiscoveryHygiene(seed.AuthorityName, firstNonEmpty(seed.BaseURL, seed.URL), seed.AuthorityType) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(seed.AuthorityName)) + "|" + strings.ToUpper(strings.TrimSpace(seed.CountryCode)) + "|" + strings.ToLower(strings.TrimSpace(seed.Category))
		if key == "||" {
			key = normalizeURL(firstNonEmpty(seed.URL, seed.BaseURL))
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, seed)
		if len(out) >= maxTargets {
			break
		}
	}
	return out
}

func searchTargetPriority(seed model.SourceCandidate) int {
	notes := strings.ToLower(strings.TrimSpace(seed.Notes))
	switch {
	case strings.Contains(notes, "replacement-search"):
		return 0
	case strings.Contains(notes, "gap-analysis"):
		return 1
	case strings.Contains(notes, "trend-spike"):
		return 2
	default:
		return 3
	}
}

func searchCandidateTarget(ctx context.Context, client searchCompleter, cfg config.Config, target model.SourceCandidate) ([]model.SourceCandidate, error) {
	maxURLs := cfg.SearchDiscoveryMaxURLsPerTarget
	if maxURLs <= 0 {
		maxURLs = 3
	}

	prompt := fmt.Sprintf(
		"Find up to %d official RSS or ATOM feed URLs for %s in %s covering %s. Prefer national/supranational authorities, but include official local law-enforcement sources when clearly operational. Return strict JSON only in the form {\"urls\":[{\"url\":\"https://...\",\"reason\":\"short\"}]}. If no official feed exists, return {\"urls\":[]}.",
		maxURLs,
		firstNonEmpty(target.AuthorityName, "high-authority OSINT sources"),
		firstNonEmpty(target.Country, "its jurisdiction"),
		searchTopicLabel(target.Category, target.AuthorityType),
	)
	if base := strings.TrimSpace(firstNonEmpty(target.BaseURL, target.URL)); base != "" {
		prompt += " Known official website: " + base + "."
	}

	content, err := client.Complete(ctx, []vet.Message{
		{
			Role:    "system",
			Content: "You are a source discovery assistant. Return strict JSON only. Keep output short. Only list official or highly authoritative RSS or ATOM feed URLs suitable for intelligence-relevant collection.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	})
	if err != nil {
		return nil, err
	}
	resp, err := decodeLLMSearchResponse(content)
	if err != nil {
		return nil, err
	}

	found := make([]model.SourceCandidate, 0, len(resp.URLs))
	seen := map[string]struct{}{}
	for _, item := range resp.URLs {
		raw := strings.TrimSpace(item.URL)
		if raw == "" {
			continue
		}
		if !looksLikeURL(raw) {
			continue
		}
		if !looksLikeFeedURL(raw) {
			continue
		}
		key := normalizeURL(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		found = append(found, model.SourceCandidate{
			URL:           raw,
			AuthorityName: target.AuthorityName,
			AuthorityType: target.AuthorityType,
			Category:      target.Category,
			Country:       target.Country,
			CountryCode:   target.CountryCode,
			Region:        target.Region,
			BaseURL:       firstNonEmpty(target.BaseURL, target.URL),
			Notes:         "llm-search:" + strings.TrimSpace(cfg.VettingProvider) + " " + strings.TrimSpace(item.Reason),
		})
		if len(found) >= maxURLs {
			break
		}
	}
	return found, nil
}

func searchTopicLabel(category string, authorityType string) string {
	switch strings.TrimSpace(category) {
	case "missing_person":
		return "missing persons and missing children"
	case "wanted_suspect":
		return "wanted persons, fugitives, and public appeals"
	case "terror_warning", "terrorism_tip":
		return "terrorism warnings and threat notices"
	case "organized_crime":
		return "organized crime and major criminal investigations"
	case "travel_warning":
		return "travel warnings and travel advisories"
	case "cyber_advisory":
		return "cyber advisories and security alerts"
	case "public_appeal":
		return "public appeals, wanted persons, and missing persons"
	case "fraud_alert":
		return "fraud alerts, financial crime warnings, and sanctions notices"
	case "intelligence_report":
		return "strategic intelligence assessments and geopolitical analysis"
	case "conflict_monitoring":
		return "armed conflict tracking, ceasefire monitoring, and peace processes"
	case "maritime_security":
		return "maritime security, piracy, shipping threats, coast guard activity, and naval incidents"
	case "legislative":
		return "sanctions, defense policy, foreign affairs, security legislation, and parliamentary security debates"
	case "humanitarian_security", "humanitarian_tasking":
		return "humanitarian operations, aid worker security, and crisis coordination"
	case "health_emergency", "disease_outbreak":
		return "disease outbreaks, epidemics, pandemic surveillance, and public health emergencies"
	case "environmental_disaster":
		return "environmental disasters, earthquakes, oil spills, volcanic activity, and nuclear incidents"
	case "public_safety", "emergency_management":
		return "civil protection, emergency management, and natural disaster warnings"
	default:
		if strings.TrimSpace(authorityType) != "" {
			return authorityType + " intelligence collection"
		}
		return "intelligence collection"
	}
}

func decodeLLMSearchResponse(content string) (llmSearchResponse, error) {
	content = strings.TrimSpace(content)
	if match := searchJSONBlockRe.FindString(content); match != "" {
		content = match
	}
	var out llmSearchResponse
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return llmSearchResponse{}, fmt.Errorf("decode search discovery response: %w", err)
	}
	return out, nil
}

func looksLikeURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

func looksLikeFeedURL(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return strings.Contains(raw, "rss") ||
		strings.Contains(raw, "atom") ||
		strings.HasSuffix(raw, ".xml") ||
		strings.Contains(raw, "/feed")
}

func socialDiscoveryCategory(category string) bool {
	switch strings.TrimSpace(strings.ToLower(category)) {
	case "conflict_monitoring", "maritime_security", "terror_warning", "terrorism_tip", "intelligence_report":
		return true
	default:
		return false
	}
}

func looksLikeSocialSignalURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return false
	}
	switch host {
	case "x.com", "twitter.com":
		if parts[0] == "i" || parts[0] == "search" || parts[0] == "explore" || parts[0] == "home" {
			return false
		}
		return true
	case "t.me", "telegram.me":
		return parts[0] != "s" || len(parts) > 1
	default:
		return false
	}
}
