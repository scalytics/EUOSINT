// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/collector/vet"
)

type ClassifiedItem struct {
	Item     parse.FeedItem
	Category string
}

type AlertLLMResponse struct {
	Yes         bool   `json:"yes"`
	Translation string `json:"translation"`
	CategoryID  string `json:"category_id"`
}

type Completer interface {
	Complete(ctx context.Context, messages []vet.Message) (string, error)
}

func BatchLLM(ctx context.Context, cfg config.Config, client Completer, defaultCategory string, items []parse.FeedItem) ([]ClassifiedItem, error) {
	out := make([]ClassifiedItem, 0, len(items))
	for _, item := range items {
		result, err := classifyItem(ctx, client, cfg.AlertLLMModel, defaultCategory, item)
		if err != nil {
			return nil, err
		}
		if !result.Yes {
			continue
		}
		next := item
		if strings.TrimSpace(result.Translation) != "" {
			next.Title = strings.TrimSpace(result.Translation)
		}
		out = append(out, ClassifiedItem{
			Item:     next,
			Category: firstNonEmpty(result.CategoryID, defaultCategory),
		})
	}
	return out, nil
}

func classifyItem(ctx context.Context, client Completer, model string, defaultCategory string, item parse.FeedItem) (AlertLLMResponse, error) {
	payload, err := json.MarshalIndent(map[string]any{
		"default_category": defaultCategory,
		"title":            strings.TrimSpace(item.Title),
		"summary":          strings.TrimSpace(item.Summary),
		"link":             strings.TrimSpace(item.Link),
		"tags":             item.Tags,
	}, "", "  ")
	if err != nil {
		return AlertLLMResponse{}, fmt.Errorf("marshal alert llm input: %w", err)
	}

	content, err := client.Complete(ctx, []vet.Message{
		{
			Role:    "system",
			Content: "You classify a public source alert item. Return strict JSON only with keys yes, translation, category_id. yes must be true only for intelligence-relevant alerts, not generic information/noise. translation must be a short English title. category_id must be one of the known category ids or the supplied default_category.",
		},
		{
			Role:    "user",
			Content: "Model: " + model + "\nEvaluate this item and return JSON only.\n\n" + string(payload),
		},
	})
	if err != nil {
		return AlertLLMResponse{}, err
	}

	return decodeAlertLLMResponse(content)
}

var alertJSONBlockRe = regexp.MustCompile(`(?s)\{.*\}`)

func decodeAlertLLMResponse(content string) (AlertLLMResponse, error) {
	content = strings.TrimSpace(content)
	if match := alertJSONBlockRe.FindString(content); match != "" {
		content = match
	}
	var out AlertLLMResponse
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return AlertLLMResponse{}, fmt.Errorf("decode alert llm response: %w", err)
	}
	out.Translation = strings.TrimSpace(out.Translation)
	out.CategoryID = strings.TrimSpace(out.CategoryID)
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
