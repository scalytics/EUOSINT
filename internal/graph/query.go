package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type TimeRange struct {
	Start string
	End   string
}

type Profile struct {
	Entity       Entity
	FirstSeen    string
	LastSeen     string
	EdgeCounts   map[string]int
	TopNeighbors []Neighbor
}

type Neighbor struct {
	EntityID   string
	EntityType string
	Weight     float64
}

type Reader struct {
	db *sql.DB
}

func NewReader(db *sql.DB) *Reader {
	return &Reader{db: db}
}

func (r *Reader) Neighborhood(ctx context.Context, entityID string, depth int, typeFilter []string, window TimeRange) ([]Entity, []Edge, error) {
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	clauses, args := edgeWindowClause(window)
	edgeFilter, edgeArgs := edgeTypeClause("e.type", typeFilter)
	if edgeFilter != "" {
		clauses = append(clauses, edgeFilter)
		args = append(args, edgeArgs...)
	}
	where := ""
	if len(clauses) > 0 {
		where = " AND " + strings.Join(clauses, " AND ")
	}

	query := fmt.Sprintf(`
		WITH RECURSIVE walk(depth, entity_id) AS (
			SELECT 0, ?
			UNION
			SELECT DISTINCT walk.depth + 1,
			       CASE WHEN e.src_id = walk.entity_id THEN e.dst_id ELSE e.src_id END
			  FROM walk
			  JOIN edges e ON (e.src_id = walk.entity_id OR e.dst_id = walk.entity_id)
			 WHERE walk.depth < ? %s
		)
		SELECT DISTINCT e.id, e.type, e.canonical_id, e.display_name, e.first_seen, e.last_seen, e.attrs_json
		  FROM walk
		  JOIN entities e ON e.id = walk.entity_id
		 ORDER BY e.last_seen DESC, e.id DESC
	`, where)
	args = append([]any{entityID, depth}, args...)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	entities := []Entity{}
	ids := make([]string, 0)
	idset := map[string]struct{}{}
	for rows.Next() {
		entity, err := scanEntityRow(rows)
		if err != nil {
			return nil, nil, err
		}
		entities = append(entities, entity)
		if _, ok := idset[entity.ID]; !ok {
			ids = append(ids, entity.ID)
			idset[entity.ID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	edges, err := r.edgesBetween(ctx, ids, typeFilter, window)
	if err != nil {
		return nil, nil, err
	}
	return entities, edges, nil
}

func (r *Reader) Path(ctx context.Context, src, dst string, maxDepth int, window TimeRange) ([]Edge, bool, error) {
	if maxDepth <= 0 {
		maxDepth = 1
	}
	if maxDepth > 3 {
		maxDepth = 3
	}
	entities, edges, err := r.Neighborhood(ctx, src, maxDepth, nil, window)
	if err != nil {
		return nil, false, err
	}
	if len(entities) == 0 {
		return nil, false, nil
	}
	adj := map[string][]Edge{}
	for _, edge := range edges {
		adj[edge.SrcID] = append(adj[edge.SrcID], edge)
		reverse := edge
		reverse.SrcID, reverse.DstID = edge.DstID, edge.SrcID
		adj[reverse.SrcID] = append(adj[reverse.SrcID], reverse)
	}

	type step struct {
		id    string
		depth int
	}
	queue := []step{{id: src, depth: 0}}
	seen := map[string]bool{src: true}
	parentNode := map[string]string{}
	parentEdge := map[string]Edge{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == dst {
			break
		}
		if cur.depth >= maxDepth {
			continue
		}
		for _, edge := range adj[cur.id] {
			if seen[edge.DstID] {
				continue
			}
			seen[edge.DstID] = true
			parentNode[edge.DstID] = cur.id
			parentEdge[edge.DstID] = edge
			queue = append(queue, step{id: edge.DstID, depth: cur.depth + 1})
		}
	}
	if !seen[dst] {
		return nil, false, nil
	}
	path := []Edge{}
	for cur := dst; cur != src; cur = parentNode[cur] {
		edge := parentEdge[cur]
		if stored, err := r.loadEdgeByShape(ctx, edge); err == nil {
			path = append(path, stored)
		} else {
			path = append(path, edge)
		}
	}
	reverseEdges(path)
	return path, true, nil
}

func (r *Reader) EntityProfile(ctx context.Context, entityID string) (Profile, error) {
	entity, err := r.entityByID(ctx, entityID)
	if err != nil {
		return Profile{}, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT type, COUNT(*)
		  FROM edges
		 WHERE src_id = ? OR dst_id = ?
		 GROUP BY type
	`, entityID, entityID)
	if err != nil {
		return Profile{}, err
	}
	defer rows.Close()

	profile := Profile{
		Entity:     entity,
		FirstSeen:  entity.FirstSeen,
		LastSeen:   entity.LastSeen,
		EdgeCounts: map[string]int{},
	}
	for rows.Next() {
		var edgeType string
		var count int
		if err := rows.Scan(&edgeType, &count); err != nil {
			return Profile{}, err
		}
		profile.EdgeCounts[edgeType] = count
	}
	if err := rows.Err(); err != nil {
		return Profile{}, err
	}

	neighborRows, err := r.db.QueryContext(ctx, `
		SELECT n.id, n.type, SUM(e.weight) AS total_weight
		  FROM edges e
		  JOIN entities n ON n.id = CASE WHEN e.src_id = ? THEN e.dst_id ELSE e.src_id END
		 WHERE e.src_id = ? OR e.dst_id = ?
		 GROUP BY n.id, n.type
		 ORDER BY total_weight DESC, n.id ASC
		 LIMIT 5
	`, entityID, entityID, entityID)
	if err != nil {
		return Profile{}, err
	}
	defer neighborRows.Close()
	for neighborRows.Next() {
		var neighbor Neighbor
		if err := neighborRows.Scan(&neighbor.EntityID, &neighbor.EntityType, &neighbor.Weight); err != nil {
			return Profile{}, err
		}
		profile.TopNeighbors = append(profile.TopNeighbors, neighbor)
	}
	return profile, neighborRows.Err()
}

func (r *Reader) Geometry(ctx context.Context, entityID string) (Geometry, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT entity_id, geometry_type, geojson, srid, min_lat, min_lon, max_lat, max_lon,
		       z_min, z_max, observed_at, valid_to
		  FROM entity_geometry
		 WHERE entity_id = ?
	`, entityID)
	return scanGeometryRow(row)
}

func (r *Reader) WithinBBox(ctx context.Context, bbox [4]float64, typeFilter []string, window TimeRange) ([]Entity, []Geometry, error) {
	args := []any{bbox[3], bbox[1], bbox[2], bbox[0]}
	query := `
		SELECT e.id, e.type, e.canonical_id, e.display_name, e.first_seen, e.last_seen, e.attrs_json,
		       g.entity_id, g.geometry_type, g.geojson, g.srid, g.min_lat, g.min_lon, g.max_lat, g.max_lon,
		       g.z_min, g.z_max, g.observed_at, g.valid_to
		  FROM entity_geometry g
		  JOIN entities e ON e.id = g.entity_id
		 WHERE g.min_lat <= ? AND g.max_lat >= ? AND g.min_lon <= ? AND g.max_lon >= ?
	`
	if len(typeFilter) > 0 {
		filter, filterArgs := edgeTypeClause("e.type", typeFilter)
		query += " AND " + filter
		args = append(args, filterArgs...)
	}
	if window.Start != "" {
		query += " AND g.observed_at >= ?"
		args = append(args, window.Start)
	}
	if window.End != "" {
		query += " AND g.observed_at <= ?"
		args = append(args, window.End)
	}
	query += " ORDER BY e.last_seen DESC, e.id DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var entities []Entity
	var geometries []Geometry
	for rows.Next() {
		var entity Entity
		var geometry Geometry
		var geojson string
		var attrs string
		var displayName sql.NullString
		var zMin, zMax sql.NullFloat64
		var validTo sql.NullString
		if err := rows.Scan(
			&entity.ID, &entity.Type, &entity.CanonicalID, &displayName, &entity.FirstSeen, &entity.LastSeen, &attrs,
			&geometry.EntityID, &geometry.GeometryType, &geojson, &geometry.SRID, &geometry.MinLat, &geometry.MinLon, &geometry.MaxLat, &geometry.MaxLon,
			&zMin, &zMax, &geometry.ObservedAt, &validTo,
		); err != nil {
			return nil, nil, err
		}
		geometry.GeoJSON = json.RawMessage(geojson)
		entity.DisplayName = displayName.String
		if attrs != "" {
			_ = json.Unmarshal([]byte(attrs), &entity.Attrs)
		}
		if zMin.Valid {
			geometry.ZMin = &zMin.Float64
		}
		if zMax.Valid {
			geometry.ZMax = &zMax.Float64
		}
		geometry.ValidTo = validTo.String
		entities = append(entities, entity)
		geometries = append(geometries, geometry)
	}
	return entities, geometries, rows.Err()
}

func (r *Reader) Intersects(ctx context.Context, entityID, areaID string, window TimeRange) (bool, error) {
	left, err := r.Geometry(ctx, entityID)
	if err != nil {
		return false, err
	}
	right, err := r.Geometry(ctx, areaID)
	if err != nil {
		return false, err
	}
	if !rangesOverlap(left.MinLat, left.MaxLat, right.MinLat, right.MaxLat) || !rangesOverlap(left.MinLon, left.MaxLon, right.MinLon, right.MaxLon) {
		return false, nil
	}
	return geoIntersects(left, right), nil
}

func (r *Reader) Nearby(ctx context.Context, point [2]float64, radiusMeters float64, typeFilter []string, window TimeRange) ([]Entity, []Geometry, error) {
	latRadius := radiusMeters / 111_320.0
	lonRadius := radiusMeters / (111_320.0 * math.Max(math.Cos(point[1]*math.Pi/180), 0.01))
	bbox := [4]float64{point[0] - lonRadius, point[1] - latRadius, point[0] + lonRadius, point[1] + latRadius}
	entities, geometries, err := r.WithinBBox(ctx, bbox, typeFilter, window)
	if err != nil {
		return nil, nil, err
	}
	filteredEntities := make([]Entity, 0, len(entities))
	filteredGeometries := make([]Geometry, 0, len(geometries))
	for i := range geometries {
		if distanceToGeometryMeters(point, geometries[i]) <= radiusMeters {
			filteredEntities = append(filteredEntities, entities[i])
			filteredGeometries = append(filteredGeometries, geometries[i])
		}
	}
	return filteredEntities, filteredGeometries, nil
}

func (r *Reader) entityByID(ctx context.Context, entityID string) (Entity, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, type, canonical_id, display_name, first_seen, last_seen, attrs_json
		  FROM entities
		 WHERE id = ?
	`, entityID)
	return scanEntityRow(row)
}

func (r *Reader) edgesBetween(ctx context.Context, ids []string, typeFilter []string, window TimeRange) ([]Edge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(ids)*2)
	for _, id := range ids {
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT id, src_id, dst_id, type, valid_from, valid_to, evidence_msg, weight, attrs_json
		  FROM edges
		 WHERE src_id IN (%s) AND dst_id IN (%s)
	`, placeholders, placeholders)
	if filter, filterArgs := edgeTypeClause("type", typeFilter); filter != "" {
		query += " AND " + filter
		args = append(args, filterArgs...)
	}
	if clauses, windowArgs := edgeWindowClause(window); len(clauses) > 0 {
		query += " AND " + strings.Join(clauses, " AND ")
		args = append(args, windowArgs...)
	}
	query += " ORDER BY valid_from DESC, id DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		_, edge, err := scanEdgeRow(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

func (r *Reader) loadEdgeByShape(ctx context.Context, edge Edge) (Edge, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, src_id, dst_id, type, valid_from, valid_to, evidence_msg, weight, attrs_json
		  FROM edges
		 WHERE src_id = ? AND dst_id = ? AND type = ? AND valid_from = ?
		 ORDER BY id DESC
		 LIMIT 1
	`, edge.SrcID, edge.DstID, edge.Type, edge.ValidFrom)
	_, stored, err := scanEdgeRow(row)
	return stored, err
}

func scanEntityRow(scanner interface{ Scan(...any) error }) (Entity, error) {
	var entity Entity
	var displayName sql.NullString
	var attrs string
	if err := scanner.Scan(&entity.ID, &entity.Type, &entity.CanonicalID, &displayName, &entity.FirstSeen, &entity.LastSeen, &attrs); err != nil {
		return Entity{}, err
	}
	entity.DisplayName = displayName.String
	if attrs != "" && attrs != "{}" {
		_ = json.Unmarshal([]byte(attrs), &entity.Attrs)
	}
	return entity, nil
}

func scanEdgeRow(scanner interface{ Scan(...any) error }) (int64, Edge, error) {
	var id int64
	var edge Edge
	var validTo, evidenceMsg sql.NullString
	var attrs string
	if err := scanner.Scan(&id, &edge.SrcID, &edge.DstID, &edge.Type, &edge.ValidFrom, &validTo, &evidenceMsg, &edge.Weight, &attrs); err != nil {
		return 0, Edge{}, err
	}
	edge.ValidTo = validTo.String
	edge.EvidenceMsg = evidenceMsg.String
	if attrs != "" && attrs != "{}" {
		_ = json.Unmarshal([]byte(attrs), &edge.Attrs)
	}
	return id, edge, nil
}

func scanGeometryRow(scanner interface{ Scan(...any) error }) (Geometry, error) {
	var geometry Geometry
	var geojson string
	var zMin, zMax sql.NullFloat64
	var validTo sql.NullString
	if err := scanner.Scan(&geometry.EntityID, &geometry.GeometryType, &geojson, &geometry.SRID, &geometry.MinLat, &geometry.MinLon, &geometry.MaxLat, &geometry.MaxLon, &zMin, &zMax, &geometry.ObservedAt, &validTo); err != nil {
		return Geometry{}, err
	}
	geometry.GeoJSON = json.RawMessage(geojson)
	if zMin.Valid {
		geometry.ZMin = &zMin.Float64
	}
	if zMax.Valid {
		geometry.ZMax = &zMax.Float64
	}
	geometry.ValidTo = validTo.String
	return geometry, nil
}

func edgeTypeClause(column string, values []string) (string, []any) {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return "", nil
	}
	args := make([]any, 0, len(filtered))
	parts := make([]string, 0, len(filtered))
	for _, value := range filtered {
		parts = append(parts, "?")
		args = append(args, value)
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(parts, ",")), args
}

func edgeWindowClause(window TimeRange) ([]string, []any) {
	clauses := []string{}
	args := []any{}
	if window.End != "" {
		clauses = append(clauses, "e.valid_from <= ?")
		args = append(args, window.End)
	}
	if window.Start != "" {
		clauses = append(clauses, "(e.valid_to IS NULL OR e.valid_to >= ?)")
		args = append(args, window.Start)
	}
	return clauses, args
}

func reverseEdges(edges []Edge) {
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}
}

func rangesOverlap(aMin, aMax, bMin, bMax float64) bool {
	return aMin <= bMax && aMax >= bMin
}

func geoIntersects(left, right Geometry) bool {
	leftPoints := extractPoints(left.GeoJSON)
	rightPoints := extractPoints(right.GeoJSON)
	if len(leftPoints) == 0 || len(rightPoints) == 0 {
		return true
	}
	for _, point := range leftPoints {
		if pointInGeometry(point, right.GeoJSON) {
			return true
		}
	}
	for _, point := range rightPoints {
		if pointInGeometry(point, left.GeoJSON) {
			return true
		}
	}
	return false
}

func distanceToGeometryMeters(point [2]float64, geometry Geometry) float64 {
	points := extractPoints(geometry.GeoJSON)
	if len(points) == 0 {
		return math.Inf(1)
	}
	best := math.Inf(1)
	for _, candidate := range points {
		best = math.Min(best, haversineMeters(point[1], point[0], candidate[1], candidate[0]))
	}
	return best
}

func extractPoints(raw json.RawMessage) [][2]float64 {
	var payload struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	switch payload.Type {
	case "Point":
		var coords []float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil && len(coords) >= 2 {
			return [][2]float64{{coords[0], coords[1]}}
		}
	case "LineString", "MultiPoint":
		var coords [][]float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil {
			return collectPoints(coords)
		}
	case "Polygon":
		var coords [][][]float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil {
			points := make([][2]float64, 0)
			for _, ring := range coords {
				points = append(points, collectPoints(ring)...)
			}
			return points
		}
	case "MultiPolygon":
		var coords [][][][]float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil {
			points := make([][2]float64, 0)
			for _, poly := range coords {
				for _, ring := range poly {
					points = append(points, collectPoints(ring)...)
				}
			}
			return points
		}
	}
	return nil
}

func collectPoints(coords [][]float64) [][2]float64 {
	points := make([][2]float64, 0, len(coords))
	for _, coord := range coords {
		if len(coord) >= 2 {
			points = append(points, [2]float64{coord[0], coord[1]})
		}
	}
	return points
}

func pointInGeometry(point [2]float64, raw json.RawMessage) bool {
	var payload struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	switch payload.Type {
	case "Point":
		var coords []float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil && len(coords) >= 2 {
			return almostEqual(point[0], coords[0]) && almostEqual(point[1], coords[1])
		}
	case "Polygon":
		var coords [][][]float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil {
			for _, ring := range coords {
				if pointInRing(point, ring) {
					return true
				}
			}
		}
	case "MultiPolygon":
		var coords [][][][]float64
		if json.Unmarshal(payload.Coordinates, &coords) == nil {
			for _, poly := range coords {
				for _, ring := range poly {
					if pointInRing(point, ring) {
						return true
					}
				}
			}
		}
	default:
		for _, candidate := range extractPoints(raw) {
			if almostEqual(point[0], candidate[0]) && almostEqual(point[1], candidate[1]) {
				return true
			}
		}
	}
	return false
}

func pointInRing(point [2]float64, ring [][]float64) bool {
	if len(ring) < 3 {
		return false
	}
	inside := false
	j := len(ring) - 1
	for i := 0; i < len(ring); i++ {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		intersects := ((yi > point[1]) != (yj > point[1])) &&
			(point[0] < (xj-xi)*(point[1]-yi)/(yj-yi+1e-12)+xi)
		if intersects {
			inside = !inside
		}
		j = i
	}
	return inside
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

func (r *Reader) BenchmarkNeighborhoodData() error {
	return errors.New("benchmarks are deferred to W12")
}

func sortNeighbors(neighbors []Neighbor) {
	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].Weight == neighbors[j].Weight {
			return neighbors[i].EntityID < neighbors[j].EntityID
		}
		return neighbors[i].Weight > neighbors[j].Weight
	})
}
