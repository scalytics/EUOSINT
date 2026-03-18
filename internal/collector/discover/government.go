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

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
)

// GovernmentOrg represents a government body, parliament, or ministry
// discovered via Wikidata whose website we should probe for feeds.
type GovernmentOrg struct {
	Name          string
	Country       string
	CountryCode   string
	Website       string
	AuthorityType string // "government", "legislative", "diplomatic"
	Category      string // suggested registry category
}

// governmentTypeIDs are Wikidata entity types for government bodies,
// parliaments, foreign affairs ministries, and embassies.
var governmentTypeIDs = []struct {
	id            string
	authorityType string
	category      string
}{
	// ── Legislative / Parliament ─────────────────────────────────
	{"Q11204", "legislative", "legislative"},         // parliament
	{"Q35749", "legislative", "legislative"},          // legislature
	{"Q637846", "legislative", "legislative"},         // unicameral legislature
	{"Q187997", "legislative", "legislative"},         // bicameral legislature
	{"Q1752346", "government", "legislative"},         // head of government office
	// ── Executive / Government ──────────────────────────────────
	{"Q35798", "government", "legislative"},           // executive body of government
	{"Q2659904", "government", "legislative"},         // government agency
	// ── Foreign Affairs / Diplomatic ────────────────────────────
	{"Q192350", "government", "travel_warning"},       // ministry of foreign affairs
	{"Q3917681", "diplomatic", "travel_warning"},      // embassy (filtered to major countries)
	// ── Defence / Military ──────────────────────────────────────
	{"Q691583", "national_security", "conflict_monitoring"}, // ministry of defence
	{"Q176799", "national_security", "conflict_monitoring"}, // military organization
}

func buildGovernmentQuery(typeID string) string {
	return fmt.Sprintf(`
SELECT ?website ?countryCode WHERE {
  ?org wdt:P31 wd:%s ;
       wdt:P856 ?website ;
       wdt:P17 ?country .
  ?country wdt:P297 ?countryCode .
} LIMIT 50
`, strings.TrimSpace(typeID))
}

// FetchGovernmentOrgs queries Wikidata for government bodies, parliaments,
// foreign affairs ministries, and defence ministries worldwide.
func FetchGovernmentOrgs(ctx context.Context, cfg config.Config, client *fetch.Client) ([]GovernmentOrg, error) {
	seen := map[string]struct{}{}
	var orgs []GovernmentOrg
	var failures []string

	for _, entry := range governmentTypeIDs {
		if ctx.Err() != nil {
			break
		}
		query := strings.TrimSpace(buildGovernmentQuery(entry.id))
		reqURL := wikidataSPARQL + "?format=json&query=" + url.QueryEscape(query)
		body, err := fetchWikidataTextWithCache(ctx, cfg, client, reqURL, "application/sparql-results+json, application/json;q=0.9")
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.id, err))
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
			failures = append(failures, fmt.Sprintf("%s: parse: %v", entry.id, err))
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

			orgs = append(orgs, GovernmentOrg{
				Name:          hostToName(host),
				Country:       countryFromCode(b.CountryCode.Value),
				CountryCode:   strings.ToUpper(strings.TrimSpace(b.CountryCode.Value)),
				Website:       website,
				AuthorityType: entry.authorityType,
				Category:      entry.category,
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
		return nil, fmt.Errorf("wikidata SPARQL government: %s", strings.Join(failures, " | "))
	}
	return nil, nil
}
