package model

type RegistrySource struct {
	Type            string            `json:"type"`
	FollowRedirects bool              `json:"followRedirects"`
	FeedURL         string            `json:"feed_url"`
	FeedURLs        []string          `json:"feed_urls,omitempty"`
	Category        string            `json:"category"`
	RegionTag       string            `json:"region_tag"`
	Lat             float64           `json:"lat"`
	Lng             float64           `json:"lng"`
	MaxItems        int               `json:"max_items"`
	IncludeKeywords []string          `json:"include_keywords,omitempty"`
	ExcludeKeywords []string          `json:"exclude_keywords,omitempty"`
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
	SourceID      string `json:"source_id"`
	AuthorityName string `json:"authority_name"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code"`
	Region        string `json:"region"`
	AuthorityType string `json:"authority_type"`
	BaseURL       string `json:"base_url"`
}
