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

// GeoLLM resolves a location description to coordinates via an LLM.
type GeoLLM interface {
	GeoLocate(ctx context.Context, query string) (lat, lng float64, ok bool)
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

// Geocoder chains the geocoding tiers:
// 1. Explicit coordinates in text
// 2. Maritime region lookup
// 3. City gazetteer (GeoNames in SQLite)
// 4. Nominatim (OSM)
// 5. Country-level (text scanning + capital coords)
// 6. LLM fallback (optional, for unresolvable locations)
type Geocoder struct {
	cities    CityLookup       // may be nil
	nominatim *NominatimClient // may be nil
	llm       GeoLLM           // may be nil
}

// NewGeocoder creates a geocoder. All deps are optional — pass nil to skip.
func NewGeocoder(cities CityLookup, nominatim *NominatimClient) *Geocoder {
	return &Geocoder{cities: cities, nominatim: nominatim}
}

// SetLLM adds an LLM geocoding fallback for locations that can't be resolved
// by coordinates, maritime regions, city DB, Nominatim, or country text.
func (g *Geocoder) SetLLM(llm GeoLLM) {
	g.llm = llm
}

// wordBoundaryRe matches sequences of word characters for tokenizing text
// into potential city name candidates.
var wordBoundaryRe = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}\s'-]{2,30}`)

// Resolve geocodes a text string (typically alert title + summary) to
// the most precise coordinates available. countryHint is the source's
// country code (e.g. "DE") and helps disambiguate city names.
func (g *Geocoder) Resolve(ctx context.Context, text string, countryHint string) GeoResult {
	countryHint = strings.ToUpper(strings.TrimSpace(countryHint))

	// ── Tier 0a: Explicit coordinates in text ───────────────────
	if lat, lng, ok := ExtractCoordinates(text); ok {
		// Try to resolve country from the coordinates via nearest country.
		code := countryHint
		if code == "" || code == "INT" {
			if _, _, c, cok := geocodeText(text); cok {
				code = c
			}
		}
		return GeoResult{Lat: lat, Lng: lng, CountryCode: code, Source: "coordinates"}
	}

	// ── Tier 0b: Maritime region lookup ─────────────────────────
	if lat, lng, _, ok := MatchMaritimeRegion(text); ok {
		code := countryHint
		if code == "" || code == "INT" {
			if _, _, c, cok := geocodeText(text); cok {
				code = c
			}
		}
		return GeoResult{Lat: lat, Lng: lng, CountryCode: code, Source: "maritime-region"}
	}

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

	// ── Tier 4: LLM fallback for unresolvable locations ────────
	if g.llm != nil {
		if lat, lng, ok := g.llm.GeoLocate(ctx, text); ok {
			code := countryHint
			if code == "" || code == "INT" {
				if _, _, c, cok := geocodeText(text); cok {
					code = c
				}
			}
			return GeoResult{Lat: lat, Lng: lng, CountryCode: code, Source: "llm"}
		}
	}

	// ── Fallback: use country hint's capital ────────────────────
	if countryHint != "" && countryHint != "INT" {
		if capital, ok := capitalCoords[countryHint]; ok {
			return GeoResult{Lat: capital[0], Lng: capital[1], CountryCode: countryHint, Source: "capital"}
		}
	}

	return GeoResult{} // no match
}

// hitBetter returns true if a is a better geocoding match than b.
// Country-hint matches always win over non-hint matches. Within the same
// hint tier, rightmost position wins, then highest population.
func hitBetter(a, b struct {
	pos         int
	pop         int
	name        string
	lat         float64
	lng         float64
	code        string
	matchesHint bool
}) bool {
	// Hint match always beats non-hint match.
	if a.matchesHint != b.matchesHint {
		return a.matchesHint
	}
	// Within same tier: rightmost position wins.
	if a.pos != b.pos {
		return a.pos > b.pos
	}
	// Tiebreak: larger city wins.
	return a.pop > b.pop
}

// matchCityInText extracts candidate n-grams from text and looks them up
// in the city database. Returns the best match, preferring country-hint
// matches, then rightmost position, then highest population.
func (g *Geocoder) matchCityInText(ctx context.Context, text string, countryHint string) (GeoResult, bool) {
	candidates := extractCandidateNames(text)
	if len(candidates) == 0 {
		return GeoResult{}, false
	}

	type hit struct {
		pos         int
		pop         int
		name        string
		lat         float64
		lng         float64
		code        string
		matchesHint bool
	}
	var best *hit

	for _, c := range candidates {
		result, ok := g.cities.LookupCity(ctx, c.name, countryHint)
		if !ok {
			continue
		}
		matchesHint := countryHint != "" && result.CountryCode == countryHint

		// Cross-country matches need high population to avoid false positives
		// (e.g. "Police" → Połice, Poland; "Malta" → Malta, Brazil).
		if !matchesHint && result.Population < 50000 {
			continue
		}

		h := hit{
			pos:         c.endPos,
			pop:         result.Population,
			name:        result.Name,
			lat:         result.Lat,
			lng:         result.Lng,
			code:        result.CountryCode,
			matchesHint: matchesHint,
		}
		if best == nil || hitBetter(h, *best) {
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
			// Keep city matching focused on proper nouns. Without this guard,
			// lower-case advisory prose produces huge candidate sets and
			// expensive DB lookups (e.g. KEV/cyber feeds).
			if !looksLikePlaceSequence(parts) {
				continue
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

func looksLikePlaceSequence(parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	properCount := 0
	for _, p := range parts {
		if isCapitalizedToken(p) || isAllCapsAbbreviation(p) {
			properCount++
		}
	}
	// Require at least one proper-looking token for single words, and at
	// least half for multi-word phrases.
	if len(parts) == 1 {
		return properCount == 1
	}
	return properCount*2 >= len(parts)
}

func isCapitalizedToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	for _, r := range token {
		return unicode.IsUpper(r)
	}
	return false
}

func isAllCapsAbbreviation(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 6 {
		return false
	}
	hasLetter := false
	for _, r := range token {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
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
	// Institutional/authority terms that match city/village names worldwide.
	"police": true, "national": true, "federal": true, "general": true,
	"public": true, "central": true, "royal": true, "state": true,
	"justice": true, "defense": true, "defence": true, "interior": true,
	"ministry": true, "department": true, "office": true, "bureau": true,
	"agency": true, "service": true, "command": true, "force": true,
	"unit": true, "division": true, "section": true, "branch": true,
	"council": true, "commission": true, "authority": true, "board": true,
	// Common first/last names that are also place names.
	"mark": true, "lee": true, "jordan": true, "chase": true,
	"grant": true, "Hope": true, "hope": true, "reading": true,
	"florence": true, "victoria": true, "augusta": true, "regina": true,
	"lincoln": true, "jackson": true, "clinton": true, "hamilton": true,
	"nelson": true, "marshall": true, "stuart": true, "douglas": true,
	"orange": true, "mobile": true, "enterprise": true, "summit": true,
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
