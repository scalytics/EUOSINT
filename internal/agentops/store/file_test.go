package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileStoreImportsLegacyJSONSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentops-state.json")
	raw := `{
	  "generated_at":"2026-04-20T10:00:00Z",
	  "enabled":true,
	  "ui_mode":"AGENTOPS",
	  "profile":"agentops-default",
	  "group_name":"core",
	  "topics":["group.core.requests"],
	  "flow_count":1,
	  "trace_count":0,
	  "task_count":0,
	  "message_count":1,
	  "health":{"connected":true,"effective_topics":["group.core.requests"],"group_id":"group-a","accepted_count":1,"rejected_count":0,"mirrored_count":0,"mirror_failed_count":0,"rejected_by_reason":{},"topic_health":[]},
	  "replay_sessions":[],
	  "flows":[{"id":"corr-1","first_seen":"2026-04-20T10:00:00Z","last_seen":"2026-04-20T10:00:00Z","message_count":1,"topics":["group.core.requests"],"senders":["worker-a"],"trace_ids":[],"task_ids":[]}],
	  "traces":[],
	  "tasks":[],
	  "messages":[{"id":"group.core.requests:0:1","topic":"group.core.requests","topic_family":"requests","partition":0,"offset":1,"timestamp":"2026-04-20T10:00:00Z","sender_id":"worker-a","correlation_id":"corr-1"}]
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFileStore(path, Document{})
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	doc := fs.Snapshot()
	if doc.FlowCount != 1 || len(doc.Messages) != 1 || doc.Flows[0].ID != "corr-1" {
		t.Fatalf("unexpected imported snapshot %#v", doc)
	}
	if _, err := os.Stat(filepath.Join(dir, "agentops-state.db")); err != nil {
		t.Fatalf("expected sqlite db to exist: %v", err)
	}
}

func TestUpdatePersistsSQLiteAndJSONShadow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentops-state.json")

	fs, err := NewFileStore(path, Document{
		Health: Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	err = fs.Update(func(doc *Document) {
		doc.Enabled = true
		doc.UIMode = "AGENTOPS"
		doc.Profile = "agentops-default"
		doc.GroupName = "core"
		doc.Topics = []string{"group.core.requests"}
		doc.Flows = []Flow{{ID: "corr-1", FirstSeen: "2026-04-20T10:00:00Z", LastSeen: "2026-04-20T10:00:00Z", MessageCount: 1}}
		doc.FlowCount = 1
		doc.Health.Connected = true
		doc.Health.GroupID = "group-a"
		doc.Health.RejectedByReason = map[string]int{}
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected JSON shadow export: %v", err)
	}
	reopened, err := NewFileStore(path, Document{
		Enabled:   true,
		UIMode:    "AGENTOPS",
		Profile:   "agentops-default",
		GroupName: "core",
		Topics:    []string{"group.core.requests"},
	})
	if err != nil {
		t.Fatalf("reopen store error = %v", err)
	}
	doc := reopened.Snapshot()
	if len(doc.Flows) != 1 || doc.Flows[0].ID != "corr-1" {
		t.Fatalf("unexpected reopened snapshot %#v", doc)
	}
}

func TestNewFileStoreReturnsLegacyJSONErrorWhenDBEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentops-state.json")
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(path, Document{}); err == nil {
		t.Fatal("expected invalid legacy json to fail when bootstrapping empty db")
	}
}
