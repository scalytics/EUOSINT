// Copyright 2025 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// UCDPItem extends FeedItem with UCDP conflict metadata.
type UCDPItem struct {
	FeedItem
	ViolenceType string
	Fatalities   int
	Country      string
	Region       string
}

// ParseUCDP parses UCDP API responses with flexible envelope keys.
// Supported envelopes: {"Result":[...]}, {"results":[...]}, {"data":[...]}.
func ParseUCDP(body []byte) ([]UCDPItem, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}

	eventsRaw := firstRaw(envelope, "Result", "result", "results", "Data", "data")
	if len(eventsRaw) == 0 {
		return nil, fmt.Errorf("UCDP response missing result array")
	}

	var events []map[string]any
	if err := json.Unmarshal(eventsRaw, &events); err != nil {
		return nil, err
	}

	out := make([]UCDPItem, 0, len(events))
	for _, ev := range events {
		dateStart := firstString(ev, "date_start", "date_start_utc", "date_start_full", "event_date")
		if dateStart == "" {
			dateStart = firstString(ev, "date_start_prec", "date_end")
		}
		country := firstString(ev, "country", "country_name")
		region := firstString(ev, "region")
		sideA := firstString(ev, "side_a", "actor1")
		sideB := firstString(ev, "side_b", "actor2")
		violenceType := normalizeUCDPViolenceType(firstString(ev, "type_of_violence", "type_of_violence_text", "event_type"))
		fatalities := firstInt(ev, "best", "fatalities_best", "deaths_a", "deaths_b")
		lat := firstFloat(ev, "latitude", "lat")
		lng := firstFloat(ev, "longitude", "lon", "lng")
		id := firstString(ev, "id", "event_id", "dyad_id")
		if id == "" {
			id = firstString(ev, "source_article")
		}

		title := buildUCDPTitle(violenceType, country, sideA, sideB)
		if strings.TrimSpace(title) == "" {
			continue
		}

		summaryParts := []string{}
		if violenceType != "" {
			summaryParts = append(summaryParts, "Type: "+violenceType)
		}
		if sideA != "" {
			summaryParts = append(summaryParts, "Side A: "+sideA)
		}
		if sideB != "" {
			summaryParts = append(summaryParts, "Side B: "+sideB)
		}
		if fatalities > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("Fatalities: %d", fatalities))
		}
		if region != "" {
			summaryParts = append(summaryParts, "Region: "+region)
		}

		link := "https://ucdp.uu.se/"
		if strings.TrimSpace(id) != "" {
			link = "https://ucdp.uu.se/exploratory?id=" + strings.TrimSpace(id)
		}

		tags := []string{"ucdp", strings.ToLower(strings.TrimSpace(violenceType))}
		if country != "" {
			tags = append(tags, strings.ToLower(country))
		}

		out = append(out, UCDPItem{
			FeedItem: FeedItem{
				Title:     title,
				Link:      link,
				Published: strings.TrimSpace(dateStart),
				Summary:   strings.Join(summaryParts, ". "),
				Tags:      compactTags(tags),
				Lat:       lat,
				Lng:       lng,
			},
			ViolenceType: violenceType,
			Fatalities:   fatalities,
			Country:      country,
			Region:       region,
		})
	}
	return out, nil
}

func buildUCDPTitle(violenceType, country, sideA, sideB string) string {
	parts := []string{}
	if violenceType != "" {
		parts = append(parts, violenceType)
	} else {
		parts = append(parts, "Organized violence event")
	}
	if country != "" {
		parts = append(parts, "in "+country)
	}
	if sideA != "" && sideB != "" {
		parts = append(parts, "("+sideA+" vs "+sideB+")")
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func normalizeUCDPViolenceType(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	switch strings.ToLower(v) {
	case "1":
		return "State-based conflict"
	case "2":
		return "Non-state conflict"
	case "3":
		return "One-sided violence"
	default:
		return v
	}
}

func compactTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func firstRaw(values map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		if v, ok := values[key]; ok && len(v) > 0 && string(v) != "null" {
			return v
		}
	}
	return nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		}
	}
	return ""
}

func firstFloat(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return typed
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func firstInt(values map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return int(typed)
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}
