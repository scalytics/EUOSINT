// Copyright 2025 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package zonebrief

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
)

type lensDef struct {
	ID                  string
	Title               string
	OverlayType         string
	CoverageNote        string
	ReferenceCountryID  string
	MatchCountryCodes   map[string]struct{}
	OverlayCountryCodes map[string]struct{}
	Bounds              bounds
}

type bounds struct {
	south float64
	west  float64
	north float64
	east  float64
}

var supportedLenses = []lensDef{
	{ID: "gaza", Title: "Gaza", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "666", MatchCountryCodes: makeSet("PS", "IL", "EG", "LB", "JO"), Bounds: bounds{29.5, 32.0, 34.8, 36.5}},
	{ID: "sudan", Title: "Sudan", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "625", MatchCountryCodes: makeSet("SD", "SS", "TD", "CF", "ET", "ER"), OverlayCountryCodes: makeSet("SD"), Bounds: bounds{3.0, 21.5, 23.5, 39.5}},
	{ID: "ukraine", Title: "Ukraine South", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "369", MatchCountryCodes: makeSet("UA", "RU", "RO", "BG", "TR"), Bounds: bounds{43.0, 27.0, 49.5, 39.5}},
	{ID: "red-sea", Title: "Red Sea", OverlayType: "maritime", CoverageNote: "Structured conflict context from UCDP GED; maritime live feeds remain primary for immediate route risk.", ReferenceCountryID: "679", MatchCountryCodes: makeSet("YE", "SA", "EG", "SD", "ER", "DJ", "SO"), Bounds: bounds{10.0, 31.0, 31.8, 45.5}},
	{ID: "sahel", Title: "Sahel", OverlayType: "terror", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "432", MatchCountryCodes: makeSet("ML", "NE", "BF", "MR", "DZ", "TD"), Bounds: bounds{10.0, -17.5, 24.5, 25.0}},
	{ID: "drc-east", Title: "DRC East", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "490", MatchCountryCodes: makeSet("CD", "RW", "UG", "BI"), Bounds: bounds{-8.5, 27.0, 4.5, 31.8}},
}

func Build(items []parse.UCDPItem, now time.Time) []model.ZoneBriefingRecord {
	out := make([]model.ZoneBriefingRecord, 0, len(supportedLenses))
	for _, lens := range supportedLenses {
		brief := buildLensBrief(lens, items, now)
		out = append(out, brief)
	}
	return out
}

func buildLensBrief(lens lensDef, items []parse.UCDPItem, now time.Time) model.ZoneBriefingRecord {
	type hotspotAgg struct {
		label string
		lat   float64
		lng   float64
		count int
	}
	matched := make([]parse.UCDPItem, 0)
	catCounts := map[string]int{}
	actorCounts := map[string]int{}
	dyadCounts := map[string]int{}
	admin1Counts := map[string]int{}
	admin2Counts := map[string]int{}
	countryIDCounts := map[string]int{}
	countryLabelByID := map[string]string{}
	hotspots := map[string]*hotspotAgg{}

	var latest time.Time
	var events7d, events30d int
	var fatalities7d, fatalities30d int
	var civilians30d int
	var oneSided30d int
	var wherePrecSum, datePrecSum, claritySum float64
	var wherePrecN, datePrecN, clarityN float64

	for _, item := range items {
		if !matchesLens(lens, item) {
			continue
		}
		matched = append(matched, item)
		eventTime := parseTime(item.Published)
		if eventTime.After(latest) {
			latest = eventTime
		}
		if !eventTime.IsZero() {
			if eventTime.After(now.Add(-30 * 24 * time.Hour)) {
				events30d++
				fatalities30d += item.Fatalities
				civilians30d += item.CivilianDeaths
				if strings.EqualFold(item.ViolenceType, "One-sided violence") {
					oneSided30d++
				}
			}
			if eventTime.After(now.Add(-7 * 24 * time.Hour)) {
				events7d++
				fatalities7d += item.Fatalities
			}
		}
		if item.ViolenceType != "" {
			catCounts[item.ViolenceType]++
		}
		if item.SideA != "" {
			actorCounts[item.SideA]++
		}
		if item.SideB != "" {
			actorCounts[item.SideB]++
		}
		if item.DyadName != "" {
			dyadCounts[item.DyadName]++
		} else if item.SideA != "" && item.SideB != "" {
			dyadCounts[item.SideA+" vs "+item.SideB]++
		}
		if item.Admin1 != "" {
			admin1Counts[item.Admin1]++
		}
		if item.Admin2 != "" {
			admin2Counts[item.Admin2]++
		}
		if strings.TrimSpace(item.CountryID) != "" {
			countryIDCounts[strings.TrimSpace(item.CountryID)]++
			if ref, ok := parse.UCDPCountryRefByID(item.CountryID); ok {
				countryLabelByID[item.CountryID] = ref.Label
			} else if strings.TrimSpace(item.Country) != "" {
				countryLabelByID[item.CountryID] = strings.TrimSpace(item.Country)
			}
		} else if strings.TrimSpace(item.CountryCode) != "" {
			if ref, ok := parse.UCDPCountryRefByISO2(item.CountryCode); ok && strings.TrimSpace(ref.ID) != "" {
				countryIDCounts[ref.ID]++
				countryLabelByID[ref.ID] = ref.Label
			}
		}
		if item.WherePrecision > 0 {
			wherePrecSum += float64(item.WherePrecision)
			wherePrecN++
		}
		if item.DatePrecision > 0 {
			datePrecSum += float64(item.DatePrecision)
			datePrecN++
		}
		if item.EventClarity > 0 {
			claritySum += float64(item.EventClarity)
			clarityN++
		}
		if item.Lat != 0 || item.Lng != 0 {
			label := firstNonEmpty(item.Admin2, item.Admin1, item.Country, "Hotspot")
			key := fmt.Sprintf("%.2f:%.2f:%s", item.Lat, item.Lng, label)
			current := hotspots[key]
			if current == nil {
				current = &hotspotAgg{label: label, lat: item.Lat, lng: item.Lng}
				hotspots[key] = current
			}
			current.count++
		}
	}

	topViolence := topKeys(catCounts, 2)
	topActors := topKeys(actorCounts, 4)
	topDyads := topKeys(dyadCounts, 3)
	topAdmin1 := topKeys(admin1Counts, 3)
	topAdmin2 := topKeys(admin2Counts, 4)
	topCountryIDs := topKeys(countryIDCounts, 6)
	countryLabels := make([]string, 0, len(topCountryIDs))
	for _, id := range topCountryIDs {
		countryLabels = append(countryLabels, firstNonEmpty(countryLabelByID[id], id))
	}
	hotspotList := make([]model.ZoneBriefingHotspot, 0, len(hotspots))
	for _, hotspot := range hotspots {
		hotspotList = append(hotspotList, model.ZoneBriefingHotspot{
			Label:      hotspot.label,
			Lat:        hotspot.lat,
			Lng:        hotspot.lng,
			EventCount: hotspot.count,
		})
	}
	sort.Slice(hotspotList, func(i, j int) bool { return hotspotList[i].EventCount > hotspotList[j].EventCount })
	if len(hotspotList) > 4 {
		hotspotList = hotspotList[:4]
	}

	asOf := ""
	if !latest.IsZero() {
		asOf = latest.UTC().Format(time.RFC3339)
	}
	headline := lens.Title + " remains in structured conflict monitoring view."
	if len(topViolence) > 0 {
		headline = fmt.Sprintf("%s is currently dominated by %s in the UCDP event record.", lens.Title, topViolence[0])
	}
	bullets := []string{
		fmt.Sprintf("30d event count: %d, fatalities: %d.", events30d, fatalities30d),
	}
	if len(topAdmin2) > 0 {
		bullets = append(bullets, "Hotspots: "+strings.Join(topAdmin2, ", ")+".")
	}
	if len(topActors) > 0 {
		bullets = append(bullets, "Top actors: "+strings.Join(topActors[:min(3, len(topActors))], ", ")+".")
	}
	watchItems := []string{
		"Monitor for expansion into adjacent hotspots.",
		"Watch for higher civilian harm share in the next 7d window.",
	}

	var oneSidedShare float64
	if events30d > 0 {
		oneSidedShare = float64(oneSided30d) / float64(events30d)
	}
	var civilianShare float64
	if fatalities30d > 0 {
		civilianShare = float64(civilians30d) / float64(fatalities30d)
	}
	sourceURL := lensSourceURL(lens, topCountryIDs)

	return model.ZoneBriefingRecord{
		LensID:        lens.ID,
		Title:         lens.Title,
		Source:        "UCDP GED",
		SourceURL:     sourceURL,
		Status:        deriveStatus(events7d, events30d),
		UpdatedAt:     asOf,
		CoverageNote:  lens.CoverageNote,
		CountryIDs:    topCountryIDs,
		CountryLabels: countryLabels,
		Actors:        topActors,
		ViolenceTypes: topViolence,
		Hotspots:      hotspotList,
		Metrics: model.ZoneBriefingMetrics{
			Events7D:          events7d,
			Events30D:         events30d,
			FatalitiesBest7D:  fatalities7d,
			FatalitiesBest30D: fatalities30d,
			CivilianDeaths30D: civilians30d,
			Trend7D:           trendLabel(events7d, events30d-events7d),
			Trend30D:          trendLabel(events30d, 0),
		},
		Violence: model.ZoneBriefingViolence{
			Primary:           firstAt(topViolence, 0),
			Secondary:         firstAt(topViolence, 1),
			OneSidedShare:     round2(oneSidedShare),
			CivilianHarmShare: round2(civilianShare),
		},
		ActorSummary: model.ZoneBriefingActors{
			TopDyads:  topDyads,
			TopActors: topActors,
		},
		Geography: model.ZoneBriefingGeography{
			Hotspots: hotspotList,
			Admin1:   topAdmin1,
			Admin2:   topAdmin2,
		},
		Quality: model.ZoneBriefingQuality{
			WherePrecisionAvg: round2(avg(wherePrecSum, wherePrecN)),
			DatePrecisionAvg:  round2(avg(datePrecSum, datePrecN)),
			EventClarityAvg:   round2(avg(claritySum, clarityN)),
		},
		Summary: model.ZoneBriefingSummary{
			Headline:   headline,
			Bullets:    bullets,
			WatchItems: watchItems,
		},
	}
}

func lensSourceURL(lens lensDef, topCountryIDs []string) string {
	if strings.TrimSpace(lens.ReferenceCountryID) != "" {
		return "https://ucdp.uu.se/country/" + lens.ReferenceCountryID
	}
	for _, id := range topCountryIDs {
		if strings.TrimSpace(id) != "" {
			return "https://ucdp.uu.se/country/" + id
		}
	}
	return ""
}

func deriveStatus(events7d, events30d int) string {
	switch {
	case events7d > 0:
		return "active"
	case events30d > 0:
		return "watch"
	default:
		return "inactive"
	}
}

func matchesLens(lens lensDef, item parse.UCDPItem) bool {
	if item.Lat != 0 || item.Lng != 0 {
		if item.Lat >= lens.Bounds.south && item.Lat <= lens.Bounds.north && item.Lng >= lens.Bounds.west && item.Lng <= lens.Bounds.east {
			return true
		}
	}
	code := strings.ToUpper(strings.TrimSpace(item.CountryCode))
	if _, ok := lens.MatchCountryCodes[code]; ok {
		return true
	}
	return false
}

func makeSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[strings.ToUpper(strings.TrimSpace(value))] = struct{}{}
	}
	return out
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func topKeys(counts map[string]int, limit int) []string {
	type kv struct {
		key   string
		count int
	}
	items := make([]kv, 0, len(counts))
	for key, count := range counts {
		if strings.TrimSpace(key) == "" || count <= 0 {
			continue
		}
		items = append(items, kv{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.key)
	}
	return out
}

func trendLabel(current, baseline int) string {
	if current == 0 && baseline == 0 {
		return "flat"
	}
	if baseline <= 0 {
		return fmt.Sprintf("up %d", current)
	}
	delta := ((float64(current) - float64(baseline)) / float64(baseline)) * 100
	if delta > 0 {
		return fmt.Sprintf("up %.0f%%", delta)
	}
	if delta < 0 {
		return fmt.Sprintf("down %.0f%%", -delta)
	}
	return "flat"
}

func avg(sum, n float64) float64 {
	if n == 0 {
		return 0
	}
	return sum / n
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
