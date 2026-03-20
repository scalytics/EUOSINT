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
	ViolenceType   string
	Fatalities     int
	CivilianDeaths int
	Country        string
	CountryID      string
	CountryCode    string
	Region         string
	SideA          string
	SideB          string
	DyadName       string
	Admin1         string
	Admin2         string
	WherePrecision int
	DatePrecision  int
	EventClarity   int
}

type UCDPCountryRef struct {
	ID    string
	ISO2  string
	ISO3  string
	Label string
}

var ucdpCountryRefsByISO2 = map[string]UCDPCountryRef{
	"DZ": {ID: "615", ISO2: "DZ", ISO3: "DZA", Label: "Algeria"},
	"BF": {ID: "439", ISO2: "BF", ISO3: "BFA", Label: "Burkina Faso"},
	"BI": {ID: "516", ISO2: "BI", ISO3: "BDI", Label: "Burundi"},
	"BG": {ID: "355", ISO2: "BG", ISO3: "BGR", Label: "Bulgaria"},
	"CD": {ID: "490", ISO2: "CD", ISO3: "COD", Label: "Democratic Republic of the Congo"},
	"CF": {ID: "482", ISO2: "CF", ISO3: "CAF", Label: "Central African Republic"},
	"DJ": {ID: "522", ISO2: "DJ", ISO3: "DJI", Label: "Djibouti"},
	"EG": {ID: "651", ISO2: "EG", ISO3: "EGY", Label: "Egypt"},
	"ER": {ID: "531", ISO2: "ER", ISO3: "ERI", Label: "Eritrea"},
	"ET": {ID: "530", ISO2: "ET", ISO3: "ETH", Label: "Ethiopia"},
	"IL": {ID: "666", ISO2: "IL", ISO3: "ISR", Label: "Israel"},
	"JO": {ID: "663", ISO2: "JO", ISO3: "JOR", Label: "Jordan"},
	"LB": {ID: "660", ISO2: "LB", ISO3: "LBN", Label: "Lebanon"},
	"ML": {ID: "432", ISO2: "ML", ISO3: "MLI", Label: "Mali"},
	"MR": {ID: "435", ISO2: "MR", ISO3: "MRT", Label: "Mauritania"},
	"NE": {ID: "436", ISO2: "NE", ISO3: "NER", Label: "Niger"},
	"PS": {ISO2: "PS", ISO3: "PSE", Label: "Palestine"},
	"RO": {ID: "360", ISO2: "RO", ISO3: "ROU", Label: "Romania"},
	"RU": {ID: "365", ISO2: "RU", ISO3: "RUS", Label: "Russia"},
	"RW": {ID: "517", ISO2: "RW", ISO3: "RWA", Label: "Rwanda"},
	"SA": {ID: "670", ISO2: "SA", ISO3: "SAU", Label: "Saudi Arabia"},
	"SD": {ID: "625", ISO2: "SD", ISO3: "SDN", Label: "Sudan"},
	"SO": {ID: "520", ISO2: "SO", ISO3: "SOM", Label: "Somalia"},
	"SS": {ID: "626", ISO2: "SS", ISO3: "SSD", Label: "South Sudan"},
	"TD": {ID: "483", ISO2: "TD", ISO3: "TCD", Label: "Chad"},
	"TR": {ID: "640", ISO2: "TR", ISO3: "TUR", Label: "Turkey"},
	"UA": {ID: "369", ISO2: "UA", ISO3: "UKR", Label: "Ukraine"},
	"UG": {ID: "500", ISO2: "UG", ISO3: "UGA", Label: "Uganda"},
	"YE": {ID: "680", ISO2: "YE", ISO3: "YEM", Label: "Yemen"},
}

var ucdpCountryRefsByID = buildUCDPCountryRefsByID()
var ucdpCountryRefsByName = buildUCDPCountryRefsByName()

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
		countryID := normalizeUCDPCountryID(firstString(ev, "country_id", "gwno_loc", "gwno"))
		countryCode := normalizeUCDPCountryCode(firstString(ev, "country_code"), countryID, country)
		region := firstString(ev, "region")
		sideA := firstString(ev, "side_a", "actor1")
		sideB := firstString(ev, "side_b", "actor2")
		dyadName := firstString(ev, "dyad_name")
		violenceType := normalizeUCDPViolenceType(firstString(ev, "type_of_violence", "type_of_violence_text", "event_type"))
		fatalities := firstInt(ev, "best", "fatalities_best", "deaths_a", "deaths_b")
		civilianDeaths := firstInt(ev, "deaths_civilians", "deaths_civilian")
		lat := firstFloat(ev, "latitude", "lat")
		lng := firstFloat(ev, "longitude", "lon", "lng")
		admin1 := firstString(ev, "adm_1", "admin1")
		admin2 := firstString(ev, "adm_2", "admin2")
		wherePrecision := firstInt(ev, "where_prec", "where_precision")
		datePrecision := firstInt(ev, "date_prec", "date_precision")
		eventClarity := firstInt(ev, "event_clarity")
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
			ViolenceType:   violenceType,
			Fatalities:     fatalities,
			CivilianDeaths: civilianDeaths,
			Country:        country,
			CountryID:      countryID,
			CountryCode:    countryCode,
			Region:         region,
			SideA:          sideA,
			SideB:          sideB,
			DyadName:       dyadName,
			Admin1:         admin1,
			Admin2:         admin2,
			WherePrecision: wherePrecision,
			DatePrecision:  datePrecision,
			EventClarity:   eventClarity,
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

func UCDPCountryRefByISO2(code string) (UCDPCountryRef, bool) {
	ref, ok := ucdpCountryRefsByISO2[strings.ToUpper(strings.TrimSpace(code))]
	return ref, ok
}

func UCDPCountryRefByID(id string) (UCDPCountryRef, bool) {
	ref, ok := ucdpCountryRefsByID[strings.TrimSpace(id)]
	return ref, ok
}

func normalizeUCDPCountryID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, err := strconv.Atoi(raw); err == nil {
		return raw
	}
	return ""
}

func normalizeUCDPCountryCode(raw string, countryID string, country string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if len(raw) == 2 && raw[0] >= 'A' && raw[0] <= 'Z' && raw[1] >= 'A' && raw[1] <= 'Z' {
		return raw
	}
	if ref, ok := UCDPCountryRefByID(countryID); ok && ref.ISO2 != "" {
		return ref.ISO2
	}
	if ref, ok := ucdpCountryRefsByName[strings.ToLower(strings.TrimSpace(country))]; ok && ref.ISO2 != "" {
		return ref.ISO2
	}
	return ""
}

func buildUCDPCountryRefsByID() map[string]UCDPCountryRef {
	out := make(map[string]UCDPCountryRef, len(ucdpCountryRefsByISO2))
	for _, ref := range ucdpCountryRefsByISO2 {
		if strings.TrimSpace(ref.ID) == "" {
			continue
		}
		out[ref.ID] = ref
	}
	// UCDP Yemen records appear under both 679 and 680 across historical slices.
	if ref, ok := ucdpCountryRefsByISO2["YE"]; ok {
		out["679"] = ref
		out["680"] = ref
	}
	return out
}

func buildUCDPCountryRefsByName() map[string]UCDPCountryRef {
	out := make(map[string]UCDPCountryRef, len(ucdpCountryRefsByISO2))
	for _, ref := range ucdpCountryRefsByISO2 {
		if strings.TrimSpace(ref.Label) != "" {
			out[strings.ToLower(strings.TrimSpace(ref.Label))] = ref
		}
	}
	out["dem. rep. congo"] = ucdpCountryRefsByISO2["CD"]
	out["congo, democratic republic of the"] = ucdpCountryRefsByISO2["CD"]
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
