// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scalytics/euosint/internal/collector/fetch"
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
// website URL.
func FetchFIRSTTeams(ctx context.Context, client *fetch.Client) ([]FIRSTTeam, error) {
	var allTeams []FIRSTTeam
	offset := 0
	for {
		if ctx.Err() != nil {
			return allTeams, ctx.Err()
		}
		url := fmt.Sprintf("%s?limit=%d&offset=%d", firstAPIBase, firstPageLimit, offset)
		body, err := fetchTextWithRetry(ctx, client, url, "application/json")
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
			website := strings.TrimSpace(string(team.Website))
			if website == "" {
				website = strings.TrimSpace(team.Host)
			}
			if website == "" {
				continue
			}
			if !strings.HasPrefix(website, "http") {
				website = "https://" + website
			}
			allTeams = append(allTeams, FIRSTTeam{
				ShortName: team.ShortName,
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
