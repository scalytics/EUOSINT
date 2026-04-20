package schema

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	agentopschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
)

func TestApplyCreatesGraphTables(t *testing.T) {
	db, err := agentopschema.Open(filepath.Join(t.TempDir(), "agentops.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	for _, table := range []string{"entities", "edges", "provenance"} {
		if !graphTableExists(t, db, table) {
			t.Fatalf("expected table %q", table)
		}
	}
}

func TestApplySupportsQueryPlanIndexes(t *testing.T) {
	db, err := agentopschema.Open(filepath.Join(t.TempDir(), "agentops.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if _, err := db.Exec(`INSERT INTO entities (id, type, canonical_id, first_seen, last_seen) VALUES ('agent:alice', 'agent', 'alice', '2026-04-20T10:00:00Z', '2026-04-20T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO entities (id, type, canonical_id, first_seen, last_seen) VALUES ('task:t-1', 'task', 't-1', '2026-04-20T10:00:00Z', '2026-04-20T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages (record_id, topic, topic_family, partition, offset, timestamp, outcome) VALUES ('msg-1', 'group.core.requests', 'requests', 0, 1, '2026-04-20T10:00:00Z', 'accepted')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO edges (src_id, dst_id, type, valid_from, evidence_msg) VALUES ('agent:alice', 'task:t-1', 'sent', '2026-04-20T10:00:00Z', 'msg-1')`); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT * FROM edges WHERE src_id = ? AND type = ? ORDER BY valid_from DESC`, "agent:alice", "sent")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	usedIndex := false
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "idx_edges_src_type") {
			usedIndex = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usedIndex {
		t.Fatal("expected EXPLAIN QUERY PLAN to use idx_edges_src_type")
	}
}

func graphTableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&got)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("tableExists(%q): %v", name, err)
	}
	return got == name
}
