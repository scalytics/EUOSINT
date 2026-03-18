// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"fmt"
	"time"
)

// USGSGeoJSON is the top-level GeoJSON FeatureCollection from the USGS
// earthquake feed.
type USGSGeoJSON struct {
	Features []USGSFeature `json:"features"`
}

type USGSFeature struct {
	Properties USGSProperties `json:"properties"`
	Geometry   USGSGeometry   `json:"geometry"`
	ID         string         `json:"id"`
}

type USGSProperties struct {
	Mag     float64 `json:"mag"`
	Place   string  `json:"place"`
	Time    int64   `json:"time"`    // unix ms
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Alert   string  `json:"alert"`   // green/yellow/orange/red
	Tsunami int     `json:"tsunami"` // 0 or 1
	Type    string  `json:"type"`    // "earthquake"
}

type USGSGeometry struct {
	Coordinates []float64 `json:"coordinates"` // [lng, lat, depth]
}

// USGSItem extends FeedItem with earthquake-specific metadata.
type USGSItem struct {
	FeedItem
	Magnitude float64
	Tsunami   bool
	AlertLevel string // green/yellow/orange/red
}

// ParseUSGSGeoJSON parses a USGS GeoJSON earthquake feed.
func ParseUSGSGeoJSON(body []byte) ([]USGSItem, error) {
	var doc USGSGeoJSON
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	items := make([]USGSItem, 0, len(doc.Features))
	for _, f := range doc.Features {
		if f.Properties.Type != "" && f.Properties.Type != "earthquake" {
			continue
		}
		title := f.Properties.Title
		if title == "" {
			title = f.Properties.Place
		}
		if title == "" {
			continue
		}

		published := ""
		if f.Properties.Time > 0 {
			published = time.UnixMilli(f.Properties.Time).UTC().Format(time.RFC3339)
		}

		var lat, lng float64
		if len(f.Geometry.Coordinates) >= 2 {
			lng = f.Geometry.Coordinates[0]
			lat = f.Geometry.Coordinates[1]
		}

		summary := fmt.Sprintf("Magnitude %.1f earthquake — %s", f.Properties.Mag, f.Properties.Place)
		if f.Properties.Tsunami == 1 {
			summary += " [TSUNAMI WARNING]"
		}

		tags := []string{"earthquake"}
		if f.Properties.Tsunami == 1 {
			tags = append(tags, "tsunami")
		}
		if f.Properties.Alert != "" {
			tags = append(tags, "alert-"+f.Properties.Alert)
		}

		link := f.Properties.URL
		if link == "" {
			link = "https://earthquake.usgs.gov/earthquakes/eventpage/" + f.ID
		}

		items = append(items, USGSItem{
			FeedItem: FeedItem{
				Title:     title,
				Link:      link,
				Published: published,
				Summary:   summary,
				Tags:      tags,
				Lat:       lat,
				Lng:       lng,
			},
			Magnitude:  f.Properties.Mag,
			Tsunami:    f.Properties.Tsunami == 1,
			AlertLevel: f.Properties.Alert,
		})
	}
	return items, nil
}

// USGSSeverity returns severity based on magnitude.
func USGSSeverity(mag float64, alertLevel string) string {
	if alertLevel == "red" || mag >= 7.0 {
		return "critical"
	}
	if alertLevel == "orange" || mag >= 6.0 {
		return "critical"
	}
	if mag >= 5.0 {
		return "high"
	}
	return "medium"
}
