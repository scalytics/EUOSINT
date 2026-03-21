package parse

import (
	"testing"
)

func TestParseUCDPConflicts(t *testing.T) {
	body := []byte(`{
		"TotalCount": 2,
		"TotalPages": 1,
		"Result": [
			{
				"conflict_id": "123",
				"conflict_name": "Sudan: Government",
				"type_of_conflict": "3",
				"intensity_level": 2,
				"gwno_loc": "625",
				"start_date": "2003-02-12",
				"year": 2025,
				"region": "Africa",
				"side_a": "Government of Sudan",
				"side_b": "RSF"
			},
			{
				"conflict_id": "456",
				"conflict_name": "Ukraine: Donbas",
				"type_of_conflict": "4",
				"intensity_level": 2,
				"gwno_loc": "369",
				"year": 2025,
				"region": "Europe",
				"side_a": "Government of Ukraine",
				"side_b": "Russia"
			}
		]
	}`)

	conflicts, err := ParseUCDPConflicts(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
	found := map[string]bool{}
	for _, c := range conflicts {
		found[c.ConflictID] = true
		if c.IntensityLevel != 2 {
			t.Fatalf("expected intensity 2, got %d for %s", c.IntensityLevel, c.ConflictID)
		}
		if c.ConflictID == "123" && c.StartDate != "2003-02-12" {
			t.Fatalf("expected start_date to be parsed, got %q", c.StartDate)
		}
	}
	if !found["123"] || !found["456"] {
		t.Fatal("expected both conflict IDs")
	}
}

func TestParseUCDPConflictsDeduplicates(t *testing.T) {
	body := []byte(`{
		"Result": [
			{"conflict_id": "100", "conflict_name": "Conflict A", "type_of_conflict": "3", "intensity_level": 1, "gwno_loc": "625", "year": 2024},
			{"conflict_id": "100", "conflict_name": "Conflict A", "type_of_conflict": "3", "intensity_level": 2, "gwno_loc": "625", "year": 2025}
		]
	}`)

	conflicts, err := ParseUCDPConflicts(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 deduplicated conflict, got %d", len(conflicts))
	}
	if conflicts[0].IntensityLevel != 2 {
		t.Fatalf("expected highest year entry (intensity 2), got %d", conflicts[0].IntensityLevel)
	}
}

func TestNormalizeConflictType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"3", "Intrastate"},
		{"4", "Internationalized intrastate"},
		{"1", "Extrasystemic"},
		{"2", "Interstate"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		got := NormalizeConflictType(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeConflictType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
