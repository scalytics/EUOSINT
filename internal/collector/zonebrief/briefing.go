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

// LensDef describes a conflict monitoring lens.
type LensDef struct {
	ID                  string
	Title               string
	OverlayType         string
	CoverageNote        string
	ReferenceCountryID  string
	OverlayCountryCodes map[string]struct{}
	CountryCodes        map[string]struct{}
	Bounds              Bounds
}

type UCDPCountryRef struct {
	ID    string
	ISO2  string
	ISO3  string
	Label string
}

// Bounds is a geographic bounding box.
type Bounds struct {
	South float64
	West  float64
	North float64
	East  float64
}

// SupportedLenses lists the conflict monitoring lenses.
var SupportedLenses = []LensDef{
	{ID: "gaza", Title: "Gaza", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "666", OverlayCountryCodes: makeSet("PS"), CountryCodes: makeSet("PS", "IL", "EG", "LB", "JO"), Bounds: Bounds{29.5, 32.0, 34.8, 36.5}},
	{ID: "sudan", Title: "Sudan", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "625", OverlayCountryCodes: makeSet("SD"), CountryCodes: makeSet("SD", "SS", "TD", "CF", "ET", "ER"), Bounds: Bounds{3.0, 21.5, 23.5, 39.5}},
	{ID: "ukraine", Title: "Ukraine South", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "369", OverlayCountryCodes: makeSet("UA"), CountryCodes: makeSet("UA", "RU", "RO", "BG", "TR"), Bounds: Bounds{43.0, 27.0, 49.5, 39.5}},
	{ID: "red-sea", Title: "Red Sea", OverlayType: "maritime", CoverageNote: "Structured conflict context from UCDP GED; maritime live feeds remain primary for immediate route risk.", ReferenceCountryID: "679", OverlayCountryCodes: makeSet("YE"), CountryCodes: makeSet("YE", "SA", "EG", "SD", "ER", "DJ", "SO"), Bounds: Bounds{10.0, 31.0, 31.8, 45.5}},
	{ID: "sahel", Title: "Sahel", OverlayType: "terror", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "432", OverlayCountryCodes: makeSet("ML", "NE", "BF"), CountryCodes: makeSet("ML", "NE", "BF", "MR", "DZ", "TD"), Bounds: Bounds{10.0, -17.5, 24.5, 25.0}},
	{ID: "drc-east", Title: "DRC East", OverlayType: "conflict", CoverageNote: "Structured conflict context from UCDP GED; use live feeds for breaking updates.", ReferenceCountryID: "490", OverlayCountryCodes: makeSet("CD"), CountryCodes: makeSet("CD", "RW", "UG", "BI"), Bounds: Bounds{-8.5, 27.0, 4.5, 31.8}},
}

// UCDPCountryRefs maps ISO2 codes to UCDP country metadata.
var UCDPCountryRefs = map[string]UCDPCountryRef{
	"DZ": {ID: "615", ISO2: "DZ", ISO3: "DZA", Label: "Algeria"},
	"BF": {ID: "439", ISO2: "BF", ISO3: "BFA", Label: "Burkina Faso"},
	"BI": {ID: "516", ISO2: "BI", ISO3: "BDI", Label: "Burundi"},
	"BG": {ID: "355", ISO2: "BG", ISO3: "BGR", Label: "Bulgaria"},
	"CD": {ID: "490", ISO2: "CD", ISO3: "COD", Label: "Democratic Republic of the Congo"},
	"CF": {ID: "482", ISO2: "CF", ISO3: "CAF", Label: "Central African Republic"},
	"DJ": {ID: "522", ISO2: "DJ", ISO3: "DJI", Label: "Djibouti"},
	"EG": {ID: "651", ISO2: "EG", ISO3: "EGY", Label: "Egypt"},
	"ER": {ID: "531", ISO2: "ER", ISO3: "ERI", Label: "Eritrea"},
	"ET": {ID: "530", ISO2: "ET", ISO3: "ETH", Label: "Ethiopia"},
	"IL": {ID: "666", ISO2: "IL", ISO3: "ISR", Label: "Israel"},
	"JO": {ID: "663", ISO2: "JO", ISO3: "JOR", Label: "Jordan"},
	"LB": {ID: "660", ISO2: "LB", ISO3: "LBN", Label: "Lebanon"},
	"ML": {ID: "432", ISO2: "ML", ISO3: "MLI", Label: "Mali"},
	"MR": {ID: "435", ISO2: "MR", ISO3: "MRT", Label: "Mauritania"},
	"NE": {ID: "436", ISO2: "NE", ISO3: "NER", Label: "Niger"},
	"PS": {ISO2: "PS", ISO3: "PSE", Label: "Palestine"},
	"RO": {ID: "360", ISO2: "RO", ISO3: "ROU", Label: "Romania"},
	"RU": {ID: "365", ISO2: "RU", ISO3: "RUS", Label: "Russia"},
	"RW": {ID: "517", ISO2: "RW", ISO3: "RWA", Label: "Rwanda"},
	"SA": {ID: "670", ISO2: "SA", ISO3: "SAU", Label: "Saudi Arabia"},
	"SD": {ID: "625", ISO2: "SD", ISO3: "SDN", Label: "Sudan"},
	"SO": {ID: "520", ISO2: "SO", ISO3: "SOM", Label: "Somalia"},
	"SS": {ID: "626", ISO2: "SS", ISO3: "SSD", Label: "South Sudan"},
	"TD": {ID: "483", ISO2: "TD", ISO3: "TCD", Label: "Chad"},
	"TR": {ID: "640", ISO2: "TR", ISO3: "TUR", Label: "Turkey"},
	"UA": {ID: "369", ISO2: "UA", ISO3: "UKR", Label: "Ukraine"},
	"UG": {ID: "500", ISO2: "UG", ISO3: "UGA", Label: "Uganda"},
	"YE": {ID: "679", ISO2: "YE", ISO3: "YEM", Label: "Yemen"},
}

// Build creates zone briefing records from UCDP events, conflict metadata, and ACLED items.
func Build(items []parse.UCDPItem, conflicts []parse.UCDPConflict, acledItems []parse.ACLEDItem, now time.Time) []model.ZoneBriefingRecord {
	out := make([]model.ZoneBriefingRecord, 0, len(SupportedLenses))
	for _, lens := range SupportedLenses {
		brief := buildLensBrief(lens, items, now)
		enrichWithConflicts(&brief, lens, conflicts)
		enrichWithACLED(&brief, lens, acledItems, now)
		out = append(out, brief)
	}
	return out
}

func buildLensBrief(lens LensDef, items []parse.UCDPItem, now time.Time) model.ZoneBriefingRecord {
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
	countryCodeCounts := map[string]int{}
	hotspots := map[string]*hotspotAgg{}

	var latest time.Time
	var events7d, events30d int
	var fatalities7d, fatalities30d, fatalitiesTotal int
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
		fatalitiesTotal += item.Fatalities
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
		if raw := strings.TrimSpace(item.CountryCode); raw != "" {
			cc := raw
			if iso2, ok := gwnoToISO2[cc]; ok {
				cc = iso2
			}
			countryCodeCounts[cc]++
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
	topCountryCodes := topKeys(countryCodeCounts, 6)
	countryIDs := make([]string, 0, len(topCountryCodes))
	countryLabels := make([]string, 0, len(topCountryCodes))
	for _, code := range topCountryCodes {
		if ref, ok := UCDPCountryRefs[code]; ok {
			if ref.ID != "" {
				countryIDs = append(countryIDs, ref.ID)
			}
			countryLabels = append(countryLabels, ref.Label)
			continue
		}
		countryLabels = append(countryLabels, code)
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
	sourceURL := lensSourceURL(lens, topCountryCodes)

	return model.ZoneBriefingRecord{
		LensID:        lens.ID,
		Title:         lens.Title,
		Source:        "UCDP GED",
		SourceURL:     sourceURL,
		Status:        deriveStatus(events7d, events30d),
		UpdatedAt:     asOf,
		CoverageNote:  lens.CoverageNote,
		CountryIDs:    countryIDs,
		CountryLabels: countryLabels,
		Actors:        topActors,
		ViolenceTypes: topViolence,
		Hotspots:      hotspotList,
		Metrics: model.ZoneBriefingMetrics{
			Events7D:          events7d,
			Events30D:         events30d,
			FatalitiesBest7D:  fatalities7d,
			FatalitiesBest30D: fatalities30d,
			FatalitiesTotal:   fatalitiesTotal,
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

// enrichWithConflicts adds conflict-level metadata from ucdpprioconflict.
func enrichWithConflicts(brief *model.ZoneBriefingRecord, lens LensDef, conflicts []parse.UCDPConflict) {
	matched := matchConflictsToLens(lens, conflicts)
	if len(matched) == 0 {
		return
	}
	brief.ActiveConflicts = make([]model.ZoneBriefingConflict, 0, len(matched))
	maxIntensity := 0
	var primaryType string
	for _, c := range matched {
		brief.ActiveConflicts = append(brief.ActiveConflicts, model.ZoneBriefingConflict{
			ConflictID: c.ConflictID,
			Name:       c.ConflictName,
			Type:       parse.NormalizeConflictType(c.TypeOfConflict),
			Intensity:  c.IntensityLevel,
		})
		if c.IntensityLevel > maxIntensity {
			maxIntensity = c.IntensityLevel
			primaryType = parse.NormalizeConflictType(c.TypeOfConflict)
		}
	}
	switch maxIntensity {
	case 2:
		brief.ConflictIntensity = "war"
	case 1:
		brief.ConflictIntensity = "minor"
	}
	brief.ConflictType = primaryType
	brief.ConflictStartDate = earliestConflictStartDate(matched)
}

// matchConflictsToLens returns conflicts whose gwno matches the lens countries.
func matchConflictsToLens(lens LensDef, conflicts []parse.UCDPConflict) []parse.UCDPConflict {
	lensGWNOs := make(map[string]struct{})
	for cc := range lens.CountryCodes {
		if ref, ok := UCDPCountryRefs[cc]; ok && ref.ID != "" {
			lensGWNOs[ref.ID] = struct{}{}
		}
	}
	var out []parse.UCDPConflict
	for _, c := range conflicts {
		// GWNoLoc can be comma-separated for multi-country conflicts.
		for _, gwno := range strings.Split(c.GWNoLoc, ",") {
			gwno = strings.TrimSpace(gwno)
			if _, ok := lensGWNOs[gwno]; ok {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func earliestConflictStartDate(conflicts []parse.UCDPConflict) string {
	earliest := time.Time{}
	minYear := 0
	for _, c := range conflicts {
		if t := parseTime(c.StartDate); !t.IsZero() {
			if earliest.IsZero() || t.Before(earliest) {
				earliest = t
			}
		}
		if c.Year > 0 && (minYear == 0 || c.Year < minYear) {
			minYear = c.Year
		}
	}
	if !earliest.IsZero() {
		return earliest.UTC().Format(time.RFC3339)
	}
	if minYear > 0 {
		return fmt.Sprintf("%04d-01-01T00:00:00Z", minYear)
	}
	return ""
}

// enrichWithACLED adds ACLED recency data and adjusts status.
func enrichWithACLED(brief *model.ZoneBriefingRecord, lens LensDef, acledItems []parse.ACLEDItem, now time.Time) {
	if len(acledItems) == 0 {
		return
	}
	matched := matchACLEDToLens(lens, acledItems)
	if len(matched) == 0 {
		return
	}
	var events7d, fatalities7d int
	var topEvent string
	var topFatalities int
	var latest time.Time
	cutoff := now.Add(-7 * 24 * time.Hour)
	for _, item := range matched {
		eventTime := parseTime(item.Published)
		if eventTime.IsZero() || eventTime.Before(cutoff) {
			continue
		}
		events7d++
		fatalities7d += item.Fatalities
		if eventTime.After(latest) {
			latest = eventTime
		}
		if item.Fatalities > topFatalities {
			topFatalities = item.Fatalities
			topEvent = item.Title
		}
	}
	if events7d == 0 {
		return
	}
	asOf := ""
	if !latest.IsZero() {
		asOf = latest.UTC().Format(time.RFC3339)
	}
	brief.ACLEDRecency = &model.ZoneBriefingACLED{
		Events7D:     events7d,
		Fatalities7D: fatalities7d,
		TopEvent:     topEvent,
		AsOf:         asOf,
	}
	// If UCDP shows inactive/watch but ACLED has recent events, upgrade status.
	if brief.Status != "active" && events7d > 0 {
		brief.Status = "active"
	}
	// Add ACLED recency bullet.
	brief.Summary.Bullets = append(brief.Summary.Bullets,
		fmt.Sprintf("ACLED 7d: %d events, %d fatalities.", events7d, fatalities7d))
}

// matchACLEDToLens returns ACLED items matching by ISO2 or bounding box.
func matchACLEDToLens(lens LensDef, items []parse.ACLEDItem) []parse.ACLEDItem {
	var out []parse.ACLEDItem
	for _, item := range items {
		iso2 := parse.ACLEDISO2(item.ISO3)
		if iso2 != "" {
			if _, ok := lens.CountryCodes[strings.ToUpper(iso2)]; ok {
				out = append(out, item)
				continue
			}
		}
		if item.Lat != 0 || item.Lng != 0 {
			if item.Lat >= lens.Bounds.South && item.Lat <= lens.Bounds.North && item.Lng >= lens.Bounds.West && item.Lng <= lens.Bounds.East {
				out = append(out, item)
			}
		}
	}
	return out
}

func lensSourceURL(lens LensDef, topCountryCodes []string) string {
	for _, code := range topCountryCodes {
		if ref, ok := UCDPCountryRefs[code]; ok && strings.TrimSpace(ref.ID) != "" {
			return "https://ucdp.uu.se/country/" + ref.ID
		}
	}
	if strings.TrimSpace(lens.ReferenceCountryID) != "" {
		return "https://ucdp.uu.se/country/" + lens.ReferenceCountryID
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

// gwnoToISO2 maps UCDP numeric GW country codes to ISO2 letter codes.
var gwnoToISO2 = buildGWNOToISO2()

func buildGWNOToISO2() map[string]string {
	out := make(map[string]string, len(UCDPCountryRefs))
	for iso2, ref := range UCDPCountryRefs {
		if ref.ID != "" {
			out[ref.ID] = iso2
		}
	}
	return out
}

func matchesLens(lens LensDef, item parse.UCDPItem) bool {
	if item.Lat != 0 || item.Lng != 0 {
		if item.Lat >= lens.Bounds.South && item.Lat <= lens.Bounds.North && item.Lng >= lens.Bounds.West && item.Lng <= lens.Bounds.East {
			return true
		}
	}
	code := strings.ToUpper(strings.TrimSpace(item.CountryCode))
	if iso2, ok := gwnoToISO2[code]; ok {
		code = iso2
	}
	if _, ok := lens.CountryCodes[code]; ok {
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
