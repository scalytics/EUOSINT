// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/vet"
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
	out := make([]model.SourceCandidate, 0, maxTargets)
	seen := map[string]struct{}{}
	for _, seed := range seeds {
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

func searchCandidateTarget(ctx context.Context, client searchCompleter, cfg config.Config, target model.SourceCandidate) ([]model.SourceCandidate, error) {
	maxURLs := cfg.SearchDiscoveryMaxURLsPerTarget
	if maxURLs <= 0 {
		maxURLs = 3
	}

	prompt := fmt.Sprintf(
		"Find up to %d official or authoritative source URLs for %s in %s relevant to %s intelligence collection. Prefer RSS, Atom, JSON APIs, official newsroom feeds, or durable alert/listing pages. Reject local or municipal sources. Return strict JSON only in the form {\"urls\":[{\"url\":\"https://...\",\"reason\":\"short\"}]}.",
		maxURLs,
		firstNonEmpty(target.AuthorityName, "the target authority"),
		firstNonEmpty(target.Country, "its jurisdiction"),
		firstNonEmpty(target.Category, target.AuthorityType, "public safety"),
	)
	if base := strings.TrimSpace(firstNonEmpty(target.BaseURL, target.URL)); base != "" {
		prompt += " Known official website: " + base + "."
	}

	content, err := client.Complete(ctx, []vet.Message{
		{
			Role:    "system",
			Content: "You are a source discovery assistant. Return strict JSON only. Keep output short. Only list official or highly authoritative URLs likely to be usable as feeds, APIs, or durable listing pages for intelligence-relevant collection.",
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
