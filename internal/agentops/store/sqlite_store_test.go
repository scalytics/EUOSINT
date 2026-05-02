package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"strings"
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

	want := sampleSnapshot(start, end)

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

func TestQueryMethods(t *testing.T) {
	start := "2026-04-20T08:00:00Z"
	mid := "2026-04-20T09:00:00Z"
	end := "2026-04-20T10:00:00Z"
	fs := seededQueryStore(t, Snapshot{
		GeneratedAt:  "2026-04-20T10:05:00Z",
		Enabled:      true,
		UIMode:       "AGENTOPS",
		Profile:      "agentops-default",
		GroupName:    "core",
		Topics:       []string{"group.core.requests", "group.core.responses", "group.ops.alerts"},
		FlowCount:    2,
		TraceCount:   2,
		TaskCount:    2,
		MessageCount: 4,
		Health: Health{
			Connected:             true,
			EffectiveTopics:       []string{"group.core.requests", "group.core.responses", "group.ops.alerts"},
			GroupID:               "group-a",
			AcceptedCount:         8,
			RejectedCount:         1,
			RejectedByReason:      map[string]int{"invalid_envelope": 1},
			LastPollAt:            end,
			ReplayStatus:          "completed",
			ReplayLastFinishedAt:  end,
			ReplayLastRecordCount: 25,
			TopicHealth: []TopicHealth{
				{Topic: "group.core.requests", MessagesPerHour: 120, ActiveAgents: 2, LastMessageAt: end},
				{Topic: "group.core.responses", MessagesPerHour: 30, ActiveAgents: 1, LastMessageAt: mid},
				{Topic: "group.ops.alerts", MessagesPerHour: 2, ActiveAgents: 1, LastMessageAt: start},
			},
		},
		ReplaySessions: []ReplaySession{
			{ID: "replay-2", GroupID: "group-replay", Status: "running", StartedAt: end, MessageCount: 5, Topics: []string{"group.ops.alerts"}},
			{ID: "replay-1", GroupID: "group-replay", Status: "completed", StartedAt: start, FinishedAt: mid, MessageCount: 25, Topics: []string{"group.core.requests"}},
		},
		Flows: []Flow{
			{ID: "corr-2", Topics: []string{"group.ops.alerts"}, Senders: []string{"worker-c"}, TraceIDs: []string{"trace-2"}, TaskIDs: []string{"task-2"}, FirstSeen: mid, LastSeen: end, LatestStatus: "running", MessageCount: 2, LatestPreview: "Alert triage"},
			{ID: "corr-1", Topics: []string{"group.core.requests", "group.core.responses"}, Senders: []string{"worker-a", "worker-b"}, TraceIDs: []string{"trace-1"}, TaskIDs: []string{"task-1"}, FirstSeen: start, LastSeen: mid, LatestStatus: "completed", MessageCount: 2, LatestPreview: "Investigate outage"},
		},
		Traces: []Trace{
			{ID: "trace-2", SpanCount: 1, Agents: []string{"worker-c"}, SpanTypes: []string{"TOOL"}, LatestTitle: "alert trace", StartedAt: mid, EndedAt: end, DurationMs: 500},
			{ID: "trace-1", SpanCount: 2, Agents: []string{"worker-a", "worker-b"}, SpanTypes: []string{"TOOL", "LLM"}, LatestTitle: "trace title", StartedAt: start, EndedAt: mid, DurationMs: 1000},
		},
		Tasks: []Task{
			{ID: "task-2", RequesterID: "worker-c", ResponderID: "worker-d", Status: "running", Description: "Alert triage", FirstSeen: mid, LastSeen: end},
			{ID: "task-1", ParentTaskID: "root-1", DelegationDepth: 1, RequesterID: "worker-a", ResponderID: "worker-b", OriginalRequesterID: "orch-1", Status: "completed", Description: "Investigate outage", LastSummary: "done", FirstSeen: start, LastSeen: mid},
		},
		Messages: []Message{
			{ID: "msg-4", Topic: "group.ops.alerts", TopicFamily: "alerts", Partition: 0, Offset: 4, Timestamp: end, EnvelopeType: "event", SenderID: "worker-c", CorrelationID: "corr-2", TraceID: "trace-2", TaskID: "task-2", Status: "running", Preview: "alert raised", Content: `{"alert":"high"}`},
			{ID: "msg-3", Topic: "group.ops.alerts", TopicFamily: "alerts", Partition: 0, Offset: 3, Timestamp: mid, EnvelopeType: "event", SenderID: "worker-c", CorrelationID: "corr-2", TraceID: "trace-2", TaskID: "task-2", Status: "running", Preview: "triage start", Content: `{"alert":"start"}`},
			{ID: "msg-2", Topic: "group.core.responses", TopicFamily: "responses", Partition: 0, Offset: 2, Timestamp: mid, EnvelopeType: "response", SenderID: "worker-b", CorrelationID: "corr-1", TraceID: "trace-1", TaskID: "task-1", Status: "completed", Preview: "done", Content: `{"result":"ok"}`},
			{ID: "msg-1", Topic: "group.core.requests", TopicFamily: "requests", Partition: 0, Offset: 1, Timestamp: start, EnvelopeType: "request", SenderID: "worker-a", CorrelationID: "corr-1", TraceID: "trace-1", TaskID: "task-1", ParentTaskID: "root-1", Preview: "Investigate outage", Content: `{"task":"inspect"}`},
		},
	})
	ctx := context.Background()

	filteredFlows, filteredCursor, err := fs.ListFlows(ctx, FlowFilter{Topic: "group.ops.alerts"}, Pagination{Limit: 1})
	if err != nil {
		t.Fatalf("ListFlows() error = %v", err)
	}
	if len(filteredFlows) != 1 || filteredFlows[0].ID != "corr-2" || filteredCursor != "" {
		t.Fatalf("unexpected filtered flow page %#v cursor=%q", filteredFlows, filteredCursor)
	}

	flows, cursor, err := fs.ListFlows(ctx, FlowFilter{}, Pagination{Limit: 1})
	if err != nil {
		t.Fatalf("ListFlows(unfiltered) error = %v", err)
	}
	if len(flows) != 1 || flows[0].ID != "corr-2" || cursor == "" {
		t.Fatalf("unexpected first flow page %#v cursor=%q", flows, cursor)
	}
	if _, err := base64.RawURLEncoding.DecodeString(string(cursor)); err != nil {
		t.Fatalf("cursor is not base64: %v", err)
	}

	nextFlows, nextCursor, err := fs.ListFlows(ctx, FlowFilter{}, Pagination{Limit: 1, After: cursor})
	if err != nil {
		t.Fatalf("ListFlows(after) error = %v", err)
	}
	if len(nextFlows) != 1 || nextFlows[0].ID != "corr-1" || nextCursor != "" {
		t.Fatalf("unexpected second flow page %#v cursor=%q", nextFlows, nextCursor)
	}

	flow, err := fs.GetFlow(ctx, "corr-1")
	if err != nil {
		t.Fatalf("GetFlow() error = %v", err)
	}
	if flow.TopicCount != 2 || flow.SenderCount != 2 || flow.LatestStatus != "completed" {
		t.Fatalf("unexpected flow %#v", flow)
	}

	messages, msgCursor, err := fs.ListMessagesForFlow(ctx, "corr-2", Pagination{Limit: 1})
	if err != nil {
		t.Fatalf("ListMessagesForFlow() error = %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "msg-4" || msgCursor == "" {
		t.Fatalf("unexpected message page %#v cursor=%q", messages, msgCursor)
	}
	moreMessages, finalMsgCursor, err := fs.ListMessagesForFlow(ctx, "corr-2", Pagination{Limit: 1, After: msgCursor})
	if err != nil {
		t.Fatalf("ListMessagesForFlow(after) error = %v", err)
	}
	if len(moreMessages) != 1 || moreMessages[0].ID != "msg-3" || finalMsgCursor != "" {
		t.Fatalf("unexpected next message page %#v cursor=%q", moreMessages, finalMsgCursor)
	}

	traces, err := fs.ListTracesForFlow(ctx, "corr-1")
	if err != nil {
		t.Fatalf("ListTracesForFlow() error = %v", err)
	}
	if len(traces) != 1 || traces[0].ID != "trace-1" || len(traces[0].Agents) != 2 {
		t.Fatalf("unexpected traces %#v", traces)
	}

	tasks, err := fs.ListTasksForFlow(ctx, "corr-2")
	if err != nil {
		t.Fatalf("ListTasksForFlow() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-2" || tasks[0].Status != "running" {
		t.Fatalf("unexpected tasks %#v", tasks)
	}

	topics, err := fs.TopicHealth(ctx)
	if err != nil {
		t.Fatalf("TopicHealth() error = %v", err)
	}
	if len(topics) != 3 || topics[0].MessagesPerHour <= 0 || strings.TrimSpace(topics[0].MessageDensity) == "" {
		t.Fatalf("unexpected topic health %#v", topics)
	}

	health, err := fs.LatestHealth(ctx)
	if err != nil {
		t.Fatalf("LatestHealth() error = %v", err)
	}
	if health.AcceptedCount != 8 || len(health.TopicHealth) != 3 || health.TopicHealth[0].MessagesPerHour <= 0 {
		t.Fatalf("unexpected latest health %#v", health)
	}

	replays, err := fs.RecentReplays(ctx, 1)
	if err != nil {
		t.Fatalf("RecentReplays() error = %v", err)
	}
	if len(replays) != 1 || replays[0].ID != "replay-2" {
		t.Fatalf("unexpected replays %#v", replays)
	}
}

func TestQueryCursorValidation(t *testing.T) {
	fs := seededQueryStore(t, sampleSnapshot("2026-04-20T08:00:00Z", "2026-04-20T10:00:00Z"))
	if _, _, err := fs.ListFlows(context.Background(), FlowFilter{}, Pagination{After: Cursor("bad!")}); err == nil {
		t.Fatal("expected bad flow cursor to fail")
	}
	if _, _, err := fs.ListMessagesForFlow(context.Background(), "corr-1", Pagination{After: Cursor("bad!")}); err == nil {
		t.Fatal("expected bad message cursor to fail")
	}
}

func TestSqliteStoreConcurrentSnapshotAndUpdate(t *testing.T) {
	fs := seededQueryStore(t, sampleSnapshot("2026-04-20T08:00:00Z", "2026-04-20T10:00:00Z"))

	const workers = 4
	const iterations = 25

	var wg sync.WaitGroup
	errCh := make(chan error, workers*2)
	start := make(chan struct{})

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			<-start
			for iter := 0; iter < iterations; iter++ {
				if err := fs.Update(func(doc *Snapshot) {
					doc.Health.AcceptedCount++
					doc.FlowCount = len(doc.Flows)
					doc.TraceCount = len(doc.Traces)
					doc.TaskCount = len(doc.Tasks)
					doc.MessageCount = len(doc.Messages)
					doc.Health.RejectedByReason[fmt.Sprintf("writer-%d", workerID)] = iter
					doc.Messages[0].Preview = fmt.Sprintf("writer-%d-%d", workerID, iter)
				}); err != nil {
					errCh <- fmt.Errorf("writer %d iteration %d: %w", workerID, iter, err)
					return
				}
			}
		}(worker)

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for iter := 0; iter < iterations; iter++ {
				doc := fs.Snapshot()
				if doc.Health.RejectedByReason == nil {
					errCh <- fmt.Errorf("snapshot missing rejected-by-reason map")
					return
				}
				if doc.FlowCount != len(doc.Flows) {
					errCh <- fmt.Errorf("flow count mismatch: %d vs %d", doc.FlowCount, len(doc.Flows))
					return
				}
				if doc.TraceCount != len(doc.Traces) {
					errCh <- fmt.Errorf("trace count mismatch: %d vs %d", doc.TraceCount, len(doc.Traces))
					return
				}
				if doc.TaskCount != len(doc.Tasks) {
					errCh <- fmt.Errorf("task count mismatch: %d vs %d", doc.TaskCount, len(doc.Tasks))
					return
				}
				if doc.MessageCount != len(doc.Messages) {
					errCh <- fmt.Errorf("message count mismatch: %d vs %d", doc.MessageCount, len(doc.Messages))
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	doc := fs.Snapshot()
	expectedAccepted := sampleSnapshot("2026-04-20T08:00:00Z", "2026-04-20T10:00:00Z").Health.AcceptedCount + workers*iterations
	if doc.Health.AcceptedCount != expectedAccepted {
		t.Fatalf("accepted count = %d, want %d", doc.Health.AcceptedCount, expectedAccepted)
	}
}

func sampleSnapshot(start, end string) Snapshot {
	return Snapshot{
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
}

func seededQueryStore(t *testing.T, snapshot Snapshot) *SqliteStore {
	t.Helper()
	fs, err := NewSqliteStore(filepath.Join(t.TempDir(), "agentops-state.db"), Snapshot{})
	if err != nil {
		t.Fatalf("NewSqliteStore() error = %v", err)
	}
	if err := fs.Update(func(doc *Snapshot) { *doc = snapshot }); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	return fs
}
