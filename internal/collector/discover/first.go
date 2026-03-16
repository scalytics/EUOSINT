// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
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
				ShortName string `json:"short_name"`
				Country   string `json:"country"`
				Website   string `json:"website"`
				Host      string `json:"host"`
			} `json:"data"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return allTeams, fmt.Errorf("FIRST.org API parse: %w", err)
		}

		for _, team := range resp.Data {
			website := strings.TrimSpace(team.Website)
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
