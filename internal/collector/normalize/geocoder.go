// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

// CityLookup abstracts the city database so normalize doesn't import sourcedb.
type CityLookup interface {
	LookupCity(ctx context.Context, name string, countryCode string) (CityLookupResult, bool)
}

// CityLookupResult mirrors sourcedb.CityResult without creating a dependency.
type CityLookupResult struct {
	Name        string
	CountryCode string
	Lat         float64
	Lng         float64
	Population  int
}

// GeoResult is the output of the full geocoding pipeline.
type GeoResult struct {
	Lat         float64
	Lng         float64
	CountryCode string
	CityName    string
	Source      string // "city-db", "nominatim", "country-text", "capital", "registry"
}

// Geocoder chains the three geocoding tiers:
// 1. City gazetteer (GeoNames in SQLite) — fast, local, city-level precision
// 2. Nominatim (OSM) — external fallback for place names not in the DB
// 3. Country-level (text scanning + capital coords) — always available
type Geocoder struct {
	cities    CityLookup       // may be nil
	nominatim *NominatimClient // may be nil
}

// NewGeocoder creates a geocoder. Both deps are optional — pass nil to skip.
func NewGeocoder(cities CityLookup, nominatim *NominatimClient) *Geocoder {
	return &Geocoder{cities: cities, nominatim: nominatim}
}

// wordBoundaryRe matches sequences of word characters for tokenizing text
// into potential city name candidates.
var wordBoundaryRe = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}\s'-]{2,30}`)

// Resolve geocodes a text string (typically alert title + summary) to
// the most precise coordinates available. countryHint is the source's
// country code (e.g. "DE") and helps disambiguate city names.
func (g *Geocoder) Resolve(ctx context.Context, text string, countryHint string) GeoResult {
	countryHint = strings.ToUpper(strings.TrimSpace(countryHint))

	// ── Tier 1: City gazetteer ──────────────────────────────────
	if g.cities != nil {
		if result, ok := g.matchCityInText(ctx, text, countryHint); ok {
			return result
		}
	}

	// ── Tier 2: Nominatim for extracted place-like tokens ───────
	if g.nominatim != nil {
		if result, ok := g.nominatimFromText(ctx, text, countryHint); ok {
			return result
		}
	}

	// ── Tier 3: Country-level from text ─────────────────────────
	if lat, lng, code, ok := geocodeText(text); ok {
		// Use capital coords instead of centroid.
		if capital, cok := capitalCoords[code]; cok {
			return GeoResult{Lat: capital[0], Lng: capital[1], CountryCode: code, Source: "capital"}
		}
		return GeoResult{Lat: lat, Lng: lng, CountryCode: code, Source: "country-text"}
	}

	// ── Fallback: use country hint's capital ────────────────────
	if countryHint != "" && countryHint != "INT" {
		if capital, ok := capitalCoords[countryHint]; ok {
			return GeoResult{Lat: capital[0], Lng: capital[1], CountryCode: countryHint, Source: "capital"}
		}
	}

	return GeoResult{} // no match
}

// matchCityInText extracts candidate n-grams from text and looks them up
// in the city database. Returns the match with the highest population that
// appears rightmost in the text (consistent with geocodeText strategy).
func (g *Geocoder) matchCityInText(ctx context.Context, text string, countryHint string) (GeoResult, bool) {
	candidates := extractCandidateNames(text)
	if len(candidates) == 0 {
		return GeoResult{}, false
	}

	type hit struct {
		pos  int
		pop  int
		name string
		lat  float64
		lng  float64
		code string
	}
	var best *hit

	for _, c := range candidates {
		result, ok := g.cities.LookupCity(ctx, c.name, countryHint)
		if !ok {
			continue
		}
		// Skip tiny places (pop < 5000) unless they match country hint.
		if result.Population < 5000 && result.CountryCode != countryHint {
			continue
		}
		h := hit{
			pos:  c.endPos,
			pop:  result.Population,
			name: result.Name,
			lat:  result.Lat,
			lng:  result.Lng,
			code: result.CountryCode,
		}
		if best == nil ||
			h.pos > best.pos ||
			(h.pos == best.pos && h.pop > best.pop) {
			best = &h
		}
	}

	if best != nil {
		return GeoResult{
			Lat:         best.lat,
			Lng:         best.lng,
			CountryCode: best.code,
			CityName:    best.name,
			Source:      "city-db",
		}, true
	}
	return GeoResult{}, false
}

type nameCandidate struct {
	name   string
	endPos int
}

// extractCandidateNames pulls potential place names from text. It extracts
// capitalized word sequences (1-4 words) which is how city names typically
// appear in headlines. E.g. "Explosion in San Francisco kills 3" →
// ["Explosion", "San Francisco", "San Francisco kills"].
func extractCandidateNames(text string) []nameCandidate {
	words := tokenizeWords(text)
	if len(words) == 0 {
		return nil
	}

	var candidates []nameCandidate
	seen := map[string]struct{}{}

	// Single words and multi-word sequences (up to 4 words).
	for i := range words {
		for n := 1; n <= 4 && i+n <= len(words); n++ {
			var parts []string
			allEmpty := true
			for j := i; j < i+n; j++ {
				w := words[j]
				if w.text == "" {
					break
				}
				allEmpty = false
				parts = append(parts, w.text)
			}
			if allEmpty || len(parts) != n {
				break
			}
			name := strings.Join(parts, " ")
			lower := strings.ToLower(name)

			// Skip very short single words and common noise.
			if n == 1 && len(name) < 3 {
				continue
			}
			if isGeoStopword(lower) {
				continue
			}

			if _, ok := seen[lower]; ok {
				continue
			}
			seen[lower] = struct{}{}
			candidates = append(candidates, nameCandidate{
				name:   name,
				endPos: words[i+n-1].endPos,
			})
		}
	}
	return candidates
}

type wordToken struct {
	text   string
	endPos int
}

func tokenizeWords(text string) []wordToken {
	var tokens []wordToken
	inWord := false
	start := 0
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-' || r == '.' {
			if !inWord {
				start = i
				inWord = true
			}
		} else {
			if inWord {
				tokens = append(tokens, wordToken{text: text[start:i], endPos: i})
				inWord = false
			}
		}
	}
	if inWord {
		tokens = append(tokens, wordToken{text: text[start:], endPos: len(text)})
	}
	return tokens
}

var geoStopwords = map[string]bool{
	// Common English words that are also city/place names but are almost
	// never geographic references in OSINT headlines.
	"the": true, "and": true, "for": true, "new": true, "has": true,
	"was": true, "are": true, "with": true, "from": true, "that": true,
	"this": true, "not": true, "but": true, "all": true, "its": true,
	"will": true, "can": true, "more": true, "update": true, "alert": true,
	"warning": true, "report": true, "press": true, "release": true,
	"security": true, "advisory": true, "notice": true, "bulletin": true,
	"critical": true, "high": true, "medium": true, "low": true, "info": true,
}

func isGeoStopword(lower string) bool {
	return geoStopwords[lower]
}

// nominatimFromText tries Nominatim for capitalized multi-word tokens that
// look like place names. Only attempts a few lookups to stay within rate limits.
func (g *Geocoder) nominatimFromText(ctx context.Context, text string, countryHint string) (GeoResult, bool) {
	candidates := extractCandidateNames(text)
	if len(candidates) == 0 {
		return GeoResult{}, false
	}

	// Only try the last few candidates (rightmost = most likely geographic).
	maxAttempts := 3
	start := len(candidates) - maxAttempts
	if start < 0 {
		start = 0
	}

	for i := len(candidates) - 1; i >= start; i-- {
		c := candidates[i]
		if len(c.name) < 4 {
			continue
		}
		result, ok := g.nominatim.Geocode(ctx, c.name, countryHint)
		if ok {
			return GeoResult{
				Lat:         result.Lat,
				Lng:         result.Lng,
				CountryCode: result.CountryCode,
				CityName:    c.name,
				Source:      "nominatim",
			}, true
		}
	}
	return GeoResult{}, false
}
