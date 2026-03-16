package model

type Alert struct {
	AlertID        string            `json:"alert_id"`
	SourceID       string            `json:"source_id"`
	Source         SourceMetadata    `json:"source"`
	Title          string            `json:"title"`
	CanonicalURL   string            `json:"canonical_url"`
	FirstSeen      string            `json:"first_seen"`
	LastSeen       string            `json:"last_seen"`
	Status         string            `json:"status"`
	Category       string            `json:"category"`
	Severity       string            `json:"severity"`
	RegionTag      string            `json:"region_tag"`
	Lat            float64           `json:"lat"`
	Lng            float64           `json:"lng"`
	FreshnessHours int               `json:"freshness_hours"`
	Reporting      ReportingMetadata `json:"reporting,omitempty"`
	Triage         *Triage           `json:"triage,omitempty"`
}

type Triage struct {
	RelevanceScore  float64         `json:"relevance_score"`
	Threshold       float64         `json:"threshold,omitempty"`
	Confidence      string          `json:"confidence,omitempty"`
	Disposition     string          `json:"disposition,omitempty"`
	PublicationType string          `json:"publication_type,omitempty"`
	WeakSignals     []string        `json:"weak_signals,omitempty"`
	Metadata        *TriageMetadata `json:"metadata,omitempty"`
	Reasoning       string          `json:"reasoning,omitempty"`
}

type TriageMetadata struct {
	Author string   `json:"author,omitempty"`
	Tags   []string `json:"tags,omitempty"`
}

type DuplicateSample struct {
	Title string `json:"title"`
	Count int    `json:"count"`
}

type DuplicateAudit struct {
	SuppressedVariantDuplicates int               `json:"suppressed_variant_duplicates"`
	RepeatedTitleGroupsInActive int               `json:"repeated_title_groups_in_active"`
	RepeatedTitleSamples        []DuplicateSample `json:"repeated_title_samples"`
}

type SourceHealthEntry struct {
	SourceID      string `json:"source_id"`
	AuthorityName string `json:"authority_name"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	FetchedCount  int    `json:"fetched_count"`
	FeedURL       string `json:"feed_url"`
	Error         string `json:"error,omitempty"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	ActiveCount   int    `json:"active_count,omitempty"`
	FilteredCount int    `json:"filtered_count,omitempty"`
}

type SourceHealthDocument struct {
	GeneratedAt             string              `json:"generated_at"`
	CriticalSourcePrefixes  []string            `json:"critical_source_prefixes"`
	FailOnCriticalSourceGap bool                `json:"fail_on_critical_source_gap"`
	TotalSources            int                 `json:"total_sources"`
	SourcesOK               int                 `json:"sources_ok"`
	SourcesError            int                 `json:"sources_error"`
	DuplicateAudit          DuplicateAudit      `json:"duplicate_audit"`
	Sources                 []SourceHealthEntry `json:"sources"`
}
