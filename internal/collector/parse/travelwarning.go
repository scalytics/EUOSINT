// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"strings"
)

// ParseGermanAATravelWarnings parses the JSON response from the German
// Auswärtiges Amt (Federal Foreign Office) travel warning open-data API.
// The API returns an object whose keys are numeric country IDs and values
// contain the warning metadata.
func ParseGermanAATravelWarnings(body []byte) ([]FeedItem, error) {
	// The top-level structure wraps a "response" object whose keys are
	// country IDs mapping to warning objects, or it can be a flat map.
	// We try both shapes.
	var envelope struct {
		Response map[string]json.RawMessage `json:"response"`
	}
	warnings := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Response) > 0 {
		warnings = envelope.Response
	} else {
		// Fall back to flat map keyed by country ID.
		if err := json.Unmarshal(body, &warnings); err != nil {
			return nil, err
		}
	}

	type warningEntry struct {
		Title       string `json:"title"`
		Country     string `json:"country"`
		Warning     string `json:"warning"`
		Severity    string `json:"severity"`
		LastChanged string `json:"lastChanged"`
		URL         string `json:"url"`
		Effective   string `json:"effective"`
		Content     string `json:"content"`
	}

	items := make([]FeedItem, 0, len(warnings))
	for _, raw := range warnings {
		var entry warningEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		title := firstNonEmpty(entry.Title, entry.Country)
		if strings.TrimSpace(title) == "" {
			continue
		}
		link := strings.TrimSpace(entry.URL)
		if link == "" {
			link = "https://www.auswaertiges-amt.de/de/ReiseUndSicherheit/reise-und-sicherheitshinweise"
		}
		summary := firstNonEmpty(entry.Warning, entry.Content)
		published := firstNonEmpty(entry.LastChanged, entry.Effective)

		tags := []string{}
		if entry.Severity != "" {
			tags = append(tags, entry.Severity)
		}
		if entry.Country != "" {
			tags = append(tags, entry.Country)
		}

		items = append(items, FeedItem{
			Title:     title,
			Link:      link,
			Published: published,
			Summary:   summary,
			Tags:      tags,
		})
	}
	return items, nil
}

// ParseFCDOAtom parses a UK FCDO (Foreign, Commonwealth & Development Office)
// Atom feed containing travel advice entries. This delegates to the generic
// Atom parser in ParseFeed and returns the results.
func ParseFCDOAtom(body []byte) ([]FeedItem, error) {
	items := ParseFeed(string(body))
	return items, nil
}
