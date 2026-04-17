// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/parse"
	"github.com/scalytics/kafSIEM/internal/collector/vet"
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

type alertBatchResponse struct {
	Items []struct {
		Index       int    `json:"index"`
		Yes         bool   `json:"yes"`
		Translation string `json:"translation"`
		CategoryID  string `json:"category_id"`
	} `json:"items"`
}

type Completer interface {
	Complete(ctx context.Context, messages []vet.Message) (string, error)
}

func BatchLLM(ctx context.Context, cfg config.Config, client Completer, defaultCategory string, items []parse.FeedItem) ([]ClassifiedItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	results, err := classifyItems(ctx, client, cfg.AlertLLMModel, defaultCategory, items)
	if err != nil {
		return nil, err
	}
	out := make([]ClassifiedItem, 0, len(items))
	for index, item := range items {
		result, ok := results[index]
		if !ok {
			continue
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

func classifyItems(ctx context.Context, client Completer, model string, defaultCategory string, items []parse.FeedItem) (map[int]AlertLLMResponse, error) {
	batch := make([]map[string]any, 0, len(items))
	for index, item := range items {
		batch = append(batch, map[string]any{
			"index":            index,
			"default_category": defaultCategory,
			"title":            strings.TrimSpace(item.Title),
			"summary":          strings.TrimSpace(item.Summary),
			"link":             strings.TrimSpace(item.Link),
			"tags":             item.Tags,
		})
	}
	payload, err := json.MarshalIndent(map[string]any{
		"items": batch,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal alert llm input: %w", err)
	}

	content, err := client.Complete(ctx, []vet.Message{
		{
			Role:    "system",
			Content: "You classify public source alert items. Return strict JSON only in the form {\"items\":[{\"index\":0,\"yes\":true,\"translation\":\"short english title\",\"category_id\":\"known_or_default\"}]}. yes must be true only for intelligence-relevant alerts, not generic information/noise. translation must be a short English title. category_id must be one of the known category ids or the supplied default_category.",
		},
		{
			Role:    "user",
			Content: "Model: " + model + "\nEvaluate these items and return JSON only.\n\n" + string(payload),
		},
	})
	if err != nil {
		return nil, err
	}

	return decodeAlertBatchLLMResponse(content)
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

func decodeAlertBatchLLMResponse(content string) (map[int]AlertLLMResponse, error) {
	content = strings.TrimSpace(content)
	if match := alertJSONBlockRe.FindString(content); match != "" {
		content = match
	}
	var out alertBatchResponse
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("decode alert llm batch response: %w", err)
	}
	results := make(map[int]AlertLLMResponse, len(out.Items))
	for _, item := range out.Items {
		results[item.Index] = AlertLLMResponse{
			Yes:         item.Yes,
			Translation: strings.TrimSpace(item.Translation),
			CategoryID:  strings.TrimSpace(item.CategoryID),
		}
	}
	return results, nil
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
