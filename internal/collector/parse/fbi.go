// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"strings"
)

// FBIWantedResponse is the top-level response from the FBI Wanted API.
type FBIWantedResponse struct {
	Total int              `json:"total"`
	Page  int              `json:"page"`
	Items []FBIWantedEntry `json:"items"`
}

// FBIWantedEntry is a single person/case from the FBI Wanted API.
type FBIWantedEntry struct {
	UID                  string   `json:"uid"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	Details              string   `json:"details"`
	Caution              string   `json:"caution"`
	WarningMessage       string   `json:"warning_message"`
	Remarks              string   `json:"remarks"`
	Sex                  string   `json:"sex"`
	Nationality          string   `json:"nationality"`
	PlaceOfBirth         string   `json:"place_of_birth"`
	DatesOfBirthUsed     []string `json:"dates_of_birth_used"`
	Aliases              []string `json:"aliases"`
	Subjects             []string `json:"subjects"`
	Status               string   `json:"status"`
	PersonClassification string   `json:"person_classification"`
	PosterClassification string   `json:"poster_classification"`
	RewardText           string   `json:"reward_text"`
	RewardMin            int      `json:"reward_min"`
	RewardMax            int      `json:"reward_max"`
	URL                  string   `json:"url"`
	Path                 string   `json:"path"`
	Publication          string   `json:"publication"`
	Modified             string   `json:"modified"`
	FieldOffices         []string `json:"field_offices"`
	PossibleCountries    []string `json:"possible_countries"`
	PossibleStates       []string `json:"possible_states"`
	Images               []struct {
		Thumb    string `json:"thumb"`
		Original string `json:"original"`
		Large    string `json:"large"`
		Caption  string `json:"caption"`
	} `json:"images"`
}

// ParseFBIWanted parses the FBI Wanted API JSON response into FeedItems.
func ParseFBIWanted(body []byte) ([]FeedItem, int, error) {
	var resp FBIWantedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}
	items := make([]FeedItem, 0, len(resp.Items))
	for _, entry := range resp.Items {
		title := strings.TrimSpace(entry.Title)
		if title == "" {
			continue
		}
		link := strings.TrimSpace(entry.URL)
		if link == "" && strings.TrimSpace(entry.Path) != "" {
			link = "https://www.fbi.gov" + entry.Path
		}
		if link == "" {
			link = "https://www.fbi.gov/wanted"
		}

		summary := buildFBISummary(entry)
		tags := buildFBITags(entry)
		published := firstNonEmpty(entry.Modified, entry.Publication)

		items = append(items, FeedItem{
			Title:     title,
			Link:      link,
			Published: published,
			Summary:   summary,
			Tags:      tags,
		})
	}
	return items, resp.Total, nil
}

func buildFBISummary(entry FBIWantedEntry) string {
	parts := []string{}
	if desc := StripHTML(entry.Description); desc != "" {
		parts = append(parts, desc)
	}
	if entry.Nationality != "" {
		parts = append(parts, "Nationality: "+entry.Nationality)
	}
	if entry.PlaceOfBirth != "" {
		parts = append(parts, "Born: "+entry.PlaceOfBirth)
	}
	if len(entry.Aliases) > 0 {
		parts = append(parts, "Aliases: "+strings.Join(entry.Aliases, ", "))
	}
	if entry.RewardText != "" {
		parts = append(parts, "Reward: "+StripHTML(entry.RewardText))
	}
	return strings.Join(parts, ". ")
}

func buildFBITags(entry FBIWantedEntry) []string {
	tags := make([]string, 0, len(entry.Subjects)+4)
	tags = append(tags, entry.Subjects...)
	if entry.PosterClassification != "" {
		tags = append(tags, entry.PosterClassification)
	}
	if entry.PersonClassification != "" {
		tags = append(tags, entry.PersonClassification)
	}
	if entry.Sex != "" {
		tags = append(tags, entry.Sex)
	}
	if entry.WarningMessage != "" {
		tags = append(tags, "armed-dangerous")
	}
	return tags
}
