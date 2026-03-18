// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FeodoEntry is a single entry from the Feodo Tracker IP blocklist.
type FeodoEntry struct {
	IPAddress  string `json:"ip_address"`
	Port       int    `json:"port"`
	Status     string `json:"status"` // "online" or "offline"
	Hostname   string `json:"hostname"`
	ASNumber   int    `json:"as_number"`
	ASName     string `json:"as_name"`
	Country    string `json:"country"` // 2-letter country code
	FirstSeen  string `json:"first_seen"`
	LastOnline string `json:"last_online"`
	Malware    string `json:"malware"` // e.g. "Dridex", "TrickBot"
}

// FeodoItem extends FeedItem with Feodo-specific metadata.
type FeodoItem struct {
	FeedItem
	IPAddress string
	Port      int
	Status    string
	Malware   string
	Country   string // 2-letter code
	ASName    string
}

// ParseFeodo parses the Feodo Tracker JSON blocklist, returning only
// online entries (active C2 servers).
func ParseFeodo(body []byte) ([]FeodoItem, error) {
	var entries []FeodoEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	items := make([]FeodoItem, 0, len(entries)/2)
	for _, e := range entries {
		if strings.ToLower(e.Status) != "online" {
			continue
		}
		if e.IPAddress == "" {
			continue
		}
		title := fmt.Sprintf("%s C2 Server: %s:%d", e.Malware, e.IPAddress, e.Port)
		summary := fmt.Sprintf("Active %s botnet C2 at %s:%d (AS%d %s, %s)",
			e.Malware, e.IPAddress, e.Port, e.ASNumber, e.ASName, e.Country)

		tags := []string{"botnet", "c2"}
		if e.Malware != "" {
			tags = append(tags, strings.ToLower(e.Malware))
		}

		published := e.LastOnline
		if published == "" {
			published = e.FirstSeen
		}

		items = append(items, FeodoItem{
			FeedItem: FeedItem{
				Title:     title,
				Link:      "https://feodotracker.abuse.ch/browse/host/" + e.IPAddress + "/",
				Published: published,
				Summary:   summary,
				Tags:      tags,
			},
			IPAddress: e.IPAddress,
			Port:      e.Port,
			Status:    e.Status,
			Malware:   e.Malware,
			Country:   e.Country,
			ASName:    e.ASName,
		})
	}
	return items, nil
}
