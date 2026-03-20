package zonebrief

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
)

type featureCollection struct {
	Type     string       `json:"type"`
	Features []geoFeature `json:"features"`
}

type geoFeature struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Geometry   any            `json:"geometry"`
}

type boundaryCollection struct {
	Type     string            `json:"type"`
	Features []boundaryFeature `json:"features"`
}

type boundaryFeature struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Geometry   any            `json:"geometry"`
}

func BuildConflictZonesGeoJSON(briefings []model.ZoneBriefingRecord) any {
	return buildRectangleZones(briefings, false)
}

func BuildTerrorZonesGeoJSON(briefings []model.ZoneBriefingRecord) any {
	return buildRectangleZones(briefings, true)
}

func BuildConflictZonesGeoJSONFromBoundaries(briefings []model.ZoneBriefingRecord, boundariesPath string) (any, error) {
	return buildZonesFromBoundaries(briefings, boundariesPath, false)
}

func BuildTerrorZonesGeoJSONFromBoundaries(briefings []model.ZoneBriefingRecord, boundariesPath string) (any, error) {
	return buildZonesFromBoundaries(briefings, boundariesPath, true)
}

func buildRectangleZones(briefings []model.ZoneBriefingRecord, terrorOnly bool) any {
	features := make([]geoFeature, 0)
	for _, lens := range supportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil || brief.Status == "inactive" {
			continue
		}
		if terrorOnly {
			if !(lens.OverlayType == "terror" || contains(brief.ViolenceTypes, "Non-state conflict") || contains(brief.ViolenceTypes, "One-sided violence")) {
				continue
			}
		} else {
			if lens.OverlayType != "conflict" && lens.OverlayType != "maritime" {
				continue
			}
		}
		props := overlayProperties(lens, brief, "", "", "")
		if terrorOnly {
			props["type"] = terrorType(brief.Status)
			props["threat"] = joinOrDefault(brief.Actors, "Structured organized-violence actors")
		} else {
			props["type"] = conflictType(brief)
		}
		features = append(features, geoFeature{
			Type:       "Feature",
			Properties: props,
			Geometry:   rectangleGeometry(lens.Bounds),
		})
	}
	return featureCollection{Type: "FeatureCollection", Features: features}
}

func buildZonesFromBoundaries(briefings []model.ZoneBriefingRecord, boundariesPath string, terrorOnly bool) (any, error) {
	collection, err := readBoundaryCollection(boundariesPath)
	if err != nil {
		return nil, err
	}
	features := make([]geoFeature, 0)
	for _, lens := range supportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil || brief.Status == "inactive" {
			continue
		}
		if terrorOnly {
			if !(lens.OverlayType == "terror" || contains(brief.ViolenceTypes, "Non-state conflict") || contains(brief.ViolenceTypes, "One-sided violence")) {
				continue
			}
		} else {
			if lens.OverlayType != "conflict" && lens.OverlayType != "maritime" {
				continue
			}
		}
		for _, countryCode := range sortedLensCountryCodes(lens) {
			boundary := findBoundaryFeature(collection.Features, countryCode)
			if boundary == nil {
				continue
			}
			ref := ucdpCountryRefs[countryCode]
			props := overlayProperties(lens, brief, countryCode, ref.Label, ref.ID)
			if terrorOnly {
				props["type"] = terrorType(brief.Status)
				props["threat"] = joinOrDefault(brief.Actors, "Structured organized-violence actors")
			} else {
				props["type"] = conflictType(brief)
			}
			features = append(features, geoFeature{
				Type:       "Feature",
				Properties: mergeBoundaryProps(boundary.Properties, props),
				Geometry:   boundary.Geometry,
			})
		}
	}
	return featureCollection{Type: "FeatureCollection", Features: features}, nil
}

func overlayProperties(lens lensDef, brief *model.ZoneBriefingRecord, countryCode string, countryLabel string, countryID string) map[string]any {
	props := map[string]any{
		"name":           brief.Title,
		"lens_id":        brief.LensID,
		"status":         brief.Status,
		"since":          sinceYear(brief.UpdatedAt),
		"source":         brief.Source,
		"source_url":     brief.SourceURL,
		"coverage_note":  brief.CoverageNote,
		"country_ids":    brief.CountryIDs,
		"country_labels": brief.CountryLabels,
	}
	if countryCode != "" {
		props["country_code"] = countryCode
	}
	if countryLabel != "" {
		props["country_label"] = countryLabel
	}
	if countryID != "" {
		props["ucdp_country_id"] = countryID
		props["country_source_url"] = "https://ucdp.uu.se/country/" + countryID
	} else if strings.TrimSpace(lens.ReferenceCountryID) != "" {
		props["country_source_url"] = "https://ucdp.uu.se/country/" + lens.ReferenceCountryID
	}
	return props
}

func rectangleGeometry(b bounds) map[string]any {
	return map[string]any{
		"type": "Polygon",
		"coordinates": [][][]float64{{
			{b.west, b.south},
			{b.west, b.north},
			{b.east, b.north},
			{b.east, b.south},
			{b.west, b.south},
		}},
	}
}

func readBoundaryCollection(path string) (*boundaryCollection, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var collection boundaryCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, err
	}
	return &collection, nil
}

func findBoundaryFeature(features []boundaryFeature, countryCode string) *boundaryFeature {
	want := strings.ToUpper(strings.TrimSpace(countryCode))
	for i := range features {
		props := features[i].Properties
		for _, key := range []string{"country_code", "country_code2", "iso2", "ISO_A2", "shapeGroup"} {
			value := strings.ToUpper(strings.TrimSpace(stringProp(props, key)))
			if value == want {
				return &features[i]
			}
		}
	}
	return nil
}

func mergeBoundaryProps(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func stringProp(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	if value, ok := props[key]; ok {
		if s, ok := value.(string); ok {
			return s
		}
	}
	return ""
}

func sortedLensCountryCodes(lens lensDef) []string {
	out := make([]string, 0, len(lens.CountryCodes))
	for code := range lens.CountryCodes {
		out = append(out, code)
	}
	sortStrings(out)
	return out
}

func sortStrings(values []string) {
	if len(values) < 2 {
		return
	}
	for i := 0; i < len(values)-1; i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func findBrief(briefings []model.ZoneBriefingRecord, lensID string) *model.ZoneBriefingRecord {
	for i := range briefings {
		if briefings[i].LensID == lensID {
			return &briefings[i]
		}
	}
	return nil
}

func conflictType(brief *model.ZoneBriefingRecord) string {
	switch brief.Status {
	case "active":
		if brief.Violence.Primary == "State-based conflict" {
			return "active_war"
		}
		if brief.Violence.Primary == "Non-state conflict" {
			return "insurgency"
		}
		return "active_conflict"
	case "watch":
		return "high_tension"
	default:
		return "frozen_conflict"
	}
}

func terrorType(status string) string {
	switch status {
	case "active":
		return "active"
	case "watch":
		return "elevated"
	default:
		return "degraded"
	}
}

func sinceYear(raw string) string {
	if len(raw) >= 4 {
		return raw[:4]
	}
	return ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func joinOrDefault(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	out := values[0]
	for i := 1; i < len(values) && i < 3; i++ {
		out += ", " + values[i]
	}
	return out
}
