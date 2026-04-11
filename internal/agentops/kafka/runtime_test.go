package kafka

import (
	"context"
	"testing"
	"time"

	agentcfg "github.com/scalytics/euosint/internal/agentops/config"
	"github.com/scalytics/euosint/internal/agentops/store"
	collectorcfg "github.com/scalytics/euosint/internal/collector/config"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestHandleRecordRequestBuildsFlowAndTask(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	rec := &kgo.Record{
		Topic:     "group.core.requests",
		Partition: 1,
		Offset:    42,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Value:     []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","timestamp":"2026-04-10T12:00:00Z","payload":{"task_id":"task-1","description":"Investigate outage","content":"check turbine","requester_id":"worker-a","parent_task_id":"root-1","delegation_depth":1,"original_requester_id":"orch-1"}}`),
	}
	if reason, ok := svc.handleRecord(rec); !ok {
		t.Fatalf("handleRecord rejected request: %s", reason)
	}
	if len(svc.internal.flows) != 1 || len(svc.internal.tasks) != 1 || len(svc.internal.msgs) != 1 {
		t.Fatalf("unexpected state sizes flows=%d tasks=%d msgs=%d", len(svc.internal.flows), len(svc.internal.tasks), len(svc.internal.msgs))
	}
	flow := svc.internal.flows["corr-1"]
	if flow == nil || flow.MessageCount != 1 || len(flow.TaskIDs) != 1 || flow.TaskIDs[0] != "task-1" {
		t.Fatalf("unexpected flow state: %#v", flow)
	}
	task := svc.internal.tasks["task-1"]
	if task == nil || task.ParentTaskID != "root-1" || task.RequesterID != "worker-a" {
		t.Fatalf("unexpected task state: %#v", task)
	}
}

func TestNewReplayGroupID(t *testing.T) {
	got := newReplayGroupID("replay-core", time.Date(2026, 4, 10, 12, 30, 0, 0, time.UTC))
	if got != "replay-core-20260410t123000" {
		t.Fatalf("unexpected replay group id %q", got)
	}
}

func TestStartReplayWithoutRuntime(t *testing.T) {
	currentMu.Lock()
	currentService = nil
	currentMu.Unlock()
	if _, err := StartReplay(context.Background()); err == nil {
		t.Fatal("expected error when runtime is not active")
	}
}

func TestHandleRecordLFSPointerIsMetadataOnly(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	rec := &kgo.Record{
		Topic:     "group.core.responses",
		Partition: 0,
		Offset:    7,
		Timestamp: time.Date(2026, 4, 10, 12, 5, 0, 0, time.UTC),
		Value:     []byte(`{"kfs_lfs":1,"bucket":"ops","key":"core/responses/7","size":88,"sha256":"abc","content_type":"application/json","created_at":"2026-04-10T12:05:00Z","proxy_id":"lfs-1"}`),
	}
	if reason, ok := svc.handleRecord(rec); !ok {
		t.Fatalf("handleRecord rejected LFS pointer: %s", reason)
	}
	msg := svc.internal.msgs["group.core.responses:0:7"]
	if msg.LFS == nil || msg.Content != "" || msg.CorrelationID != "" {
		t.Fatalf("expected pointer-only LFS message, got %#v", msg)
	}
	if msg.LFS.Path != "s3://ops/core/responses/7" {
		t.Fatalf("unexpected LFS path %q", msg.LFS.Path)
	}
}

func TestHandleRecordResponseAndTraceUpdateTaskAndTraceState(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}

	request := &kgo.Record{
		Topic:     "group.core.requests",
		Partition: 0,
		Offset:    1,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Value:     []byte(`{"type":"request","correlation_id":"corr-2","sender_id":"worker-a","timestamp":"2026-04-10T12:00:00Z","payload":{"task_id":"task-2","description":"Check turbine","requester_id":"worker-a"}}`),
	}
	response := &kgo.Record{
		Topic:     "group.core.responses",
		Partition: 0,
		Offset:    2,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 1, 0, time.UTC),
		Value:     []byte(`{"type":"response","correlation_id":"corr-2","sender_id":"worker-b","timestamp":"2026-04-10T12:00:01Z","payload":{"task_id":"task-2","status":"completed","responder_id":"worker-b","content":"done"}}`),
	}
	trace := &kgo.Record{
		Topic:     "group.core.traces",
		Partition: 0,
		Offset:    3,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 2, 0, time.UTC),
		Value:     []byte(`{"type":"trace","correlation_id":"corr-2","sender_id":"worker-b","timestamp":"2026-04-10T12:00:02Z","payload":{"trace_id":"trace-2","span_type":"TOOL","title":"Inspect node","content":"trace content","started_at":"2026-04-10T12:00:01Z","ended_at":"2026-04-10T12:00:02Z","duration_ms":1000}}`),
	}

	for _, rec := range []*kgo.Record{request, response, trace} {
		if reason, ok := svc.handleRecord(rec); !ok {
			t.Fatalf("handleRecord rejected record: %s", reason)
		}
	}
	if err := svc.persist(); err != nil {
		t.Fatal(err)
	}

	task := svc.internal.tasks["task-2"]
	if task == nil || task.Status != "completed" || task.ResponderID != "worker-b" {
		t.Fatalf("unexpected task state: %#v", task)
	}
	traceState := svc.internal.traces["trace-2"]
	if traceState == nil || traceState.SpanCount != 1 || traceState.DurationMs != 1000 {
		t.Fatalf("unexpected trace state: %#v", traceState)
	}
	flow := svc.internal.flows["corr-2"]
	if flow == nil || flow.MessageCount != 3 || len(flow.TraceIDs) != 1 {
		t.Fatalf("unexpected flow state: %#v", flow)
	}
	doc := svc.file.Snapshot()
	if doc.Health.AcceptedCount != 3 || len(doc.Health.TopicHealth) != 3 {
		t.Fatalf("unexpected persisted health: %#v", doc.Health)
	}
}

func TestHandleRecordRejectsInvalidEnvelope(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}

	rec := &kgo.Record{
		Topic:     "group.core.requests",
		Partition: 0,
		Offset:    9,
		Timestamp: time.Date(2026, 4, 10, 12, 5, 0, 0, time.UTC),
		Value:     []byte(`{"type":"request","payload":`),
	}
	reason, ok := svc.handleRecord(rec)
	if ok || reason != "invalid_envelope" {
		t.Fatalf("expected invalid_envelope rejection, got ok=%v reason=%q", ok, reason)
	}
}

func TestPersistCapsReplayMessagesByPolicy(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{AgentOpsGroupName: "core", UIMode: "AGENTOPS", Profile: "agentops-default"},
		policy: agentcfg.Policy{
			Version:   1,
			GroupName: "core",
			Grouping: agentcfg.Grouping{
				FlowKey:          "correlation_id",
				ReplayMaxRecords: 2,
			},
		},
		file: mustFileStore(t),
		internal: state{
			flows: map[string]*store.Flow{
				"corr": {ID: "corr", FirstSeen: "2026-04-10T12:00:00Z", LastSeen: "2026-04-10T12:00:02Z", MessageCount: 3},
			},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs: map[string]store.Message{
				"a": {ID: "a", Timestamp: "2026-04-10T12:00:00Z"},
				"b": {ID: "b", Timestamp: "2026-04-10T12:00:01Z"},
				"c": {ID: "c", Timestamp: "2026-04-10T12:00:02Z"},
			},
			topic: map[string]*topicStat{
				"group.core.requests": {Count: 3, Agents: map[string]struct{}{"worker-a": {}}, LastMessageAt: "2026-04-10T12:00:02Z"},
			},
		},
		topics: []string{"group.core.requests"},
	}

	if err := svc.persist(); err != nil {
		t.Fatal(err)
	}
	doc := svc.file.Snapshot()
	if len(doc.Messages) != 2 {
		t.Fatalf("expected capped message list, got %d", len(doc.Messages))
	}
	if doc.Messages[0].ID != "c" || doc.Messages[1].ID != "b" {
		t.Fatalf("unexpected persisted message order: %#v", doc.Messages)
	}
}

func mustFileStore(t *testing.T) *store.FileStore {
	t.Helper()
	fs, err := store.NewFileStore("", store.Document{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}
