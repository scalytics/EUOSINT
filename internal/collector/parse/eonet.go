// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EONETResponse is the top-level response from NASA EONET v3.
type EONETResponse struct {
	Events []EONETEvent `json:"events"`
}

type EONETEvent struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Link       string          `json:"link"`
	Categories []EONETCategory `json:"categories"`
	Sources    []EONETSource   `json:"sources"`
	Geometry   []EONETGeometry `json:"geometry"`
}

type EONETCategory struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type EONETSource struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type EONETGeometry struct {
	Date        string    `json:"date"`
	Type        string    `json:"type"`        // "Point"
	Coordinates []float64 `json:"coordinates"` // [lng, lat]
}

// EONETItem extends FeedItem with EONET-specific metadata.
type EONETItem struct {
	FeedItem
	CategoryID    string
	CategoryTitle string
}

// ParseEONET parses a NASA EONET v3 JSON response.
func ParseEONET(body []byte) ([]EONETItem, error) {
	var doc EONETResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	items := make([]EONETItem, 0, len(doc.Events))
	for _, ev := range doc.Events {
		if ev.Title == "" {
			continue
		}

		var catID, catTitle string
		if len(ev.Categories) > 0 {
			catID = ev.Categories[0].ID
			catTitle = ev.Categories[0].Title
		}

		// Use the latest geometry point.
		var lat, lng float64
		var published string
		if len(ev.Geometry) > 0 {
			latest := ev.Geometry[len(ev.Geometry)-1]
			if len(latest.Coordinates) >= 2 {
				lng = latest.Coordinates[0]
				lat = latest.Coordinates[1]
			}
			published = latest.Date
		}

		link := ev.Link
		if link == "" && len(ev.Sources) > 0 {
			link = ev.Sources[0].URL
		}
		if link == "" {
			link = "https://eonet.gsfc.nasa.gov/api/v3/events/" + ev.ID
		}

		summary := fmt.Sprintf("%s — %s", catTitle, ev.Title)

		tags := []string{"natural-disaster"}
		if catTitle != "" {
			tags = append(tags, strings.ToLower(catTitle))
		}

		items = append(items, EONETItem{
			FeedItem: FeedItem{
				Title:     ev.Title,
				Link:      link,
				Published: published,
				Summary:   summary,
				Tags:      tags,
				Lat:       lat,
				Lng:       lng,
			},
			CategoryID:    catID,
			CategoryTitle: catTitle,
		})
	}
	return items, nil
}

// EONETSeverity returns severity based on EONET category.
func EONETSeverity(categoryID string) string {
	switch strings.ToLower(categoryID) {
	case "volcanoes":
		return "high"
	case "severeStorms", "severestorms":
		return "high"
	case "floods":
		return "high"
	case "earthquakes":
		return "high"
	case "wildfires":
		return "medium"
	case "landslides":
		return "medium"
	case "seaandlakeice", "snow":
		return "low"
	default:
		return "medium"
	}
}
