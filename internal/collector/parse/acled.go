// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ACLEDResponse is the top-level response from the ACLED API.
type ACLEDResponse struct {
	Status int          `json:"status"`
	Count  int          `json:"count"`
	Data   []ACLEDEvent `json:"data"`
}

// ACLEDEvent is a single conflict event from the ACLED API.
type ACLEDEvent struct {
	DataID       json.Number `json:"data_id"`
	EventDate    string      `json:"event_date"`
	Year         json.Number `json:"year"`
	EventType    string      `json:"event_type"`
	SubEventType string      `json:"sub_event_type"`
	Actor1       string      `json:"actor1"`
	Actor2       string      `json:"actor2"`
	Country      string      `json:"country"`
	ISO3         string      `json:"iso3"`
	Region       string      `json:"region"`
	Admin1       string      `json:"admin1"`
	Admin2       string      `json:"admin2"`
	Admin3       string      `json:"admin3"`
	Location     string      `json:"location"`
	Latitude     string      `json:"latitude"`
	Longitude    string      `json:"longitude"`
	Source       string      `json:"source"`
	SourceScale  string      `json:"source_scale"`
	Notes        string      `json:"notes"`
	Fatalities   json.Number `json:"fatalities"`
	Tags         string      `json:"tags"`
	Timestamp    json.Number `json:"timestamp"`
}

// ACLEDItem extends FeedItem with ACLED-specific metadata needed for
// category/severity mapping in the normalizer.
type ACLEDItem struct {
	FeedItem
	EventType  string
	Fatalities int
	Country    string // Full country name from ACLED
	ISO3       string // 3-letter ISO code
	Region     string // ACLED region (e.g. "Europe", "Middle East")
}

// ParseACLED parses an ACLED API JSON response into ACLEDItems.
func ParseACLED(body []byte) ([]ACLEDItem, int, error) {
	var resp ACLEDResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}
	if resp.Status != 200 && resp.Status != 0 {
		return nil, 0, fmt.Errorf("ACLED API status %d", resp.Status)
	}

	items := make([]ACLEDItem, 0, len(resp.Data))
	for _, ev := range resp.Data {
		title := buildACLEDTitle(ev)
		if title == "" {
			continue
		}

		summary := buildACLEDSummary(ev)
		tags := buildACLEDTags(ev)
		link := fmt.Sprintf("https://acleddata.com/data-export-tool/#event=%s", ev.DataID)

		lat, _ := strconv.ParseFloat(ev.Latitude, 64)
		lng, _ := strconv.ParseFloat(ev.Longitude, 64)
		fatalities, _ := strconv.Atoi(ev.Fatalities.String())

		items = append(items, ACLEDItem{
			FeedItem: FeedItem{
				Title:     title,
				Link:      link,
				Published: ev.EventDate,
				Summary:   summary,
				Tags:      tags,
				Lat:       lat,
				Lng:       lng,
			},
			EventType:  ev.EventType,
			Fatalities: fatalities,
			Country:    ev.Country,
			ISO3:       ev.ISO3,
			Region:     ev.Region,
		})
	}
	return items, resp.Count, nil
}

func buildACLEDTitle(ev ACLEDEvent) string {
	eventType := strings.TrimSpace(ev.SubEventType)
	if eventType == "" {
		eventType = strings.TrimSpace(ev.EventType)
	}
	if eventType == "" {
		return ""
	}

	location := strings.TrimSpace(ev.Location)
	admin1 := strings.TrimSpace(ev.Admin1)
	country := strings.TrimSpace(ev.Country)

	parts := []string{eventType}
	if location != "" {
		parts = append(parts, "in "+location)
	}
	if admin1 != "" && admin1 != location {
		parts = append(parts, "("+admin1+")")
	}
	if country != "" {
		parts = append(parts, "—", country)
	}
	return strings.Join(parts, " ")
}

func buildACLEDSummary(ev ACLEDEvent) string {
	parts := []string{}

	if ev.Actor1 != "" {
		parts = append(parts, "Actor: "+ev.Actor1)
	}
	if ev.Actor2 != "" {
		parts = append(parts, "vs "+ev.Actor2)
	}
	if ev.Notes != "" {
		notes := ev.Notes
		if len(notes) > 500 {
			notes = notes[:497] + "..."
		}
		parts = append(parts, notes)
	}
	fatalities, _ := strconv.Atoi(ev.Fatalities.String())
	if fatalities > 0 {
		parts = append(parts, fmt.Sprintf("Fatalities: %d", fatalities))
	}
	if ev.Source != "" {
		parts = append(parts, "Source: "+ev.Source)
	}
	return strings.Join(parts, ". ")
}

func buildACLEDTags(ev ACLEDEvent) []string {
	tags := []string{}
	if ev.EventType != "" {
		tags = append(tags, strings.ToLower(ev.EventType))
	}
	if ev.SubEventType != "" {
		tags = append(tags, strings.ToLower(ev.SubEventType))
	}
	fatalities, _ := strconv.Atoi(ev.Fatalities.String())
	if fatalities > 0 {
		tags = append(tags, "fatalities")
	}
	if fatalities >= 10 {
		tags = append(tags, "mass-casualty")
	}
	// Parse ACLED tags field (semicolon-separated).
	if ev.Tags != "" {
		for _, t := range strings.Split(ev.Tags, ";") {
			t = strings.TrimSpace(strings.ToLower(t))
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

// ISO3toISO2 converts a 3-letter ISO country code to 2-letter.
// Covers the most common conflict-affected countries in ACLED data.
var iso3toISO2 = map[string]string{
	"AFG": "AF", "ALB": "AL", "DZA": "DZ", "AGO": "AO", "ARG": "AR",
	"ARM": "AM", "AUS": "AU", "AUT": "AT", "AZE": "AZ", "BHR": "BH",
	"BGD": "BD", "BLR": "BY", "BEL": "BE", "BEN": "BJ", "BOL": "BO",
	"BIH": "BA", "BWA": "BW", "BRA": "BR", "BGR": "BG", "BFA": "BF",
	"BDI": "BI", "KHM": "KH", "CMR": "CM", "CAN": "CA", "CAF": "CF",
	"TCD": "TD", "CHL": "CL", "CHN": "CN", "COL": "CO", "COD": "CD",
	"COG": "CG", "CRI": "CR", "HRV": "HR", "CUB": "CU", "CYP": "CY",
	"CZE": "CZ", "DNK": "DK", "DJI": "DJ", "ECU": "EC", "EGY": "EG",
	"SLV": "SV", "ERI": "ER", "EST": "EE", "ETH": "ET", "FIN": "FI",
	"FRA": "FR", "GAB": "GA", "GMB": "GM", "GEO": "GE", "DEU": "DE",
	"GHA": "GH", "GRC": "GR", "GTM": "GT", "GIN": "GN", "GNB": "GW",
	"HTI": "HT", "HND": "HN", "HUN": "HU", "IND": "IN", "IDN": "ID",
	"IRN": "IR", "IRQ": "IQ", "IRL": "IE", "ISR": "IL", "ITA": "IT",
	"CIV": "CI", "JAM": "JM", "JPN": "JP", "JOR": "JO", "KAZ": "KZ",
	"KEN": "KE", "XKX": "XK", "KWT": "KW", "KGZ": "KG", "LAO": "LA",
	"LVA": "LV", "LBN": "LB", "LBR": "LR", "LBY": "LY", "LTU": "LT",
	"MDG": "MG", "MWI": "MW", "MYS": "MY", "MLI": "ML", "MLT": "MT",
	"MRT": "MR", "MEX": "MX", "MDA": "MD", "MNG": "MN", "MNE": "ME",
	"MAR": "MA", "MOZ": "MZ", "MMR": "MM", "NAM": "NA", "NPL": "NP",
	"NLD": "NL", "NZL": "NZ", "NIC": "NI", "NER": "NE", "NGA": "NG",
	"PRK": "KP", "MKD": "MK", "NOR": "NO", "OMN": "OM", "PAK": "PK",
	"PSE": "PS", "PAN": "PA", "PNG": "PG", "PRY": "PY", "PER": "PE",
	"PHL": "PH", "POL": "PL", "PRT": "PT", "QAT": "QA", "ROU": "RO",
	"RUS": "RU", "RWA": "RW", "SAU": "SA", "SEN": "SN", "SRB": "RS",
	"SLE": "SL", "SGP": "SG", "SVK": "SK", "SVN": "SI", "SOM": "SO",
	"ZAF": "ZA", "KOR": "KR", "SSD": "SS", "ESP": "ES", "LKA": "LK",
	"SDN": "SD", "SWE": "SE", "CHE": "CH", "SYR": "SY", "TWN": "TW",
	"TJK": "TJ", "TZA": "TZ", "THA": "TH", "TGO": "TG", "TUN": "TN",
	"TUR": "TR", "TKM": "TM", "UGA": "UG", "UKR": "UA", "ARE": "AE",
	"GBR": "GB", "USA": "US", "URY": "UY", "UZB": "UZ", "VEN": "VE",
	"VNM": "VN", "YEM": "YE", "ZMB": "ZM", "ZWE": "ZW",
}

// ACLEDISO2 converts an ACLED ISO3 code to ISO2. Returns "" if unknown.
func ACLEDISO2(iso3 string) string {
	return iso3toISO2[strings.ToUpper(strings.TrimSpace(iso3))]
}

// ACLEDEventCategory maps ACLED event types to kafSIEM categories.
func ACLEDEventCategory(eventType string) string {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "battles":
		return "conflict_monitoring"
	case "explosions/remote violence":
		return "conflict_monitoring"
	case "violence against civilians":
		return "conflict_monitoring"
	case "protests":
		return "public_safety"
	case "riots":
		return "public_safety"
	case "strategic developments":
		return "intelligence_report"
	default:
		return "conflict_monitoring"
	}
}

// ACLEDEventSeverity infers severity from ACLED event characteristics.
func ACLEDEventSeverity(eventType string, fatalities int) string {
	if fatalities >= 10 {
		return "critical"
	}
	if fatalities > 0 {
		return "high"
	}
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "battles", "explosions/remote violence":
		return "high"
	case "violence against civilians":
		return "high"
	case "riots":
		return "medium"
	case "protests":
		return "medium"
	case "strategic developments":
		return "medium"
	default:
		return "medium"
	}
}
