// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/scalytics/kafSIEM/internal/collector/fetch"
	"github.com/scalytics/kafSIEM/internal/collector/parse"
)

// maxTranslateChars is the maximum character count sent to Google Translate
// per field. The free gtx endpoint returns 413 for payloads above ~5 KB.
const maxTranslateChars = 2000

var nonLatinRE = regexp.MustCompile(`[\p{Han}\p{Hangul}\p{Cyrillic}\p{Arabic}\p{Thai}]`)

func Batch(ctx context.Context, client *fetch.Client, items []parse.FeedItem) ([]parse.FeedItem, error) {
	out := make([]parse.FeedItem, 0, len(items))
	for _, item := range items {
		next := item
		var err error
		if nonLatinRE.MatchString(next.Title) {
			next.Title, err = toEnglish(ctx, client, next.Title)
			if err != nil {
				return nil, err
			}
		}
		if nonLatinRE.MatchString(next.Summary) {
			next.Summary, err = toEnglish(ctx, client, next.Summary)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, next)
	}
	return out, nil
}

func toEnglish(ctx context.Context, client *fetch.Client, text string) (string, error) {
	// Strip HTML (RSS descriptions often contain full page markup) and truncate
	// to avoid 413 from the free Google Translate endpoint.
	text = parse.StripHTML(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return text, nil
	}
	if len(text) > maxTranslateChars {
		text = text[:maxTranslateChars]
	}
	endpoint := "https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=en&dt=t&q=" + url.QueryEscape(text)
	body, err := client.Text(ctx, endpoint, true, "application/json")
	if err != nil {
		return text, err
	}
	var doc []any
	if err := json.Unmarshal(body, &doc); err != nil {
		return text, fmt.Errorf("decode translate response: %w", err)
	}
	first, ok := doc[0].([]any)
	if !ok {
		return text, nil
	}
	var builder strings.Builder
	for _, segment := range first {
		pair, ok := segment.([]any)
		if !ok || len(pair) == 0 {
			continue
		}
		value, ok := pair[0].(string)
		if ok {
			builder.WriteString(value)
		}
	}
	translated := strings.TrimSpace(builder.String())
	if translated == "" {
		return text, nil
	}
	return translated, nil
}
