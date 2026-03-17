// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package model

// Supported source types: rss, html-list, kev-json, interpol-red-json,
// interpol-yellow-json, fbi-wanted-json, travelwarning-json, travelwarning-atom.
type RegistrySource struct {
	Type            string            `json:"type"`
	FetchMode       string            `json:"fetch_mode,omitempty"` // "stealth" (default) or "browser"
	FollowRedirects bool              `json:"followRedirects"`
	FeedURL         string            `json:"feed_url"`
	FeedURLs        []string          `json:"feed_urls,omitempty"`
	Category        string            `json:"category"`
	RegionTag       string            `json:"region_tag"`
	Lat             float64           `json:"lat"`
	Lng             float64           `json:"lng"`
	MaxItems        int               `json:"max_items"`
	Accumulate      bool              `json:"accumulate,omitempty"`
	IncludeKeywords []string          `json:"include_keywords,omitempty"`
	ExcludeKeywords []string          `json:"exclude_keywords,omitempty"`
	SourceQuality   float64           `json:"source_quality,omitempty"`
	PromotionStatus string            `json:"promotion_status,omitempty"`
	RejectionReason string            `json:"rejection_reason,omitempty"`
	IsMirror        bool              `json:"is_mirror,omitempty"`
	PreferredRank   int               `json:"preferred_source_rank,omitempty"`
	Reporting       ReportingMetadata `json:"reporting"`
	Source          SourceMetadata    `json:"source"`
}

type ReportingMetadata struct {
	Label string `json:"label,omitempty"`
	URL   string `json:"url,omitempty"`
	Phone string `json:"phone,omitempty"`
	Notes string `json:"notes,omitempty"`
}

type SourceMetadata struct {
	SourceID             string   `json:"source_id"`
	AuthorityName        string   `json:"authority_name"`
	Country              string   `json:"country"`
	CountryCode          string   `json:"country_code"`
	Region               string   `json:"region"`
	AuthorityType        string   `json:"authority_type"`
	BaseURL              string   `json:"base_url"`
	Scope                string   `json:"scope,omitempty"`
	Level                string   `json:"level,omitempty"`
	ParentAgencyID       string   `json:"parent_agency_id,omitempty"`
	JurisdictionName     string   `json:"jurisdiction_name,omitempty"`
	MissionTags          []string `json:"mission_tags,omitempty"`
	OperationalRelevance float64  `json:"operational_relevance,omitempty"`
	IsCurated            bool     `json:"is_curated,omitempty"`
	IsHighValue          bool     `json:"is_high_value,omitempty"`
	LanguageCode         string   `json:"language_code,omitempty"`
}
