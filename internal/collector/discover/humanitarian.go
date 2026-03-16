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

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
)

// HumanitarianOrg represents a humanitarian/emergency agency discovered via Wikidata.
type HumanitarianOrg struct {
	Name        string
	Country     string
	CountryCode string
	Website     string
}

var humanitarianTypeIDs = []string{
	"Q895526",
	"Q15220109",
	"Q1460420",
	"Q1066476",
}

func buildHumanitarianQuery(typeIDs []string) string {
	values := make([]string, 0, len(typeIDs))
	for _, typeID := range typeIDs {
		typeID = strings.TrimSpace(typeID)
		if typeID == "" {
			continue
		}
		values = append(values, "wd:"+typeID)
	}
	return fmt.Sprintf(`
SELECT ?org ?orgLabel ?website ?countryLabel ?countryCode WHERE {
  VALUES ?type { %s }
  ?org wdt:P31/wdt:P279* ?type ;
       wdt:P856 ?website ;
       wdt:P17 ?country .
  ?country wdt:P297 ?countryCode .
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . }
}
`, strings.Join(values, " "))
}

// FetchHumanitarianOrgs queries Wikidata for humanitarian, emergency management,
// and civil protection agencies worldwide.
func FetchHumanitarianOrgs(ctx context.Context, cfg config.Config, client *fetch.Client) ([]HumanitarianOrg, error) {
	var failures []string
	seen := map[string]struct{}{}
	var orgs []HumanitarianOrg

	for _, chunk := range chunkTypeIDs(humanitarianTypeIDs, 2) {
		query := strings.TrimSpace(buildHumanitarianQuery(chunk))
		reqURL := wikidataSPARQL + "?format=json&query=" + url.QueryEscape(query)
		body, err := fetchWikidataTextWithCache(ctx, cfg, client, reqURL, "application/sparql-results+json, application/json;q=0.9")
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", strings.Join(chunk, ","), err))
			continue
		}

		var resp struct {
			Results struct {
				Bindings []struct {
					OrgLabel     struct{ Value string } `json:"orgLabel"`
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

			orgs = append(orgs, HumanitarianOrg{
				Name:        b.OrgLabel.Value,
				Country:     b.CountryLabel.Value,
				CountryCode: strings.ToUpper(strings.TrimSpace(b.CountryCode.Value)),
				Website:     website,
			})
		}
	}
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].Country != orgs[j].Country {
			return orgs[i].Country < orgs[j].Country
		}
		return orgs[i].Name < orgs[j].Name
	})
	if len(orgs) > 0 {
		return orgs, nil
	}
	if len(failures) > 0 {
		return nil, fmt.Errorf("wikidata SPARQL humanitarian: %s", strings.Join(failures, " | "))
	}
	return nil, nil
}
