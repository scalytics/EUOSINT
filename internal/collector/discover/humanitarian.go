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
	"Q895526",   // humanitarian aid org
	"Q15220109", // disaster management authority
	"Q1460420",  // civil protection
	"Q1066476",  // emergency management
	// Subclasses we'd miss without P279* traversal:
	"Q3918693",  // emergency service
	"Q863734",   // rescue service
	"Q167546",   // NGO (filtered later by hygiene)
	"Q484652",   // international organization
}

func buildHumanitarianQuery(typeID string) string {
	// Query ONE type ID at a time with P31 (no P279* subclass traversal).
	// No label service, no rdfs:label OPTIONAL — both cause Wikidata
	// timeouts. We extract the hostname as a proxy for the org name.
	return fmt.Sprintf(`
SELECT ?website ?countryCode WHERE {
  ?org wdt:P31 wd:%s ;
       wdt:P856 ?website ;
       wdt:P17 ?country .
  ?country wdt:P297 ?countryCode .
} LIMIT 50
`, strings.TrimSpace(typeID))
}

// FetchHumanitarianOrgs queries Wikidata for humanitarian, emergency management,
// and civil protection agencies worldwide. Queries one type ID at a time
// with LIMIT 50 to stay within Wikidata's public SPARQL timeout limits.
func FetchHumanitarianOrgs(ctx context.Context, cfg config.Config, client *fetch.Client) ([]HumanitarianOrg, error) {
	var failures []string
	seen := map[string]struct{}{}
	var orgs []HumanitarianOrg

	for _, typeID := range humanitarianTypeIDs {
		if ctx.Err() != nil {
			break
		}
		query := strings.TrimSpace(buildHumanitarianQuery(typeID))
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

			orgs = append(orgs, HumanitarianOrg{
				Name:        hostToName(host),
				Country:     countryFromCode(b.CountryCode.Value),
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
