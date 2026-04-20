// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"database/sql"
	"encoding/json"
)

type Entity struct {
	ID          string
	Type        string
	CanonicalID string
	DisplayName string
	FirstSeen   string
	LastSeen    string
	Attrs       map[string]any
}

type Edge struct {
	SrcID       string
	DstID       string
	Type        string
	ValidFrom   string
	ValidTo     string
	EvidenceMsg string
	Weight      float64
	Attrs       map[string]any
}

type Provenance struct {
	SubjectKind string
	SubjectID   string
	Stage       string
	PolicyVer   string
	Inputs      map[string]any
	Decision    string
	Reasons     []string
	ProducedAt  string
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
