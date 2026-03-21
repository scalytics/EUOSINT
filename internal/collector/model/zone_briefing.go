// Copyright 2025 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package model

type ZoneBriefingRecord struct {
	LensID            string                 `json:"lens_id"`
	Title             string                 `json:"title"`
	Source            string                 `json:"source"`
	SourceURL         string                 `json:"source_url,omitempty"`
	Status            string                 `json:"status,omitempty"`
	UpdatedAt         string                 `json:"updated_at,omitempty"`
	CoverageNote      string                 `json:"coverage_note,omitempty"`
	CountryIDs        []string               `json:"country_ids,omitempty"`
	CountryLabels     []string               `json:"country_labels,omitempty"`
	Actors            []string               `json:"actors,omitempty"`
	ViolenceTypes     []string               `json:"violence_types,omitempty"`
	Hotspots          []ZoneBriefingHotspot  `json:"hotspots,omitempty"`
	Metrics           ZoneBriefingMetrics    `json:"metrics"`
	Violence          ZoneBriefingViolence   `json:"violence"`
	ActorSummary      ZoneBriefingActors     `json:"actors_summary"`
	Geography         ZoneBriefingGeography  `json:"geography"`
	Quality           ZoneBriefingQuality    `json:"quality"`
	Summary           ZoneBriefingSummary    `json:"summary"`
	ConflictIntensity string                 `json:"conflict_intensity,omitempty"`
	ConflictType      string                 `json:"conflict_type,omitempty"`
	ConflictStartDate string                 `json:"conflict_start_date,omitempty"`
	ActiveConflicts   []ZoneBriefingConflict `json:"active_conflicts,omitempty"`
	ACLEDRecency      *ZoneBriefingACLED     `json:"acled_recency,omitempty"`
}

type ZoneBriefingHotspot struct {
	Label      string  `json:"label"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	EventCount int     `json:"event_count"`
}

type ZoneBriefingMetrics struct {
	Events7D          int    `json:"events_7d"`
	Events30D         int    `json:"events_30d"`
	FatalitiesBest7D  int    `json:"fatalities_best_7d"`
	FatalitiesBest30D int    `json:"fatalities_best_30d"`
	FatalitiesTotal   int    `json:"fatalities_total"`
	CivilianDeaths30D int    `json:"civilian_deaths_30d"`
	Trend7D           string `json:"trend_7d"`
	Trend30D          string `json:"trend_30d"`
}

type ZoneBriefingViolence struct {
	Primary           string  `json:"primary,omitempty"`
	Secondary         string  `json:"secondary,omitempty"`
	OneSidedShare     float64 `json:"one_sided_share,omitempty"`
	CivilianHarmShare float64 `json:"civilian_harm_share,omitempty"`
}

type ZoneBriefingActors struct {
	TopDyads  []string `json:"top_dyads,omitempty"`
	TopActors []string `json:"top_actors,omitempty"`
}

type ZoneBriefingGeography struct {
	Hotspots []ZoneBriefingHotspot `json:"hotspots,omitempty"`
	Admin1   []string              `json:"admin1,omitempty"`
	Admin2   []string              `json:"admin2,omitempty"`
}

type ZoneBriefingQuality struct {
	WherePrecisionAvg float64 `json:"where_precision_avg,omitempty"`
	DatePrecisionAvg  float64 `json:"date_precision_avg,omitempty"`
	EventClarityAvg   float64 `json:"event_clarity_avg,omitempty"`
}

type ZoneBriefingSummary struct {
	Headline   string   `json:"headline,omitempty"`
	Bullets    []string `json:"bullets,omitempty"`
	WatchItems []string `json:"watch_items,omitempty"`
}

type ZoneBriefingConflict struct {
	ConflictID string `json:"conflict_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Intensity  int    `json:"intensity"`
}

type ZoneBriefingACLED struct {
	Events7D     int    `json:"events_7d"`
	Fatalities7D int    `json:"fatalities_7d"`
	TopEvent     string `json:"top_event,omitempty"`
	AsOf         string `json:"as_of,omitempty"`
}
