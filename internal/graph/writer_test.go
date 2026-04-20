package graph

import (
	"path/filepath"
	"testing"

	agentopschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
	"github.com/scalytics/kafSIEM/internal/graph/schema"
)

func TestEntityID(t *testing.T) {
	if got := EntityID("agent", "alice"); got != "agent:alice" {
		t.Fatalf("EntityID() = %q", got)
	}
	if got := EntityID("", "alice"); got != "" {
		t.Fatalf("expected empty entity id, got %q", got)
	}
}

func TestUpsertEntityAndAppendEdge(t *testing.T) {
	db, err := agentopschema.Open(filepath.Join(t.TempDir(), "agentops.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := schema.Apply(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages (record_id, topic, topic_family, partition, offset, timestamp, outcome) VALUES ('msg-1', 'group.core.requests', 'requests', 0, 1, '2026-04-20T10:00:00Z', 'accepted')`); err != nil {
		t.Fatal(err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertEntity(tx, Entity{
		ID:          EntityID("agent", "alice"),
		Type:        "agent",
		CanonicalID: "alice",
		DisplayName: "Alice",
		FirstSeen:   "2026-04-20T10:00:00Z",
		LastSeen:    "2026-04-20T10:00:00Z",
		Attrs:       map[string]any{"role": "requester"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertEntity(tx, Entity{
		ID:          EntityID("task", "task-1"),
		Type:        "task",
		CanonicalID: "task-1",
		FirstSeen:   "2026-04-20T10:00:00Z",
		LastSeen:    "2026-04-20T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	edgeID, inserted, err := AppendEdge(tx, Edge{
		SrcID:       EntityID("agent", "alice"),
		DstID:       EntityID("task", "task-1"),
		Type:        "sent",
		ValidFrom:   "2026-04-20T10:00:00Z",
		EvidenceMsg: "msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inserted || edgeID == 0 {
		t.Fatalf("expected inserted edge, got id=%d inserted=%v", edgeID, inserted)
	}
	if err := AppendProvenance(tx, Provenance{
		SubjectKind: "edge",
		SubjectID:   "1",
		Stage:       "graph",
		Decision:    "inserted",
		Reasons:     []string{"requests"},
		ProducedAt:  "2026-04-20T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	dupID, inserted, err := AppendEdge(tx, Edge{
		SrcID:       EntityID("agent", "alice"),
		DstID:       EntityID("task", "task-1"),
		Type:        "sent",
		ValidFrom:   "2026-04-20T10:00:00Z",
		EvidenceMsg: "msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted || dupID != edgeID {
		t.Fatalf("expected duplicate edge detection, got id=%d inserted=%v", dupID, inserted)
	}
}
