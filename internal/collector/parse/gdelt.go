// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"strings"
)

// GDELTResponse is the top-level response from GDELT v2 ArtList mode.
type GDELTResponse struct {
	Articles []GDELTArticle `json:"articles"`
}

type GDELTArticle struct {
	URL            string `json:"url"`
	Title          string `json:"title"`
	Seendate       string `json:"seendate"`       // "20260318T120000Z"
	Domain         string `json:"domain"`
	Language       string `json:"language"`
	SourceCountry  string `json:"sourcecountry"`  // "United States"
}

// ParseGDELT parses a GDELT v2 ArtList JSON response.
func ParseGDELT(body []byte) ([]FeedItem, error) {
	var doc GDELTResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	items := make([]FeedItem, 0, len(doc.Articles))
	seen := make(map[string]struct{}, len(doc.Articles))
	for _, a := range doc.Articles {
		if a.Title == "" || a.URL == "" {
			continue
		}
		// Deduplicate by URL.
		if _, dup := seen[a.URL]; dup {
			continue
		}
		seen[a.URL] = struct{}{}

		published := normalizeGDELTDate(a.Seendate)

		tags := []string{"gdelt"}
		if a.SourceCountry != "" {
			tags = append(tags, strings.ToLower(a.SourceCountry))
		}

		items = append(items, FeedItem{
			Title:     a.Title,
			Link:      a.URL,
			Published: published,
			Author:    a.Domain,
			Summary:   a.Title,
			Tags:      tags,
		})
	}
	return items, nil
}

// normalizeGDELTDate converts GDELT's "20260318T120000Z" to RFC3339.
func normalizeGDELTDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return s
	}
	// "20260318T120000Z" → "2026-03-18T12:00:00Z"
	if len(s) >= 15 && s[8] == 'T' {
		return s[0:4] + "-" + s[4:6] + "-" + s[6:8] + "T" + s[9:11] + ":" + s[11:13] + ":" + s[13:15] + "Z"
	}
	// Fallback: just date.
	return s[0:4] + "-" + s[4:6] + "-" + s[6:8]
}

// GDELTCountryToISO2 maps common GDELT sourcecountry names to ISO2 codes.
var gdeltCountryToISO2 = map[string]string{
	"united states":  "US", "united kingdom": "GB", "france": "FR",
	"germany": "DE", "russia": "RU", "china": "CN", "india": "IN",
	"japan": "JP", "brazil": "BR", "canada": "CA", "australia": "AU",
	"italy": "IT", "spain": "ES", "turkey": "TR", "iran": "IR",
	"iraq": "IQ", "israel": "IL", "ukraine": "UA", "poland": "PL",
	"south korea": "KR", "north korea": "KP", "pakistan": "PK",
	"saudi arabia": "SA", "egypt": "EG", "nigeria": "NG",
	"south africa": "ZA", "mexico": "MX", "indonesia": "ID",
	"netherlands": "NL", "belgium": "BE", "sweden": "SE",
	"norway": "NO", "switzerland": "CH", "austria": "AT",
	"greece": "GR", "romania": "RO", "hungary": "HU",
	"czech republic": "CZ", "portugal": "PT", "ireland": "IE",
	"denmark": "DK", "finland": "FI", "colombia": "CO",
	"argentina": "AR", "chile": "CL", "peru": "PE",
	"venezuela": "VE", "philippines": "PH", "thailand": "TH",
	"vietnam": "VN", "malaysia": "MY", "singapore": "SG",
	"taiwan": "TW", "syria": "SY", "lebanon": "LB",
	"jordan": "JO", "yemen": "YE", "afghanistan": "AF",
	"myanmar": "MM", "ethiopia": "ET", "kenya": "KE",
	"morocco": "MA", "algeria": "DZ", "tunisia": "TN",
	"libya": "LY", "sudan": "SD", "somalia": "SO",
}

// GDELTCountryISO2 converts a GDELT sourcecountry string to ISO2.
func GDELTCountryISO2(country string) string {
	return gdeltCountryToISO2[strings.ToLower(strings.TrimSpace(country))]
}
