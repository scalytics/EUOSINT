// Copyright 2025 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type UCDPConflict struct {
	ConflictID     string
	ConflictName   string
	TypeOfConflict string // "3" = intrastate, "4" = internationalized intrastate, etc.
	IntensityLevel int    // 1 = minor, 2 = war
	GWNoLoc        string // country gwno
	Year           int
	Region         string
	SideA          string
	SideB          string
}

func ParseUCDPConflicts(body []byte) ([]UCDPConflict, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}

	raw := firstRaw(envelope, "Result", "result", "results", "Data", "data")
	if len(raw) == 0 {
		return nil, fmt.Errorf("UCDP conflict response missing result array")
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}

	out := make([]UCDPConflict, 0, len(rows))
	for _, row := range rows {
		conflictID := firstString(row, "conflict_id")
		if conflictID == "" {
			continue
		}
		year := 0
		if v := firstString(row, "year"); v != "" {
			year, _ = strconv.Atoi(v)
		}
		intensity := firstInt(row, "intensity_level")
		out = append(out, UCDPConflict{
			ConflictID:     conflictID,
			ConflictName:   firstString(row, "conflict_name", "side_a", "incompatibility"),
			TypeOfConflict: firstString(row, "type_of_conflict"),
			IntensityLevel: intensity,
			GWNoLoc:        firstString(row, "gwno_loc", "gwno_a"),
			Year:           year,
			Region:         firstString(row, "region"),
			SideA:          firstString(row, "side_a"),
			SideB:          firstString(row, "side_b", "side_b_2nd"),
		})
	}

	// Deduplicate by conflict_id keeping highest year.
	best := map[string]UCDPConflict{}
	for _, c := range out {
		if existing, ok := best[c.ConflictID]; !ok || c.Year > existing.Year {
			best[c.ConflictID] = c
		}
	}
	deduped := make([]UCDPConflict, 0, len(best))
	for _, c := range best {
		deduped = append(deduped, c)
	}
	return deduped, nil
}

func NormalizeConflictType(typeCode string) string {
	switch strings.TrimSpace(typeCode) {
	case "1":
		return "Extrasystemic"
	case "2":
		return "Interstate"
	case "3":
		return "Intrastate"
	case "4":
		return "Internationalized intrastate"
	default:
		return typeCode
	}
}
