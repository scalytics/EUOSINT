// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"fmt"
	"io"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
)

// gapTarget defines a country and the minimum categories it should have.
type gapTarget struct {
	Country     string
	CountryCode string
	Categories  []string
}

// coreCategories is the minimum set of feed categories every country should have.
var coreCategories = []string{
	"cyber_advisory",
	"public_appeal",
}

// expandedCategories adds intel, travel, and fraud for countries where these
// are commonly published by government agencies.
var expandedCategories = []string{
	"cyber_advisory",
	"public_appeal",
	"travel_warning",
	"intelligence_report",
	"fraud_alert",
}

// targetCountries lists all countries the system should cover with at least
// core categories. European/Nordic countries use the expanded set.
var targetCountries = []gapTarget{
	// ─── Nordics ────────────────────────────────────────────────
	{"Norway", "NO", expandedCategories},
	{"Sweden", "SE", expandedCategories},
	{"Finland", "FI", expandedCategories},
	{"Denmark", "DK", expandedCategories},
	{"Iceland", "IS", coreCategories},

	// ─── Western Europe ─────────────────────────────────────────
	{"United Kingdom", "GB", expandedCategories},
	{"France", "FR", expandedCategories},
	{"Germany", "DE", expandedCategories},
	{"Netherlands", "NL", expandedCategories},
	{"Belgium", "BE", expandedCategories},
	{"Luxembourg", "LU", coreCategories},
	{"Ireland", "IE", expandedCategories},
	{"Switzerland", "CH", expandedCategories},
	{"Austria", "AT", expandedCategories},

	// ─── Southern Europe ────────────────────────────────────────
	{"Spain", "ES", expandedCategories},
	{"Portugal", "PT", expandedCategories},
	{"Italy", "IT", expandedCategories},
	{"Greece", "GR", expandedCategories},
	{"Malta", "MT", coreCategories},
	{"Cyprus", "CY", coreCategories},

	// ─── Eastern Europe ─────────────────────────────────────────
	{"Poland", "PL", expandedCategories},
	{"Czech Republic", "CZ", expandedCategories},
	{"Slovakia", "SK", coreCategories},
	{"Hungary", "HU", coreCategories},
	{"Romania", "RO", expandedCategories},
	{"Bulgaria", "BG", coreCategories},
	{"Croatia", "HR", coreCategories},
	{"Slovenia", "SI", coreCategories},
	{"Serbia", "RS", coreCategories},
	{"Bosnia", "BA", coreCategories},
	{"Montenegro", "ME", coreCategories},
	{"North Macedonia", "MK", coreCategories},
	{"Albania", "AL", coreCategories},
	{"Kosovo", "XK", coreCategories},
	{"Moldova", "MD", coreCategories},

	// ─── Baltics ────────────────────────────────────────────────
	{"Estonia", "EE", expandedCategories},
	{"Latvia", "LV", expandedCategories},
	{"Lithuania", "LT", expandedCategories},

	// ─── East ───────────────────────────────────────────────────
	{"Ukraine", "UA", expandedCategories},
	{"Georgia", "GE", coreCategories},
	{"Turkey", "TR", expandedCategories},

	// ─── Americas ───────────────────────────────────────────────
	{"United States", "US", expandedCategories},
	{"Canada", "CA", expandedCategories},
	{"Mexico", "MX", expandedCategories},
	{"Brazil", "BR", expandedCategories},
	{"Argentina", "AR", expandedCategories},
	{"Colombia", "CO", coreCategories},
	{"Chile", "CL", coreCategories},
	{"Peru", "PE", coreCategories},
	{"Ecuador", "EC", coreCategories},
	{"Venezuela", "VE", coreCategories},
	{"Cuba", "CU", coreCategories},
	{"Panama", "PA", coreCategories},
	{"Costa Rica", "CR", coreCategories},
	{"Guatemala", "GT", coreCategories},
	{"Honduras", "HN", coreCategories},
	{"El Salvador", "SV", coreCategories},
	{"Nicaragua", "NI", coreCategories},
	{"Dominican Republic", "DO", coreCategories},
	{"Jamaica", "JM", coreCategories},
	{"Haiti", "HT", coreCategories},
	{"Paraguay", "PY", coreCategories},
	{"Uruguay", "UY", coreCategories},
	{"Bolivia", "BO", coreCategories},

	// ─── Asia-Pacific ───────────────────────────────────────────
	{"Japan", "JP", expandedCategories},
	{"South Korea", "KR", expandedCategories},
	{"China", "CN", expandedCategories},
	{"India", "IN", expandedCategories},
	{"Australia", "AU", expandedCategories},
	{"New Zealand", "NZ", coreCategories},
	{"Indonesia", "ID", expandedCategories},
	{"Malaysia", "MY", coreCategories},
	{"Singapore", "SG", expandedCategories},
	{"Thailand", "TH", coreCategories},
	{"Philippines", "PH", coreCategories},
	{"Vietnam", "VN", coreCategories},
	{"Taiwan", "TW", expandedCategories},
	{"Pakistan", "PK", coreCategories},
	{"Bangladesh", "BD", coreCategories},
	{"Sri Lanka", "LK", coreCategories},
	{"Nepal", "NP", coreCategories},
	{"Mongolia", "MN", coreCategories},
	{"Cambodia", "KH", coreCategories},
	{"Myanmar", "MM", coreCategories},
	{"Laos", "LA", coreCategories},

	// ─── Middle East ────────────────────────────────────────────
	{"Israel", "IL", expandedCategories},
	{"Saudi Arabia", "SA", expandedCategories},
	{"United Arab Emirates", "AE", expandedCategories},
	{"Qatar", "QA", coreCategories},
	{"Kuwait", "KW", coreCategories},
	{"Bahrain", "BH", coreCategories},
	{"Oman", "OM", coreCategories},
	{"Jordan", "JO", coreCategories},
	{"Lebanon", "LB", coreCategories},
	{"Iraq", "IQ", coreCategories},
	{"Iran", "IR", expandedCategories},
	{"Syria", "SY", coreCategories},
	{"Yemen", "YE", coreCategories},
	{"Palestine", "PS", coreCategories},

	// ─── Africa ─────────────────────────────────────────────────
	{"South Africa", "ZA", expandedCategories},
	{"Nigeria", "NG", expandedCategories},
	{"Kenya", "KE", coreCategories},
	{"Egypt", "EG", expandedCategories},
	{"Morocco", "MA", coreCategories},
	{"Algeria", "DZ", coreCategories},
	{"Tunisia", "TN", coreCategories},
	{"Libya", "LY", coreCategories},
	{"Ethiopia", "ET", coreCategories},
	{"Ghana", "GH", coreCategories},
	{"Tanzania", "TZ", coreCategories},
	{"Uganda", "UG", coreCategories},
	{"Rwanda", "RW", coreCategories},
	{"Senegal", "SN", coreCategories},
	{"Ivory Coast", "CI", coreCategories},
	{"Cameroon", "CM", coreCategories},
	{"Congo", "CD", coreCategories},
	{"Mozambique", "MZ", coreCategories},
	{"Zimbabwe", "ZW", coreCategories},
	{"Zambia", "ZM", coreCategories},
	{"Mali", "ML", coreCategories},
	{"Burkina Faso", "BF", coreCategories},
	{"Niger", "NE", coreCategories},
	{"Chad", "TD", coreCategories},
	{"Sudan", "SD", coreCategories},
	{"South Sudan", "SS", coreCategories},
	{"Somalia", "SO", coreCategories},
	{"Eritrea", "ER", coreCategories},
	{"Madagascar", "MG", coreCategories},
	{"Angola", "AO", coreCategories},
	{"Namibia", "NA", coreCategories},
	{"Botswana", "BW", coreCategories},

	// ─── Central Asia ───────────────────────────────────────────
	{"Kazakhstan", "KZ", coreCategories},
	{"Uzbekistan", "UZ", coreCategories},
	{"Kyrgyzstan", "KG", coreCategories},
	{"Tajikistan", "TJ", coreCategories},
	{"Turkmenistan", "TM", coreCategories},

	// ─── Caucasus ───────────────────────────────────────────────
	{"Armenia", "AM", coreCategories},
	{"Azerbaijan", "AZ", coreCategories},

	// ─── Russia / Belarus ───────────────────────────────────────
	{"Russia", "RU", expandedCategories},
	{"Belarus", "BY", coreCategories},
}

// gapCandidate is a synthetic search candidate generated by gap analysis.
type gapCandidate struct {
	Country     string
	CountryCode string
	Category    string
}

// AnalyzeGaps compares the active registry against target countries and
// returns synthetic candidates for missing country+category combinations.
func AnalyzeGaps(sources []model.RegistrySource, stderr io.Writer) []model.SourceCandidate {
	// Build coverage map: country_code → set of categories with active feeds.
	coverage := map[string]map[string]bool{}
	for _, src := range sources {
		cc := strings.ToUpper(src.Source.CountryCode)
		if cc == "" || cc == "INT" {
			continue
		}
		if coverage[cc] == nil {
			coverage[cc] = map[string]bool{}
		}
		coverage[cc][src.Category] = true
	}

	var gaps []gapCandidate
	for _, target := range targetCountries {
		covered := coverage[target.CountryCode]
		for _, cat := range target.Categories {
			if covered != nil && covered[cat] {
				continue
			}
			gaps = append(gaps, gapCandidate{
				Country:     target.Country,
				CountryCode: target.CountryCode,
				Category:    cat,
			})
		}
	}

	if len(gaps) == 0 {
		fmt.Fprintf(stderr, "Gap analysis: full coverage for all %d target countries\n", len(targetCountries))
		return nil
	}

	fmt.Fprintf(stderr, "Gap analysis: found %d missing country+category combinations across %d target countries\n", len(gaps), countUniqueCountries(gaps))

	// Convert gaps to search candidates with descriptive names.
	candidates := make([]model.SourceCandidate, 0, len(gaps))
	for _, gap := range gaps {
		candidates = append(candidates, model.SourceCandidate{
			AuthorityName: gapAuthorityLabel(gap.Country, gap.Category),
			AuthorityType: gapAuthorityType(gap.Category),
			Category:      gap.Category,
			Country:       gap.Country,
			CountryCode:   gap.CountryCode,
			Notes:         "autonomous seed: gap-analysis",
		})
	}
	return candidates
}

func countUniqueCountries(gaps []gapCandidate) int {
	seen := map[string]bool{}
	for _, g := range gaps {
		seen[g.CountryCode] = true
	}
	return len(seen)
}

// gapAuthorityLabel generates a descriptive search label for a gap.
func gapAuthorityLabel(country string, category string) string {
	switch category {
	case "cyber_advisory":
		return country + " national CERT or CSIRT"
	case "public_appeal":
		return country + " national police"
	case "travel_warning":
		return country + " ministry of foreign affairs"
	case "intelligence_report":
		return country + " intelligence or security service"
	case "fraud_alert":
		return country + " financial regulator or central bank"
	case "wanted_suspect":
		return country + " wanted persons"
	default:
		return country + " " + strings.ReplaceAll(category, "_", " ")
	}
}

func gapAuthorityType(category string) string {
	switch category {
	case "cyber_advisory":
		return "cert"
	case "public_appeal", "wanted_suspect":
		return "police"
	case "travel_warning":
		return "government"
	case "intelligence_report":
		return "national_security"
	case "fraud_alert":
		return "regulatory"
	default:
		return "government"
	}
}
