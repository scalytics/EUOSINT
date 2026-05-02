// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"database/sql"
	"encoding/json"
)

type Entity struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	CanonicalID string         `json:"canonical_id"`
	DisplayName string         `json:"display_name,omitempty"`
	FirstSeen   string         `json:"first_seen"`
	LastSeen    string         `json:"last_seen"`
	Attrs       map[string]any `json:"attrs,omitempty"`
}

type Edge struct {
	SrcID       string         `json:"src_id"`
	DstID       string         `json:"dst_id"`
	Type        string         `json:"type"`
	ValidFrom   string         `json:"valid_from"`
	ValidTo     string         `json:"valid_to,omitempty"`
	EvidenceMsg string         `json:"evidence_msg,omitempty"`
	Weight      float64        `json:"weight"`
	Attrs       map[string]any `json:"attrs,omitempty"`
}

type Provenance struct {
	SubjectKind string         `json:"subject_kind"`
	SubjectID   string         `json:"subject_id"`
	Stage       string         `json:"stage"`
	PolicyVer   string         `json:"policy_ver,omitempty"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	Decision    string         `json:"decision,omitempty"`
	Reasons     []string       `json:"reasons,omitempty"`
	ProducedAt  string         `json:"produced_at"`
}

type Geometry struct {
	EntityID     string          `json:"entity_id"`
	GeometryType string          `json:"geometry_type"`
	GeoJSON      json.RawMessage `json:"geojson"`
	SRID         int             `json:"srid"`
	MinLat       float64         `json:"min_lat"`
	MinLon       float64         `json:"min_lon"`
	MaxLat       float64         `json:"max_lat"`
	MaxLon       float64         `json:"max_lon"`
	ZMin         *float64        `json:"z_min,omitempty"`
	ZMax         *float64        `json:"z_max,omitempty"`
	ObservedAt   string          `json:"observed_at"`
	ValidTo      string          `json:"valid_to,omitempty"`
}

func UpsertEntity(tx *sql.Tx, entity Entity) error {
	attrs, err := marshalJSON(entity.Attrs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO entities (id, type, canonical_id, display_name, first_seen, last_seen, attrs_json)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			display_name = COALESCE(NULLIF(excluded.display_name, ''), entities.display_name),
			last_seen = CASE
				WHEN excluded.last_seen > entities.last_seen THEN excluded.last_seen
				ELSE entities.last_seen
			END,
			attrs_json = excluded.attrs_json
	`, entity.ID, entity.Type, entity.CanonicalID, entity.DisplayName, entity.FirstSeen, entity.LastSeen, attrs)
	return err
}

func AppendEdge(tx *sql.Tx, edge Edge) (int64, bool, error) {
	attrs, err := marshalJSON(edge.Attrs)
	if err != nil {
		return 0, false, err
	}
	if edge.Weight == 0 {
		edge.Weight = 1
	}
	res, err := tx.Exec(`
		INSERT OR IGNORE INTO edges (src_id, dst_id, type, valid_from, valid_to, evidence_msg, weight, attrs_json)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)
	`, edge.SrcID, edge.DstID, edge.Type, edge.ValidFrom, edge.ValidTo, edge.EvidenceMsg, edge.Weight, attrs)
	if err != nil {
		return 0, false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if rows == 0 {
		var id int64
		err = tx.QueryRow(`
			SELECT id FROM edges
			WHERE src_id = ? AND dst_id = ? AND type = ? AND valid_from = ?
			  AND IFNULL(valid_to, '') = IFNULL(NULLIF(?, ''), '')
			  AND IFNULL(evidence_msg, '') = IFNULL(NULLIF(?, ''), '')
		`, edge.SrcID, edge.DstID, edge.Type, edge.ValidFrom, edge.ValidTo, edge.EvidenceMsg).Scan(&id)
		return id, false, err
	}
	id, err := res.LastInsertId()
	return id, true, err
}

func AppendProvenance(tx *sql.Tx, provenance Provenance) error {
	inputs, err := marshalJSON(provenance.Inputs)
	if err != nil {
		return err
	}
	reasons, err := marshalJSON(provenance.Reasons)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO provenance (subject_kind, subject_id, stage, policy_ver, inputs_json, decision, reasons_json, produced_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?)
	`, provenance.SubjectKind, provenance.SubjectID, provenance.Stage, provenance.PolicyVer, inputs, provenance.Decision, reasons, provenance.ProducedAt)
	return err
}

func CloseOpenEdges(tx *sql.Tx, dstID string, edgeTypes []string, validTo string) error {
	if len(edgeTypes) == 0 {
		return nil
	}
	args := make([]any, 0, len(edgeTypes)+2)
	query := `UPDATE edges SET valid_to = ? WHERE dst_id = ? AND valid_to IS NULL AND type IN (`
	args = append(args, validTo, dstID)
	for i, edgeType := range edgeTypes {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, edgeType)
	}
	query += ")"
	_, err := tx.Exec(query, args...)
	return err
}

func UpsertGeometry(tx *sql.Tx, geometry Geometry) error {
	srid := geometry.SRID
	if srid == 0 {
		srid = 4326
	}
	_, err := tx.Exec(`
		INSERT INTO entity_geometry (
			entity_id, geometry_type, geojson, srid, min_lat, min_lon, max_lat, max_lon,
			z_min, z_max, observed_at, valid_to
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
		ON CONFLICT(entity_id) DO UPDATE SET
			geometry_type = excluded.geometry_type,
			geojson = excluded.geojson,
			srid = excluded.srid,
			min_lat = excluded.min_lat,
			min_lon = excluded.min_lon,
			max_lat = excluded.max_lat,
			max_lon = excluded.max_lon,
			z_min = excluded.z_min,
			z_max = excluded.z_max,
			observed_at = excluded.observed_at,
			valid_to = excluded.valid_to
	`, geometry.EntityID, geometry.GeometryType, string(geometry.GeoJSON), srid,
		geometry.MinLat, geometry.MinLon, geometry.MaxLat, geometry.MaxLon,
		nilableFloat(geometry.ZMin), nilableFloat(geometry.ZMax), geometry.ObservedAt, geometry.ValidTo)
	return err
}

func marshalJSON(v any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func nilableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
