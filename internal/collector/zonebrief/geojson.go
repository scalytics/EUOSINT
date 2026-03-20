package zonebrief

import "github.com/scalytics/euosint/internal/collector/model"

type featureCollection struct {
	Type     string        `json:"type"`
	Features []geoFeature  `json:"features"`
}

type geoFeature struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Geometry   geometry       `json:"geometry"`
}

type geometry struct {
	Type        string        `json:"type"`
	Coordinates [][][]float64 `json:"coordinates"`
}

func BuildConflictZonesGeoJSON(briefings []model.ZoneBriefingRecord) any {
	features := make([]geoFeature, 0)
	for _, lens := range supportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil || brief.Status == "inactive" {
			continue
		}
		if lens.OverlayType != "conflict" && lens.OverlayType != "maritime" {
			continue
		}
		features = append(features, geoFeature{
			Type: "Feature",
			Properties: map[string]any{
				"name":    brief.Title,
				"type":    conflictType(brief),
				"since":   sinceYear(brief.UpdatedAt),
				"lens_id": brief.LensID,
				"status":  brief.Status,
			},
			Geometry: rectangleGeometry(lens.Bounds),
		})
	}
	return featureCollection{Type: "FeatureCollection", Features: features}
}

func BuildTerrorZonesGeoJSON(briefings []model.ZoneBriefingRecord) any {
	features := make([]geoFeature, 0)
	for _, lens := range supportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil || brief.Status == "inactive" {
			continue
		}
		if !(lens.OverlayType == "terror" || contains(brief.ViolenceTypes, "Non-state conflict") || contains(brief.ViolenceTypes, "One-sided violence")) {
			continue
		}
		features = append(features, geoFeature{
			Type: "Feature",
			Properties: map[string]any{
				"name":    brief.Title,
				"type":    terrorType(brief.Status),
				"threat":  joinOrDefault(brief.Actors, "Structured organized-violence actors"),
				"since":   sinceYear(brief.UpdatedAt),
				"lens_id": brief.LensID,
				"status":  brief.Status,
			},
			Geometry: rectangleGeometry(lens.Bounds),
		})
	}
	return featureCollection{Type: "FeatureCollection", Features: features}
}

func rectangleGeometry(b bounds) geometry {
	return geometry{
		Type: "Polygon",
		Coordinates: [][][]float64{{
			{b.west, b.south},
			{b.west, b.north},
			{b.east, b.north},
			{b.east, b.south},
			{b.west, b.south},
		}},
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
