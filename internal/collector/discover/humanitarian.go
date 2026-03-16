// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/scalytics/euosint/internal/collector/fetch"
)

// HumanitarianOrg represents a humanitarian/emergency agency discovered via Wikidata.
type HumanitarianOrg struct {
	Name        string
	Country     string
	CountryCode string
	Website     string
}

// The SPARQL query finds humanitarian, disaster-relief, and emergency management
// agencies with official websites. Covers:
//   - Emergency management agency (Q895526)
//   - Humanitarian aid organization (Q15220109)
//   - Disaster management (Q1460420)
//   - Civil protection (Q1066476)
const humanitarianQuery = `
SELECT ?org ?orgLabel ?website ?countryLabel ?countryCode WHERE {
  VALUES ?type {
    wd:Q895526
    wd:Q15220109
    wd:Q1460420
    wd:Q1066476
  }
  ?org wdt:P31/wdt:P279* ?type ;
       wdt:P856 ?website ;
       wdt:P17 ?country .
  ?country wdt:P297 ?countryCode .
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . }
}
`

// FetchHumanitarianOrgs queries Wikidata for humanitarian, emergency management,
// and civil protection agencies worldwide.
func FetchHumanitarianOrgs(ctx context.Context, client *fetch.Client) ([]HumanitarianOrg, error) {
	query := strings.TrimSpace(humanitarianQuery)
	reqURL := wikidataSPARQL + "?format=json&query=" + url.QueryEscape(query)
	body, err := fetchTextWithRetry(ctx, client, reqURL, "application/sparql-results+json, application/json;q=0.9")
	if err != nil {
		return nil, fmt.Errorf("wikidata SPARQL humanitarian: %w", err)
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
		return nil, fmt.Errorf("wikidata SPARQL humanitarian parse: %w", err)
	}

	seen := map[string]struct{}{}
	var orgs []HumanitarianOrg
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
	return orgs, nil
}
