// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/fetch"
)

const wikidataSPARQL = "https://query.wikidata.org/sparql"

// PoliceAgency represents a law-enforcement or national-security agency
// discovered via Wikidata whose website we should probe for feeds.
type PoliceAgency struct {
	Name              string
	Country           string
	CountryCode       string
	Website           string
	AuthorityType     string   // "police", "national_security"
	Category          string   // suggested registry category
	PressReleasePaths []string // fallback HTML paths to probe if no RSS found
}

// commonPressReleasePaths are subpaths frequently used by police/government
// sites for press-release listing pages.
var commonPressReleasePaths = []string{
	"/en/resources/press-releases/",
	"/en/press-releases/",
	"/en/news/",
	"/en/media/press-releases/",
	"/press-releases/",
	"/news/press-releases/",
	"/newsroom/press-releases/",
	"/media/news/",
	"/latest-news/",
	"/news/",
}

var policeAgencyTypeIDs = []string{
	"Q732717",   // police
	"Q35535",    // gendarmerie
	"Q15636005", // intelligence agency
	"Q584085",   // coast guard
	"Q1752939",  // customs
	"Q12039646", // secret police
	// Subclasses we'd miss without P279* traversal:
	"Q17032608", // law enforcement agency
	"Q56318653", // national police
	"Q19832486", // federal police
	"Q2102290",  // border guard
	"Q15925165", // national security agency
	"Q68416",    // national guard
	"Q189290",   // military police
	"Q7188",     // security service
}

func buildPoliceAgencyQuery(typeID string) string {
	// Query ONE type ID at a time with P31 (no P279* subclass traversal).
	// No label service, no rdfs:label OPTIONAL — both cause Wikidata
	// timeouts. We extract the hostname as a proxy for the org name.
	return fmt.Sprintf(`
SELECT ?website ?countryCode WHERE {
  ?agency wdt:P31 wd:%s ;
          wdt:P856 ?website ;
          wdt:P17 ?country .
  ?country wdt:P297 ?countryCode .
} LIMIT 50
`, strings.TrimSpace(typeID))
}

// FetchPoliceAgencies queries Wikidata for law-enforcement agencies
// worldwide that have official websites. Queries one type ID at a time
// with LIMIT 50 to stay within Wikidata's public SPARQL timeout limits.
func FetchPoliceAgencies(ctx context.Context, cfg config.Config, client *fetch.Client) ([]PoliceAgency, error) {
	seen := map[string]struct{}{}
	var agencies []PoliceAgency
	var failures []string

	for _, typeID := range policeAgencyTypeIDs {
		if ctx.Err() != nil {
			break
		}
		query := strings.TrimSpace(buildPoliceAgencyQuery(typeID))
		reqURL := wikidataSPARQL + "?format=json&query=" + url.QueryEscape(query)
		body, err := fetchWikidataTextWithCache(ctx, cfg, client, reqURL, "application/sparql-results+json, application/json;q=0.9")
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", typeID, err))
			continue
		}

		var resp struct {
			Results struct {
				Bindings []struct {
					Website     struct{ Value string } `json:"website"`
					CountryCode struct{ Value string } `json:"countryCode"`
				} `json:"bindings"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			failures = append(failures, fmt.Sprintf("%s: parse: %v", typeID, err))
			continue
		}

		for _, b := range resp.Results.Bindings {
			website := strings.TrimRight(strings.TrimSpace(b.Website.Value), "/")
			if website == "" {
				continue
			}
			u, err := url.Parse(website)
			if err != nil {
				continue
			}
			host := strings.ToLower(u.Hostname())
			if _, ok := seen[host]; ok {
				continue
			}
			seen[host] = struct{}{}

			agencies = append(agencies, PoliceAgency{
				Name:              hostToName(host),
				Country:           countryFromCode(b.CountryCode.Value),
				CountryCode:       strings.ToUpper(strings.TrimSpace(b.CountryCode.Value)),
				Website:           website,
				AuthorityType:     "police",
				Category:          "public_appeal",
				PressReleasePaths: commonPressReleasePaths,
			})
		}
	}

	sort.Slice(agencies, func(i, j int) bool {
		if agencies[i].Country != agencies[j].Country {
			return agencies[i].Country < agencies[j].Country
		}
		return agencies[i].Name < agencies[j].Name
	})
	if len(agencies) > 0 {
		return agencies, nil
	}
	if len(failures) > 0 {
		return nil, fmt.Errorf("wikidata SPARQL: %s", strings.Join(failures, " | "))
	}
	return nil, nil
}

// hostToName derives a readable name from a hostname.
// e.g. "www.politi.dk" → "politi.dk"
func hostToName(host string) string {
	host = strings.TrimPrefix(host, "www.")
	return host
}

// countryFromCode returns a country name for a code, or the code itself.
func countryFromCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	countries := map[string]string{
		"NO": "Norway", "SE": "Sweden", "FI": "Finland", "DK": "Denmark", "IS": "Iceland",
		"GB": "United Kingdom", "FR": "France", "DE": "Germany", "NL": "Netherlands",
		"BE": "Belgium", "LU": "Luxembourg", "IE": "Ireland", "CH": "Switzerland",
		"AT": "Austria", "ES": "Spain", "PT": "Portugal", "IT": "Italy", "GR": "Greece",
		"MT": "Malta", "CY": "Cyprus", "PL": "Poland", "CZ": "Czech Republic",
		"SK": "Slovakia", "HU": "Hungary", "RO": "Romania", "BG": "Bulgaria",
		"HR": "Croatia", "SI": "Slovenia", "RS": "Serbia", "BA": "Bosnia",
		"ME": "Montenegro", "MK": "North Macedonia", "AL": "Albania", "XK": "Kosovo",
		"EE": "Estonia", "LV": "Latvia", "LT": "Lithuania", "UA": "Ukraine",
		"GE": "Georgia", "TR": "Turkey", "US": "United States", "CA": "Canada",
		"AU": "Australia", "NZ": "New Zealand", "JP": "Japan", "KR": "South Korea",
		"CN": "China", "IN": "India", "BR": "Brazil", "MX": "Mexico",
		"IL": "Israel", "SA": "Saudi Arabia", "AE": "United Arab Emirates",
		"RU": "Russia", "BY": "Belarus", "MD": "Moldova",
		"ZA": "South Africa", "NG": "Nigeria", "KE": "Kenya", "EG": "Egypt",
		"ID": "Indonesia", "MY": "Malaysia", "SG": "Singapore", "TH": "Thailand",
		"PH": "Philippines", "PK": "Pakistan", "AR": "Argentina", "CO": "Colombia",
	}
	if name, ok := countries[code]; ok {
		return name
	}
	return code
}
