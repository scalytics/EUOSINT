// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/scalytics/euosint/internal/collector/fetch"
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
	"Q732717",
	"Q35535",
	"Q15636005",
	"Q584085",
	"Q1752939",
	"Q12039646",
}

func buildPoliceAgencyQuery(typeIDs []string) string {
	values := make([]string, 0, len(typeIDs))
	for _, typeID := range typeIDs {
		typeID = strings.TrimSpace(typeID)
		if typeID == "" {
			continue
		}
		values = append(values, "wd:"+typeID)
	}
	return fmt.Sprintf(`
SELECT ?agency ?agencyLabel ?website ?countryLabel ?countryCode WHERE {
  VALUES ?type { %s }
  ?agency wdt:P31/wdt:P279* ?type ;
          wdt:P856 ?website ;
          wdt:P17 ?country .
  ?country wdt:P297 ?countryCode .
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . }
}
`, strings.Join(values, " "))
}

// FetchPoliceAgencies queries Wikidata for law-enforcement agencies
// worldwide that have official websites. This replaces a static curated
// directory and scales to every country automatically.
func FetchPoliceAgencies(ctx context.Context, client *fetch.Client) ([]PoliceAgency, error) {
	// Query Wikidata in smaller chunks so one slow response does not zero out
	// the entire law-enforcement directory.
	seen := map[string]struct{}{}
	var agencies []PoliceAgency
	var failures []string

	for _, chunk := range chunkTypeIDs(policeAgencyTypeIDs, 2) {
		query := strings.TrimSpace(buildPoliceAgencyQuery(chunk))
		reqURL := wikidataSPARQL + "?format=json&query=" + url.QueryEscape(query)
		body, err := fetchTextWithRetry(ctx, client, reqURL, "application/sparql-results+json, application/json;q=0.9")
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", strings.Join(chunk, ","), err))
			continue
		}

		var resp struct {
			Results struct {
				Bindings []struct {
					AgencyLabel  struct{ Value string } `json:"agencyLabel"`
					Website      struct{ Value string } `json:"website"`
					CountryLabel struct{ Value string } `json:"countryLabel"`
					CountryCode  struct{ Value string } `json:"countryCode"`
				} `json:"bindings"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			failures = append(failures, fmt.Sprintf("%s: parse: %v", strings.Join(chunk, ","), err))
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
				Name:              b.AgencyLabel.Value,
				Country:           b.CountryLabel.Value,
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

func chunkTypeIDs(values []string, size int) [][]string {
	if size <= 0 {
		size = len(values)
	}
	var chunks [][]string
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		chunk := append([]string(nil), values[start:end]...)
		chunks = append(chunks, chunk)
	}
	return chunks
}
