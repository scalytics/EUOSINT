// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package dictionary

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
)

type Entry struct {
	Strong   []string `json:"strong"`
	Weak     []string `json:"weak"`
	Negative []string `json:"negative"`
	URLHints []string `json:"url_hints"`
}

type CategoryDictionary struct {
	Default   Entry            `json:"default"`
	Languages map[string]Entry `json:"languages"`
}

type Document struct {
	Categories map[string]CategoryDictionary `json:"categories"`
}

type Store struct {
	doc Document
}

func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read category dictionary %s: %w", path, err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode category dictionary %s: %w", path, err)
	}
	if doc.Categories == nil {
		doc.Categories = map[string]CategoryDictionary{}
	}
	return &Store{doc: doc}, nil
}

func (s *Store) Match(category string, source model.RegistrySource, title string, link string) bool {
	if s == nil {
		return true
	}
	cat := s.doc.Categories[strings.ToLower(strings.TrimSpace(category))]
	combined := strings.ToLower(strings.TrimSpace(title + " " + link))
	urlOnly := strings.ToLower(strings.TrimSpace(link))

	positive := merge(cat.Default.Strong)
	negative := merge(cat.Default.Negative)
	urlHints := merge(cat.Default.URLHints)
	for _, lang := range inferLanguages(source) {
		entry, ok := cat.Languages[lang]
		if !ok {
			continue
		}
		positive = append(positive, entry.Strong...)
		positive = append(positive, entry.Weak...)
		negative = append(negative, entry.Negative...)
		urlHints = append(urlHints, entry.URLHints...)
	}

	if len(positive) == 0 && len(urlHints) == 0 && len(negative) == 0 {
		return true
	}
	for _, term := range negative {
		if contains(combined, term) {
			return false
		}
	}
	for _, term := range positive {
		if contains(combined, term) {
			return true
		}
	}
	for _, term := range urlHints {
		if contains(urlOnly, term) {
			return true
		}
	}
	return false
}

func inferLanguages(source model.RegistrySource) []string {
	set := map[string]struct{}{"default": {}}
	for _, code := range languagesForCountry(strings.ToUpper(strings.TrimSpace(source.Source.CountryCode))) {
		set[code] = struct{}{}
	}
	lowerFeedURL := strings.ToLower(source.FeedURL + " " + strings.Join(source.FeedURLs, " "))
	switch {
	case strings.Contains(lowerFeedURL, "idiomaactual=ca"), strings.Contains(lowerFeedURL, "/_ca/"):
		set["ca"] = struct{}{}
	case strings.Contains(lowerFeedURL, "idiomaactual=eu"):
		set["eu"] = struct{}{}
	case strings.Contains(lowerFeedURL, "idiomaactual=gl"):
		set["gl"] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for code := range set {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func languagesForCountry(countryCode string) []string {
	switch countryCode {
	case "ES", "MX", "AR", "CL", "CO", "CR", "GT", "SV", "HN", "NI", "PA", "PE", "UY", "PY", "VE", "BO", "DO", "CU", "EC":
		return []string{"es"}
	case "FR", "BE", "LU", "MC", "SN", "CI", "CM", "TN", "DZ", "MA":
		return []string{"fr"}
	case "DE", "AT":
		return []string{"de"}
	case "IT", "SM", "VA":
		return []string{"it"}
	case "PT", "BR", "AO", "MZ", "GW", "CV", "ST", "TL":
		return []string{"pt"}
	case "NL", "SR":
		return []string{"nl"}
	case "SE":
		return []string{"sv"}
	case "NO":
		return []string{"no"}
	case "DK":
		return []string{"da"}
	case "FI":
		return []string{"fi"}
	case "PL":
		return []string{"pl"}
	case "CZ":
		return []string{"cs"}
	case "SK":
		return []string{"sk"}
	case "SI":
		return []string{"sl"}
	case "HR", "BA", "RS", "ME":
		return []string{"hr", "sr"}
	case "RO", "MD":
		return []string{"ro"}
	case "HU":
		return []string{"hu"}
	case "LT":
		return []string{"lt"}
	case "LV":
		return []string{"lv"}
	case "EE":
		return []string{"et"}
	case "GR", "CY":
		return []string{"el"}
	case "TR":
		return []string{"tr"}
	case "UA":
		return []string{"uk"}
	case "RU", "BY", "KG":
		return []string{"ru"}
	case "GE":
		return []string{"ka"}
	case "AM":
		return []string{"hy"}
	case "IL":
		return []string{"he"}
	case "SA", "AE", "EG", "JO", "LB", "IQ", "QA", "KW", "OM", "BH":
		return []string{"ar"}
	case "IR":
		return []string{"fa"}
	case "IN":
		return []string{"hi", "en"}
	case "PK":
		return []string{"ur", "en"}
	case "BD":
		return []string{"bn"}
	case "LK":
		return []string{"si", "ta", "en"}
	case "NP":
		return []string{"ne"}
	case "CN":
		return []string{"zh"}
	case "TW":
		return []string{"zh-hant", "zh"}
	case "HK", "MO":
		return []string{"zh-hant", "zh", "en"}
	case "JP":
		return []string{"ja"}
	case "KR":
		return []string{"ko"}
	case "TH":
		return []string{"th"}
	case "VN":
		return []string{"vi"}
	case "ID":
		return []string{"id"}
	case "MY":
		return []string{"ms", "en"}
	case "PH":
		return []string{"fil", "en"}
	case "ZA":
		return []string{"en", "af"}
	case "KE", "UG", "TZ":
		return []string{"sw", "en"}
	default:
		return nil
	}
}

func merge(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func contains(haystack string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	return strings.Contains(haystack, needle)
}
