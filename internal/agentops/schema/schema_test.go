package schema

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAppliesSchemaAndPragmas(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agentops.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, table := range []string{
		"messages",
		"flows",
		"flow_participants",
		"traces",
		"trace_agents",
		"trace_span_types",
		"tasks",
		"topic_stats",
		"topic_agents",
		"replay_sessions",
		"health_snapshots",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("expected table %q to exist", table)
		}
	}

	if got := pragmaValue(t, db, "foreign_keys"); got != 1 {
		t.Fatalf("foreign_keys pragma = %d, want 1", got)
	}
	if got := pragmaValue(t, db, "busy_timeout"); got != 5000 {
		t.Fatalf("busy_timeout pragma = %d, want 5000", got)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	db, err := Open("")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := Apply(db); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&got)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("tableExists(%q): %v", name, err)
	}
	return got == name
}

func pragmaValue(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var got int
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return got
}
