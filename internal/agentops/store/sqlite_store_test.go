package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSqliteStorePersistsAndReloadsSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentops-state.db")

	fs, err := NewSqliteStore(path, Snapshot{
		Enabled:   true,
		UIMode:    "AGENTOPS",
		Profile:   "agentops-default",
		GroupName: "core",
		Topics:    []string{"group.core.requests"},
		Health:    Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}

	err = fs.Update(func(doc *Snapshot) {
		doc.GeneratedAt = "2026-04-20T10:00:00Z"
		doc.Flows = []Flow{{ID: "corr-1", FirstSeen: "2026-04-20T10:00:00Z", LastSeen: "2026-04-20T10:00:01Z", MessageCount: 1}}
		doc.Messages = []Message{{ID: "msg-1", Topic: "group.core.requests", TopicFamily: "requests", Timestamp: "2026-04-20T10:00:00Z"}}
		doc.FlowCount = 1
		doc.MessageCount = 1
		doc.Health.Connected = true
		doc.Health.GroupID = "group-a"
		doc.Health.EffectiveTopics = []string{"group.core.requests"}
		doc.Health.RejectedByReason = map[string]int{}
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	reopened, err := NewSqliteStore(path, Snapshot{})
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	doc := reopened.Snapshot()
	if doc.FlowCount != 1 || len(doc.Messages) != 1 || doc.Flows[0].ID != "corr-1" {
		t.Fatalf("unexpected persisted snapshot %#v", doc)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected sqlite db to exist: %v", err)
	}
}

func TestSqliteStoreRoundTripFullSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentops-state.db")
	start := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	end := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)

	fs, err := NewSqliteStore(path, Snapshot{})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}

	want := Snapshot{
		GeneratedAt:  "2026-04-20T10:00:00Z",
		Enabled:      true,
		UIMode:       "AGENTOPS",
		Profile:      "agentops-default",
		GroupName:    "core",
		Topics:       []string{"group.core.requests", "group.core.responses"},
		FlowCount:    1,
		TraceCount:   1,
		TaskCount:    1,
		MessageCount: 2,
		Health: Health{
			Connected:             true,
			EffectiveTopics:       []string{"group.core.requests", "group.core.responses"},
			GroupID:               "group-a",
			AcceptedCount:         4,
			RejectedCount:         1,
			MirroredCount:         1,
			RejectedByReason:      map[string]int{"invalid_envelope": 1},
			LastReject:            "invalid_envelope",
			LastPollAt:            time.Now().UTC().Format(time.RFC3339),
			ReplayStatus:          "completed",
			ReplayActive:          0,
			ReplayLastFinishedAt:  time.Now().UTC().Format(time.RFC3339),
			ReplayLastRecordCount: 25,
			TopicHealth: []TopicHealth{
				{Topic: "group.core.requests", MessagesPerHour: 120, MessageDensity: "high", ActiveAgents: 2, IsStale: false, LastMessageAt: end},
				{Topic: "group.core.responses", MessagesPerHour: 2, MessageDensity: "low", ActiveAgents: 1, IsStale: true, LastMessageAt: start},
			},
		},
		ReplaySessions: []ReplaySession{
			{ID: "replay-1", GroupID: "group-replay", Status: "completed", StartedAt: start, FinishedAt: end, MessageCount: 25, Topics: []string{"group.core.requests"}},
		},
		Flows: []Flow{
			{ID: "corr-1", TopicCount: 2, SenderCount: 2, Topics: []string{"group.core.requests", "group.core.responses"}, Senders: []string{"worker-a", "worker-b"}, TraceIDs: []string{"trace-1"}, TaskIDs: []string{"task-1"}, FirstSeen: start, LastSeen: end, LatestStatus: "completed", MessageCount: 2, LatestPreview: "Investigate outage"},
		},
		Traces: []Trace{
			{ID: "trace-1", SpanCount: 2, Agents: []string{"worker-a", "worker-b"}, SpanTypes: []string{"TOOL", "LLM"}, LatestTitle: "trace title", StartedAt: start, EndedAt: end, DurationMs: 1000},
		},
		Tasks: []Task{
			{ID: "task-1", ParentTaskID: "root-1", DelegationDepth: 1, RequesterID: "worker-a", ResponderID: "worker-b", OriginalRequesterID: "orch-1", Status: "completed", Description: "Investigate outage", LastSummary: "done", FirstSeen: start, LastSeen: end},
		},
		Messages: []Message{
			{ID: "msg-2", Topic: "group.core.responses", TopicFamily: "responses", Partition: 0, Offset: 2, Timestamp: end, EnvelopeType: "response", SenderID: "worker-b", CorrelationID: "corr-1", TraceID: "trace-1", TaskID: "task-1", Status: "completed", Preview: "done", Content: `{"result":"ok"}`},
			{ID: "msg-1", Topic: "group.core.requests", TopicFamily: "requests", Partition: 0, Offset: 1, Timestamp: start, EnvelopeType: "request", SenderID: "worker-a", CorrelationID: "corr-1", TaskID: "task-1", ParentTaskID: "root-1", Preview: "Investigate outage", Content: `{"task":"inspect"}`, LFS: &LFSPointer{Bucket: "ops", Key: "req/1", Size: 88, SHA256: "abc", ContentType: "application/json", CreatedAt: start, ProxyID: "lfs-1", Path: "s3://ops/req/1"}},
		},
	}

	if err := fs.Update(func(doc *Snapshot) { *doc = want }); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got := fs.Snapshot()
	if got.Health.GroupID != "group-a" || len(got.Traces) != 1 || len(got.ReplaySessions) != 1 {
		t.Fatalf("unexpected round-tripped snapshot %#v", got)
	}
	if len(got.Messages) != 2 || got.Messages[1].LFS == nil || got.Messages[1].LFS.Path != "s3://ops/req/1" {
		t.Fatalf("unexpected message round-trip %#v", got.Messages)
	}
}

func TestSqliteStoreHelpers(t *testing.T) {
	if got := sqlitePath(""); got != "" {
		t.Fatalf("sqlitePath empty = %q", got)
	}
	if got := sqlitePath("state.json"); got != "state.db" {
		t.Fatalf("sqlitePath json = %q", got)
	}
	if got := sqlitePath("state.db"); got != "state.db" {
		t.Fatalf("sqlitePath db = %q", got)
	}
	if got := boolToInt(true); got != 1 {
		t.Fatalf("boolToInt(true) = %d", got)
	}
	if got := boolToInt(false); got != 0 {
		t.Fatalf("boolToInt(false) = %d", got)
	}
	if got := strconvI(42); got != "42" {
		t.Fatalf("strconvI = %q", got)
	}
	if !storeTopicIsStale("") {
		t.Fatal("empty timestamp should be stale")
	}
	if !storeTopicIsStale("bad") {
		t.Fatal("bad timestamp should be stale")
	}
	if storeTopicIsStale(time.Now().UTC().Format(time.RFC3339)) {
		t.Fatal("fresh timestamp should not be stale")
	}
	if got := storeDensityBucket(0); got != "low" {
		t.Fatalf("storeDensityBucket(0) = %q", got)
	}
	if got := storeDensityBucket(10); got != "medium" {
		t.Fatalf("storeDensityBucket(10) = %q", got)
	}
	if got := storeDensityBucket(60); got != "high" {
		t.Fatalf("storeDensityBucket(60) = %q", got)
	}
}

func TestReadDocumentHandlesClosedDB(t *testing.T) {
	fs, err := NewSqliteStore("", Snapshot{})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}
	if err := fs.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if _, err := fs.readDocument(); err == nil {
		t.Fatal("expected readDocument to fail on closed DB")
	}
	if _, err := fs.isEmpty(); err == nil {
		t.Fatal("expected isEmpty to fail on closed DB")
	}
}

func TestNewSqliteStoreFailsWhenCreateDirFails(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewSqliteStore(filepath.Join(blocker, "child.db"), Snapshot{})
	if err == nil {
		t.Fatal("expected create dir failure")
	}
}

func TestStoreReloadPreservesInitialShellWhenDatabaseIsEmpty(t *testing.T) {
	fs, err := NewSqliteStore("", Snapshot{
		Enabled:   true,
		UIMode:    "AGENTOPS",
		Profile:   "agentops-default",
		GroupName: "core",
		Topics:    []string{"group.core.requests"},
		Health:    Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}
	doc := fs.Snapshot()
	if !doc.Enabled || doc.UIMode != "AGENTOPS" || doc.GroupName != "core" {
		t.Fatalf("unexpected preserved shell %#v", doc)
	}
}

func TestInsertMessagesErrorPropagation(t *testing.T) {
	fs, err := NewSqliteStore("", Snapshot{})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}
	tx, err := fs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("DROP TABLE messages"); err != nil {
		t.Fatal(err)
	}
	if err := insertMessages(tx, []Message{{ID: "m1", Topic: "t", TopicFamily: "requests", Timestamp: time.Now().UTC().Format(time.RFC3339)}}); err == nil {
		t.Fatal("expected insertMessages to fail when table is missing")
	}
	_ = tx.Rollback()
}

func TestReadHealthWithNoRows(t *testing.T) {
	fs, err := NewSqliteStore("", Snapshot{})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}
	got, err := readHealth(fs.db)
	if err != nil {
		t.Fatalf("readHealth error = %v", err)
	}
	if got.RejectedByReason == nil {
		t.Fatal("expected rejected reason map initialization")
	}
}

func TestReadMessagesErrorPropagation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:bad-read-messages?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := readMessages(db); err == nil {
		t.Fatal("expected readMessages failure without schema")
	}
}
