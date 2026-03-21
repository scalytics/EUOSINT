package zonebrief

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
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

const maxWherePrecisionForFootprint = 3

// BuildConflictFootprintsGeoJSON builds event-derived conflict footprints.
// This avoids painting entire countries as active conflict extent.
func BuildConflictFootprintsGeoJSON(briefings []model.ZoneBriefingRecord, items []parse.UCDPItem) any {
	features := make([]geoFeature, 0)
	for _, lens := range SupportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil {
			continue
		}
		if lens.OverlayType != "conflict" && lens.OverlayType != "maritime" {
			continue
		}
		baseProps := overlayProperties(lens, brief, "", "", "", "")
		baseProps["type"] = conflictType(brief)
		baseProps["geometry_role"] = "footprint"
		clusters := clusterLensEventPoints(lens, items)
		for _, cluster := range clusters {
			geom, ok := lensFootprintGeometryFromPoints(cluster)
			if !ok {
				continue
			}
			features = append(features, geoFeature{
				Type:       "Feature",
				Properties: baseProps,
				Geometry:   clipGeometryToBounds(geom, lens.Bounds),
			})
		}
	}
	return featureCollection{Type: "FeatureCollection", Features: features}
}

// FilterZonesGeoJSONByLens keeps only features for a single lens ID.
// On parse failure it returns an empty FeatureCollection.
func FilterZonesGeoJSONByLens(data any, lensID string) any {
	encoded, err := json.Marshal(data)
	if err != nil {
		return featureCollection{Type: "FeatureCollection", Features: []geoFeature{}}
	}
	var parsed featureCollection
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return featureCollection{Type: "FeatureCollection", Features: []geoFeature{}}
	}
	want := strings.TrimSpace(lensID)
	if want == "" {
		return parsed
	}
	filtered := make([]geoFeature, 0, len(parsed.Features))
	for _, feature := range parsed.Features {
		if featureLensID(feature.Properties) != want {
			continue
		}
		filtered = append(filtered, feature)
	}
	return featureCollection{Type: "FeatureCollection", Features: filtered}
}

func buildRectangleZones(briefings []model.ZoneBriefingRecord, terrorOnly bool) any {
	features := make([]geoFeature, 0)
	for _, lens := range SupportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil {
			continue
		}
		if terrorOnly {
			// Terror overlay should be lens-explicit, not a broad proxy for all
			// non-state conflict lenses; otherwise it mirrors conflict coverage.
			if lens.OverlayType != "terror" {
				continue
			}
		} else {
			if lens.OverlayType != "conflict" && lens.OverlayType != "maritime" {
				continue
			}
		}
		props := overlayProperties(lens, brief, "", "", "", "")
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
	for _, lens := range SupportedLenses {
		brief := findBrief(briefings, lens.ID)
		if brief == nil {
			continue
		}
		if terrorOnly {
			if lens.OverlayType != "terror" {
				continue
			}
		} else {
			if lens.OverlayType != "conflict" && lens.OverlayType != "maritime" {
				continue
			}
		}
		for _, countryCode := range sortedOverlayCountryCodes(lens) {
			boundary := findBoundaryFeature(collection.Features, countryCode)
			if boundary == nil {
				continue
			}
			ref := UCDPCountryRefs[countryCode]
			props := overlayProperties(lens, brief, countryCode, ref.Label, ref.ID, "primary")
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

func overlayProperties(lens LensDef, brief *model.ZoneBriefingRecord, countryCode string, countryLabel string, countryID string, countryRole string) map[string]any {
	props := map[string]any{
		"name":           brief.Title,
		"lens_id":        brief.LensID,
		"status":         brief.Status,
		"since":          sinceYear(firstNonEmpty(brief.ConflictStartDate, brief.UpdatedAt)),
		"source":         brief.Source,
		"source_url":     brief.SourceURL,
		"coverage_note":  brief.CoverageNote,
		"country_ids":    brief.CountryIDs,
		"country_labels": brief.CountryLabels,
	}
	if brief.ConflictIntensity != "" {
		props["conflict_intensity"] = brief.ConflictIntensity
	}
	if brief.ConflictType != "" {
		props["conflict_type"] = brief.ConflictType
	}
	if countryCode != "" {
		props["country_code"] = countryCode
	}
	if strings.TrimSpace(countryRole) != "" {
		props["country_role"] = countryRole
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

func rectangleGeometry(b Bounds) map[string]any {
	return map[string]any{
		"type": "Polygon",
		"coordinates": [][][]float64{{
			{b.West, b.South},
			{b.West, b.North},
			{b.East, b.North},
			{b.East, b.South},
			{b.West, b.South},
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

func sortedOverlayCountryCodes(lens LensDef) []string {
	source := lens.OverlayCountryCodes
	if len(source) == 0 {
		source = lens.CountryCodes
	}
	out := make([]string, 0, len(source))
	for code := range source {
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

func featureLensID(props map[string]any) string {
	if props == nil {
		return ""
	}
	if value, ok := props["lens_id"]; ok {
		if s, ok := value.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	if value, ok := props["lensId"]; ok {
		if s, ok := value.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

type geoPoint struct {
	x float64 // lng
	y float64 // lat
}

func clusterLensEventPoints(lens LensDef, items []parse.UCDPItem) [][]geoPoint {
	points := make([]geoPoint, 0)
	for _, item := range items {
		if !isValidFootprintPoint(item) {
			continue
		}
		if !pointWithinBounds(item.Lat, item.Lng, lens.Bounds) {
			continue
		}
		if !matchesLens(lens, item) {
			continue
		}
		points = append(points, geoPoint{x: item.Lng, y: item.Lat})
	}
	if len(points) == 0 {
		return nil
	}

	eps := lensClusterDistance(lens.Bounds)
	visited := make([]bool, len(points))
	clusters := make([][]geoPoint, 0)
	for i := range points {
		if visited[i] {
			continue
		}
		visited[i] = true
		cluster := []geoPoint{points[i]}
		queue := []int{i}
		for len(queue) > 0 {
			idx := queue[0]
			queue = queue[1:]
			for j := range points {
				if visited[j] {
					continue
				}
				if pointDistance(points[idx], points[j]) > eps {
					continue
				}
				visited[j] = true
				cluster = append(cluster, points[j])
				queue = append(queue, j)
			}
		}
		clusters = append(clusters, cluster)
	}
	return clusters
}

func isValidFootprintPoint(item parse.UCDPItem) bool {
	if item.Lat == 0 && item.Lng == 0 {
		return false
	}
	if item.WherePrecision > 0 && item.WherePrecision > maxWherePrecisionForFootprint {
		return false
	}
	return true
}

func pointWithinBounds(lat float64, lng float64, b Bounds) bool {
	return lat >= b.South && lat <= b.North && lng >= b.West && lng <= b.East
}

func lensClusterDistance(bounds Bounds) float64 {
	latSpan := bounds.North - bounds.South
	lngSpan := bounds.East - bounds.West
	span := latSpan
	if lngSpan > span {
		span = lngSpan
	}
	eps := span * 0.12
	if eps < 0.3 {
		return 0.3
	}
	if eps > 1.8 {
		return 1.8
	}
	return eps
}

func pointDistance(a geoPoint, b geoPoint) float64 {
	dx := a.x - b.x
	dy := a.y - b.y
	return math.Hypot(dx, dy)
}

func lensFootprintGeometryFromPoints(points []geoPoint) (map[string]any, bool) {
	if len(points) == 0 {
		return nil, false
	}
	padding := 0.12 + float64(len(points))*0.002
	if padding > 0.4 {
		padding = 0.4
	}
	if len(points) == 1 {
		return circlePolygon(points[0], padding), true
	}
	if len(points) == 2 {
		return segmentBoxPolygon(points[0], points[1], padding), true
	}
	hull := convexHull(points)
	if len(hull) < 3 {
		return segmentBoxPolygon(points[0], points[1], padding), true
	}
	return expandedHullPolygon(hull, padding*0.45), true
}

func clipGeometryToBounds(geom map[string]any, b Bounds) map[string]any {
	typ, _ := geom["type"].(string)
	if typ != "Polygon" {
		return geom
	}
	rawCoords, ok := geom["coordinates"].([][][]float64)
	if !ok {
		return geom
	}
	out := make([][][]float64, 0, len(rawCoords))
	for _, ring := range rawCoords {
		clipped := make([][]float64, 0, len(ring))
		for _, coord := range ring {
			if len(coord) < 2 {
				continue
			}
			lng := coord[0]
			lat := coord[1]
			if lng < b.West {
				lng = b.West
			}
			if lng > b.East {
				lng = b.East
			}
			if lat < b.South {
				lat = b.South
			}
			if lat > b.North {
				lat = b.North
			}
			clipped = append(clipped, []float64{lng, lat})
		}
		if len(clipped) > 0 {
			if clipped[0][0] != clipped[len(clipped)-1][0] || clipped[0][1] != clipped[len(clipped)-1][1] {
				clipped = append(clipped, []float64{clipped[0][0], clipped[0][1]})
			}
			out = append(out, clipped)
		}
	}
	return map[string]any{
		"type":        "Polygon",
		"coordinates": out,
	}
}

func lensFootprintGeometry(hotspots []model.ZoneBriefingHotspot) (map[string]any, bool) {
	points := make([]geoPoint, 0, len(hotspots))
	totalEvents := 0
	for _, hotspot := range hotspots {
		if hotspot.Lat == 0 && hotspot.Lng == 0 {
			continue
		}
		points = append(points, geoPoint{x: hotspot.Lng, y: hotspot.Lat})
		totalEvents += hotspot.EventCount
	}
	if len(points) == 0 {
		return nil, false
	}
	padding := 0.12 + float64(totalEvents)*0.005
	if padding > 0.4 {
		padding = 0.4
	}
	if len(points) == 1 {
		return circlePolygon(points[0], padding), true
	}
	if len(points) == 2 {
		return segmentBoxPolygon(points[0], points[1], padding), true
	}
	hull := convexHull(points)
	if len(hull) < 3 {
		return segmentBoxPolygon(points[0], points[1], padding), true
	}
	return expandedHullPolygon(hull, padding*0.45), true
}

func circlePolygon(center geoPoint, radius float64) map[string]any {
	const steps = 24
	coords := make([][]float64, 0, steps+1)
	for i := 0; i < steps; i++ {
		theta := (2 * math.Pi * float64(i)) / float64(steps)
		coords = append(coords, []float64{
			center.x + radius*math.Cos(theta),
			center.y + radius*math.Sin(theta),
		})
	}
	coords = append(coords, coords[0])
	return map[string]any{
		"type":        "Polygon",
		"coordinates": [][][]float64{coords},
	}
}

func segmentBoxPolygon(a geoPoint, b geoPoint, pad float64) map[string]any {
	minX, maxX := a.x, b.x
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := a.y, b.y
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return map[string]any{
		"type": "Polygon",
		"coordinates": [][][]float64{{
			{minX - pad, minY - pad},
			{minX - pad, maxY + pad},
			{maxX + pad, maxY + pad},
			{maxX + pad, minY - pad},
			{minX - pad, minY - pad},
		}},
	}
}

func expandedHullPolygon(hull []geoPoint, pad float64) map[string]any {
	cx, cy := 0.0, 0.0
	for _, p := range hull {
		cx += p.x
		cy += p.y
	}
	cx /= float64(len(hull))
	cy /= float64(len(hull))

	coords := make([][]float64, 0, len(hull)+1)
	for _, p := range hull {
		dx := p.x - cx
		dy := p.y - cy
		d := math.Hypot(dx, dy)
		if d < 1e-6 {
			coords = append(coords, []float64{p.x, p.y})
			continue
		}
		scale := (d + pad) / d
		coords = append(coords, []float64{cx + dx*scale, cy + dy*scale})
	}
	coords = append(coords, coords[0])
	return map[string]any{
		"type":        "Polygon",
		"coordinates": [][][]float64{coords},
	}
}

func convexHull(points []geoPoint) []geoPoint {
	if len(points) <= 1 {
		return points
	}
	pts := append([]geoPoint(nil), points...)
	for i := 0; i < len(pts)-1; i++ {
		for j := i + 1; j < len(pts); j++ {
			if pts[j].x < pts[i].x || (pts[j].x == pts[i].x && pts[j].y < pts[i].y) {
				pts[i], pts[j] = pts[j], pts[i]
			}
		}
	}

	lower := make([]geoPoint, 0, len(pts))
	for _, p := range pts {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}

	upper := make([]geoPoint, 0, len(pts))
	for i := len(pts) - 1; i >= 0; i-- {
		p := pts[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}

	hull := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	return hull
}

func cross(a, b, c geoPoint) float64 {
	return (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
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
