// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/fetch"
)

const firstAPIBase = "https://api.first.org/data/v1/teams"
const firstPageLimit = 100

// FIRSTTeam represents a CSIRT team from the FIRST.org API.
type FIRSTTeam struct {
	ShortName string `json:"short_name"`
	Country   string `json:"country"`
	Website   string `json:"website"`
}

type firstWebsiteField string

func (f *firstWebsiteField) UnmarshalJSON(data []byte) error {
	data = []byte(strings.TrimSpace(string(data)))
	if string(data) == "null" || len(data) == 0 {
		*f = ""
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*f = firstWebsiteField(strings.TrimSpace(single))
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err == nil {
		for _, entry := range many {
			entry = strings.TrimSpace(entry)
			if entry != "" {
				*f = firstWebsiteField(entry)
				return nil
			}
		}
		*f = ""
		return nil
	}
	return fmt.Errorf("unsupported FIRST website field: %s", string(data))
}

// FetchFIRSTTeams fetches all CSIRT teams from the FIRST.org API,
// paginating through all results. Returns teams that have a non-empty
// website URL. Results are cached via the Wikidata/discovery cache.
func FetchFIRSTTeams(ctx context.Context, cfg config.Config, client *fetch.Client) ([]FIRSTTeam, error) {
	var allTeams []FIRSTTeam
	offset := 0
	for {
		if ctx.Err() != nil {
			return allTeams, ctx.Err()
		}
		reqURL := fmt.Sprintf("%s?limit=%d&offset=%d", firstAPIBase, firstPageLimit, offset)
		body, err := fetchWikidataTextWithCache(ctx, cfg, client, reqURL, "application/json")
		if err != nil {
			return allTeams, fmt.Errorf("FIRST.org API page offset=%d: %w", offset, err)
		}

		var resp struct {
			Data []struct {
				ShortName string            `json:"short_name"`
				Country   string            `json:"country"`
				Website   firstWebsiteField `json:"website"`
				Host      string            `json:"host"`
			} `json:"data"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return allTeams, fmt.Errorf("FIRST.org API parse: %w", err)
		}

		for _, team := range resp.Data {
			website := normalizeFIRSTWebsite(firstNonEmpty(string(team.Website), team.Host))
			if website == "" {
				continue
			}
			parsed, err := url.Parse(website)
			if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
				continue
			}
			authorityName := strings.TrimSpace(team.ShortName)
			if authorityName == "" {
				authorityName = hostToName(strings.ToLower(parsed.Hostname()))
			}
			allTeams = append(allTeams, FIRSTTeam{
				ShortName: authorityName,
				Country:   team.Country,
				Website:   strings.TrimRight(website, "/"),
			})
		}

		offset += firstPageLimit
		if len(resp.Data) < firstPageLimit || offset >= resp.Total {
			break
		}
	}
	return allTeams, nil
}

func normalizeFIRSTWebsite(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(raw), "http://") && !strings.HasPrefix(strings.ToLower(raw), "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(parsed.Hostname()) == "" || strings.Contains(parsed.Hostname(), " ") {
		return ""
	}
	return parsed.String()
}
