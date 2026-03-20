package zonebrief

import (
	"encoding/json"
	"testing"

	"github.com/scalytics/euosint/internal/collector/model"
)

func TestBuildConflictZonesGeoJSONSkipsInactive(t *testing.T) {
	data := BuildConflictZonesGeoJSON([]model.ZoneBriefingRecord{
		{LensID: "gaza", Title: "Gaza", Status: "active", UpdatedAt: "2026-03-20T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
		{LensID: "ukraine", Title: "Ukraine South", Status: "inactive", UpdatedAt: "2026-03-20T00:00:00Z", Violence: model.ZoneBriefingViolence{Primary: "State-based conflict"}},
	})
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Features) != 1 {
		t.Fatalf("expected 1 active feature, got %d", len(parsed.Features))
	}
}
