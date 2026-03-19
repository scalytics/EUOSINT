// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"net/url"
	"strings"
	"unicode"
)

var genericNewsroomTerms = []string{
	"newsroom",
	"press office",
	"media centre",
	"media center",
	"communications office",
}

// nonOSINTTerms catches organizations that have no intelligence relevance.
// Matched against lowered name and hostname.
var nonOSINTTerms = []string{
	"school", "university", "college", "academy", "education",
	"world bank", "imf", "monetary fund",
	"library", "museum", "archive",
	"tourism", "tourist", "travel agency",
	"sport", "olympic", "football", "soccer", "fifa",
	"entertainment", "oscars", "grammy", "billboard",
	"recipe", "cooking", "food network",
	"weather forecast", "meteorolog",
	"real estate", "property",
	"fashion", "beauty", "lifestyle",
	"church", "mosque", "synagogue", "cathedral",
	"kindergarten", "daycare", "nursery",
	"zoo", "aquarium", "botanical",
	"lottery", "casino", "gambling",
	"dating", "matrimon",
	"openstreetmap", "missing maps", "mapathon", "tasking manager",
}

// nonOSINTHosts rejects entire domains that are never OSINT-relevant.
var nonOSINTHosts = []string{
	"worldbank.org", "imf.org",
	"unesco.org", "unicef.org",
	"wikipedia.org", "wiktionary.org",
	"facebook.com", "twitter.com", "x.com", "instagram.com",
	"youtube.com", "tiktok.com", "reddit.com",
	"linkedin.com", "pinterest.com",
	"amazon.com", "ebay.com", "alibaba.com",
	"spotify.com", "netflix.com",
	"stackoverflow.com", "github.com",
	"schoolnet.eu", "european-schoolnet",
	"openstreetmap.org", "hotosm.org", "missingmaps.org",
}

func passesDiscoveryHygiene(name string, website string, authorityType string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	authorityType = strings.ToLower(strings.TrimSpace(authorityType))
	if name == "" {
		return false
	}
	for _, term := range nonOSINTTerms {
		if containsTerm(name, term) {
			return false
		}
	}
	if authorityType == "police" {
		for _, term := range genericNewsroomTerms {
			if strings.Contains(name, term) {
				return false
			}
		}
	}
	if hostLooksLocal(website) || hostIsNonOSINT(website) {
		return false
	}
	return true
}

func hostLooksLocal(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	return strings.HasPrefix(host, "city.") ||
		strings.HasPrefix(host, "county.") ||
		strings.Contains(host, ".city.") ||
		strings.Contains(host, ".county.") ||
		strings.Contains(host, ".municipal.")
}

func hostIsNonOSINT(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, blocked := range nonOSINTHosts {
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}
	}
	return false
}

func containsTerm(text string, term string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	term = strings.ToLower(strings.TrimSpace(term))
	if text == "" || term == "" {
		return false
	}
	offset := 0
	for {
		idx := strings.Index(text[offset:], term)
		if idx < 0 {
			return false
		}
		start := offset + idx
		end := start + len(term)
		if isBoundary(text, start-1) && isBoundary(text, end) {
			return true
		}
		offset = start + 1
	}
}

func isBoundary(text string, idx int) bool {
	if idx < 0 || idx >= len(text) {
		return true
	}
	r := rune(text[idx])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}
