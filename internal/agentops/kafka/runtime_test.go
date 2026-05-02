package kafka

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcfg "github.com/scalytics/kafSIEM/internal/agentops/config"
	"github.com/scalytics/kafSIEM/internal/agentops/store"
	collectorcfg "github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestHandleRecordRequestBuildsFlowAndTask(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustSqliteStore(t),
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
	doc := svc.file.Snapshot()
	if len(doc.Flows) != 1 || len(doc.Tasks) != 1 || len(doc.Messages) != 1 {
		t.Fatalf("unexpected persisted state %#v", doc)
	}
	flow := doc.Flows[0]
	if flow.MessageCount != 1 || len(flow.TaskIDs) != 1 || flow.TaskIDs[0] != "task-1" {
		t.Fatalf("unexpected flow state: %#v", flow)
	}
	task := doc.Tasks[0]
	if task.ParentTaskID != "root-1" || task.RequesterID != "worker-a" {
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
		file:   mustSqliteStore(t),
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
	msg := svc.file.Snapshot().Messages[0]
	if msg.LFS == nil || msg.Content != "" || msg.CorrelationID != "" {
		t.Fatalf("expected pointer-only LFS message, got %#v", msg)
	}
	if msg.LFS.Path != "s3://ops/core/responses/7" {
		t.Fatalf("unexpected LFS path %q", msg.LFS.Path)
	}
}

func TestHandleRecordStatusAuditUnknownAndDuplicateBranches(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustSqliteStore(t),
	}
	statusRec := &kgo.Record{
		Topic:     "group.core.tasks.status",
		Partition: 0,
		Offset:    1,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Value:     []byte(`{"type":"task_status","correlation_id":"corr-3","sender_id":"worker-a","payload":{"task_id":"task-3","status":"waiting","summary":"queued","responder_id":"worker-b"}}`),
	}
	if reason, ok := svc.handleRecord(statusRec); !ok {
		t.Fatalf("unexpected reject: %s", reason)
	}
	auditRec := &kgo.Record{
		Topic:     "group.core.observe.audit",
		Partition: 0,
		Offset:    2,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 1, 0, time.UTC),
		Value:     []byte(`{"type":"audit","correlation_id":"corr-3","sender_id":"worker-a","payload":{"what":"done"}}`),
	}
	if reason, ok := svc.handleRecord(auditRec); !ok {
		t.Fatalf("unexpected reject: %s", reason)
	}
	if reason, ok := svc.handleRecord(auditRec); !ok || reason != "" {
		t.Fatalf("expected duplicate record to be treated as accepted noop, got ok=%v reason=%q", ok, reason)
	}
	unknown := &kgo.Record{Topic: "group.other.requests", Partition: 0, Offset: 3, Value: []byte(`{}`)}
	if reason, ok := svc.handleRecord(unknown); ok || reason != "unknown_topic" {
		t.Fatalf("expected unknown_topic rejection, got ok=%v reason=%q", ok, reason)
	}
	doc := svc.file.Snapshot()
	if len(doc.Tasks) != 1 || len(doc.Messages) != 2 {
		t.Fatalf("unexpected persisted state %#v", doc)
	}
	task := doc.Tasks[0]
	if task.Status != "waiting" || task.LastSummary != "queued" {
		t.Fatalf("unexpected task state %#v", task)
	}
	msg := doc.Messages[0]
	if msg.Preview == "" {
		t.Fatalf("expected audit preview, got %#v", msg)
	}
}

func TestUpdateTraceAndTaskGuardBranches(t *testing.T) {
	doc := &store.Snapshot{}
	updateTraceMessage(doc, "2026-04-10T12:00:00Z", store.Message{}, nil)
	updateTaskMessage(doc, "2026-04-10T12:00:00Z", store.Message{}, nil)
	if len(doc.Traces) != 0 || len(doc.Tasks) != 0 {
		t.Fatalf("expected guard branches to avoid mutation, got %#v", doc)
	}
}

func TestStartDisabledReturnsNil(t *testing.T) {
	if err := Start(context.Background(), collectorcfg.Config{}); err != nil {
		t.Fatalf("expected disabled AgentOps start to return nil, got %v", err)
	}
}

func TestStartValidatesPolicyAndStore(t *testing.T) {
	cfg := collectorcfg.Config{
		AgentOpsEnabled:     true,
		AgentOpsGroupName:   "core",
		AgentOpsPolicyPath:  filepath.Join(t.TempDir(), "missing.yaml"),
		AgentOpsRejectTopic: "group.core.agentops.rejects",
	}
	if err := Start(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "AGENTOPS_BROKERS") {
		t.Fatalf("expected runtime config error after policy fallback, got %v", err)
	}

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg = collectorcfg.Config{
		AgentOpsEnabled:     true,
		AgentOpsGroupName:   "core",
		AgentOpsGroupID:     "group-a",
		AgentOpsBrokers:     []string{"localhost:9092"},
		AgentOpsRejectTopic: "group.core.agentops.rejects",
		AgentOpsOutputPath:  filepath.Join(blocker, "agentops.db"),
	}
	if err := Start(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "agentops store") {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestStartUsesDefaultClientFactoryAndBootstrapsStoredState(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "agentops-state.db")
	fs, err := store.NewSqliteStore(outputPath, store.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Update(func(doc *store.Snapshot) {
		doc.Flows = []store.Flow{{ID: "corr-1", FirstSeen: "2026-04-10T12:00:00Z", LastSeen: "2026-04-10T12:00:00Z", MessageCount: 1}}
		doc.Messages = []store.Message{{ID: "group.core.requests:0:1", Topic: "group.core.requests", TopicFamily: "requests", Partition: 0, Offset: 1, Timestamp: "2026-04-10T12:00:00Z"}}
		doc.FlowCount = 1
		doc.MessageCount = 1
	}); err != nil {
		t.Fatal(err)
	}

	originalFactory := defaultClientFactory
	t.Cleanup(func() {
		defaultClientFactory = originalFactory
		setCurrentService(nil)
	})

	defaultClientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return &mockAgentOpsClient{
			polls: []kgo.Fetches{nil},
			onPoll: func(int) {
				// Allow Start to exit cleanly after one empty poll.
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defaultClientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return &mockAgentOpsClient{
			polls: []kgo.Fetches{nil},
			onPoll: func(n int) {
				if n >= 1 {
					cancel()
				}
			},
		}, nil
	}
	cfg := collectorcfg.Config{
		AgentOpsEnabled:     true,
		AgentOpsGroupName:   "core",
		AgentOpsGroupID:     "group-a",
		AgentOpsBrokers:     []string{"localhost:9092"},
		AgentOpsRejectTopic: "group.core.agentops.rejects",
		AgentOpsOutputPath:  outputPath,
		UIMode:              "AGENTOPS",
		Profile:             "agentops-default",
	}
	if err := Start(ctx, cfg); err != nil {
		t.Fatalf("expected Start to succeed with injected client: %v", err)
	}
	currentMu.RLock()
	current := currentService
	currentMu.RUnlock()
	if current == nil {
		t.Fatalf("expected stored state bootstrap, got %#v", current)
	}
	doc := current.file.Snapshot()
	if len(doc.Flows) != 1 || len(doc.Messages) != 1 {
		t.Fatalf("expected stored state bootstrap, got %#v", doc)
	}
}

func TestRunReturnsInitialStateStoreError(t *testing.T) {
	fs, err := store.NewSqliteStore("", store.Snapshot{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Close(); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-a",
			AgentOpsClientID:  "client-a",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   fs,
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{}, nil
		},
	}
	if err := svc.run(context.Background()); err == nil {
		t.Fatal("expected initial store update failure")
	}
}

func TestHandleRecordResponseAndTraceUpdateTaskAndTraceState(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustSqliteStore(t),
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
	doc := svc.file.Snapshot()
	task := doc.Tasks[0]
	if task.Status != "completed" || task.ResponderID != "worker-b" {
		t.Fatalf("unexpected task state: %#v", task)
	}
	traceState := doc.Traces[0]
	if traceState.SpanCount != 1 || traceState.DurationMs != 1000 {
		t.Fatalf("unexpected trace state: %#v", traceState)
	}
	flow := doc.Flows[0]
	if flow.MessageCount != 3 || len(flow.TraceIDs) != 1 {
		t.Fatalf("unexpected flow state: %#v", flow)
	}
	if len(doc.Health.TopicHealth) != 3 {
		t.Fatalf("unexpected persisted health: %#v", doc.Health)
	}
}

func TestHandleRecordWritesGraphEntitiesEdgesAndProvenance(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustSqliteStore(t),
	}
	for _, rec := range []*kgo.Record{
		{
			Topic:     "group.core.requests",
			Partition: 0,
			Offset:    1,
			Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
			Value:     []byte(`{"type":"request","correlation_id":"corr-9","sender_id":"worker-a","timestamp":"2026-04-10T12:00:00Z","payload":{"task_id":"task-9","description":"Check turbine","requester_id":"worker-a","parent_task_id":"root-9"}}`),
		},
		{
			Topic:     "group.core.responses",
			Partition: 0,
			Offset:    2,
			Timestamp: time.Date(2026, 4, 10, 12, 0, 1, 0, time.UTC),
			Value:     []byte(`{"type":"response","correlation_id":"corr-9","sender_id":"worker-b","timestamp":"2026-04-10T12:00:01Z","payload":{"task_id":"task-9","status":"completed","responder_id":"worker-b","content":"done"}}`),
		},
		{
			Topic:     "group.core.traces",
			Partition: 0,
			Offset:    3,
			Timestamp: time.Date(2026, 4, 10, 12, 0, 2, 0, time.UTC),
			Value:     []byte(`{"type":"trace","correlation_id":"corr-9","sender_id":"worker-b","timestamp":"2026-04-10T12:00:02Z","payload":{"trace_id":"trace-9","span_type":"TOOL","title":"Inspect node"}}`),
		},
	} {
		if reason, ok := svc.handleRecord(rec); !ok {
			t.Fatalf("handleRecord rejected record: %s", reason)
		}
	}

	err := svc.file.Apply(func(tx *sql.Tx) error {
		var entities, edges, provenance int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entities); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&edges); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM provenance`).Scan(&provenance); err != nil {
			return err
		}
		if entities < 6 {
			t.Fatalf("expected graph entities, got %d", entities)
		}
		if edges < 7 {
			t.Fatalf("expected graph edges, got %d", edges)
		}
		if provenance < 7 {
			t.Fatalf("expected graph provenance rows, got %d", provenance)
		}
		var edgesWithoutProvenance int
		if err := tx.QueryRow(`
			SELECT COUNT(*)
			  FROM edges e
			  LEFT JOIN provenance p
			    ON p.subject_kind = 'edge'
			   AND p.subject_id = CAST(e.id AS TEXT)
			 WHERE p.id IS NULL
		`).Scan(&edgesWithoutProvenance); err != nil {
			return err
		}
		if edgesWithoutProvenance != 0 {
			t.Fatalf("expected every graph edge to have provenance, missing=%d", edgesWithoutProvenance)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHandleRecordRejectsInvalidEnvelope(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
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
		file:   mustSqliteStore(t),
		topics: []string{"group.core.requests"},
	}
	svc.bootstrapFromStore(store.Snapshot{
		Flows: []store.Flow{
			{ID: "corr", FirstSeen: "2026-04-10T12:00:00Z", LastSeen: "2026-04-10T12:00:02Z", MessageCount: 3},
		},
		Messages: []store.Message{
			{ID: "a", Topic: "group.core.requests", TopicFamily: "requests", Timestamp: "2026-04-10T12:00:00Z"},
			{ID: "b", Topic: "group.core.requests", TopicFamily: "requests", Timestamp: "2026-04-10T12:00:01Z"},
			{ID: "c", Topic: "group.core.requests", TopicFamily: "requests", Timestamp: "2026-04-10T12:00:02Z"},
		},
	})

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

func TestHandleRecordAcceptsRawNonJSONContent(t *testing.T) {
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustSqliteStore(t),
	}
	rec := &kgo.Record{
		Topic:     "group.core.responses",
		Partition: 0,
		Offset:    8,
		Timestamp: time.Date(2026, 4, 10, 12, 5, 0, 0, time.UTC),
		Value:     []byte("plain-text agent note"),
	}
	if reason, ok := svc.handleRecord(rec); !ok {
		t.Fatalf("expected raw content acceptance, got %q", reason)
	}
	msg := svc.file.Snapshot().Messages[0]
	if msg.EnvelopeType != "raw" || msg.Content != "plain-text agent note" || msg.CorrelationID == "" {
		t.Fatalf("unexpected raw message %#v", msg)
	}
}

func TestRunCountsMirrorFailureButCommitsRejectedRecord(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:     true,
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsClientID:    "client-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
			KafkaPollTimeoutMS:  1,
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
	}
	mock := &mockAgentOpsClient{
		polls: []kgo.Fetches{
			fetchesWithRecords(&kgo.Record{
				Topic:     "group.core.requests",
				Partition: 0,
				Offset:    2,
				Value:     []byte(`{`),
			}),
			nil,
		},
		produceErr: errors.New("mirror failed"),
		onPoll: func(n int) {
			if n >= 2 {
				cancel()
			}
		},
	}
	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return mock, nil
	}
	if err := svc.run(ctx); err != nil {
		t.Fatalf("expected graceful shutdown, got %v", err)
	}
	doc := svc.file.Snapshot()
	if doc.Health.RejectedCount != 1 || doc.Health.MirroredCount != 0 || doc.Health.MirrorFailedCount != 1 {
		t.Fatalf("unexpected mirror failure counts %#v", doc.Health)
	}
	if doc.Health.LastMirrorError != "mirror failed" {
		t.Fatalf("unexpected last mirror error %q", doc.Health.LastMirrorError)
	}
	if len(mock.committed) != 1 {
		t.Fatalf("expected rejected record commit, got %d", len(mock.committed))
	}
}

func TestPersistComputesTopicHealthMetrics(t *testing.T) {
	originalNow := nowFunc
	nowFunc = func() time.Time {
		return time.Date(2026, 4, 10, 13, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() { nowFunc = originalNow })

	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core", AgentOpsGroupID: "group-a", UIMode: "AGENTOPS", Profile: "agentops-default"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   mustSqliteStore(t),
		topics: []string{"group.core.requests", "group.core.responses"},
	}
	messages := make([]store.Message, 0, 121)
	for i := 0; i < 120; i++ {
		sender := "worker-a"
		if i%2 == 1 {
			sender = "worker-b"
		}
		messages = append(messages, store.Message{
			ID:          fmt.Sprintf("req-%03d", i),
			Topic:       "group.core.requests",
			TopicFamily: "requests",
			SenderID:    sender,
			Timestamp:   fmt.Sprintf("2026-04-10T12:%02d:00Z", i%60),
		})
	}
	messages = append(messages, store.Message{
		ID:          "resp-1",
		Topic:       "group.core.responses",
		TopicFamily: "responses",
		SenderID:    "worker-a",
		Timestamp:   "2026-04-10T12:00:00Z",
	})
	svc.bootstrapFromStore(store.Snapshot{
		Flows:    []store.Flow{{ID: "corr-1", FirstSeen: "2026-04-10T12:00:00Z", LastSeen: "2026-04-10T12:10:00Z"}},
		Messages: messages,
	})
	if err := svc.persist(); err != nil {
		t.Fatal(err)
	}
	doc := svc.file.Snapshot()
	if doc.FlowCount != 1 {
		t.Fatalf("expected flow count to persist, got %d", doc.FlowCount)
	}
	if len(doc.Health.TopicHealth) != 2 {
		t.Fatalf("unexpected topic health %#v", doc.Health.TopicHealth)
	}
	if doc.Health.TopicHealth[0].Topic != "group.core.requests" || doc.Health.TopicHealth[0].ActiveAgents != 2 || doc.Health.TopicHealth[0].MessageDensity != "high" || doc.Health.TopicHealth[0].IsStale {
		t.Fatalf("unexpected fresh topic health %#v", doc.Health.TopicHealth[0])
	}
	if !doc.Health.TopicHealth[1].IsStale || doc.Health.TopicHealth[1].MessageDensity != "low" {
		t.Fatalf("unexpected stale topic health %#v", doc.Health.TopicHealth[1])
	}
}

func TestReplayCancellationMarksSessionCanceled(t *testing.T) {
	originalNow := nowFunc
	nowFunc = func() time.Time {
		return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() { nowFunc = originalNow })

	block := make(chan struct{})
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsReplayEnabled: true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
	}
	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return &mockAgentOpsClient{
			onPoll: func(int) {
				<-block
			},
			polls: []kgo.Fetches{nil},
		}, nil
	}
	session, err := svc.startReplay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !svc.cancelReplay(session.ID) {
		t.Fatal("expected replay cancel to succeed")
	}
	close(block)
	waitForReplayStatus(t, svc.file, session.ID, "canceled")
	doc := svc.file.Snapshot()
	if doc.Health.ReplayStatus != "canceled" || doc.Health.ReplayActive != 0 {
		t.Fatalf("unexpected replay health %#v", doc.Health)
	}
}

func TestStartReplayWithTopicsUsesScopedSubscription(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsReplayEnabled: true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests", "group.core.responses"},
		file:   mustSqliteStore(t),
	}
	var gotTopics []string
	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		gotTopics = append([]string{}, topics...)
		return &mockAgentOpsClient{polls: []kgo.Fetches{nil, nil, nil}}, nil
	}
	session, err := svc.startReplayWithTopics(context.Background(), []string{"group.core.responses"})
	if err != nil {
		t.Fatal(err)
	}
	waitForReplayStatus(t, svc.file, session.ID, "completed")
	if len(gotTopics) != 1 || gotTopics[0] != "group.core.responses" {
		t.Fatalf("expected scoped replay topics, got %#v", gotTopics)
	}
}

func TestProcessReplayRequestsClaimsAndStartsReplay(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsReplayEnabled: true,
			AgentOpsGroupName:     "core",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
	}
	if err := svc.file.Apply(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO replay_requests (id, requested_at, status, topics_json) VALUES ('req-1', '2026-04-20T10:00:00Z', 'pending', '["group.core.responses"]')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var gotTopics []string
	svc.replayStarter = func(ctx context.Context, topics []string) (store.ReplaySession, error) {
		gotTopics = append([]string{}, topics...)
		return store.ReplaySession{ID: "session-1", GroupID: "group-replay", Status: "running", StartedAt: "2026-04-20T10:00:01Z", Topics: topics}, nil
	}
	if err := svc.processReplayRequests(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(gotTopics) != 1 || gotTopics[0] != "group.core.responses" {
		t.Fatalf("unexpected replay topics %#v", gotTopics)
	}
	var status, startedSessionID string
	if err := svc.file.Apply(func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT status, COALESCE(started_session_id, '') FROM replay_requests WHERE id = 'req-1'`).Scan(&status, &startedSessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" || startedSessionID != "session-1" {
		t.Fatalf("unexpected replay request state status=%q session=%q", status, startedSessionID)
	}
}

func mustSqliteStore(t *testing.T) *store.SqliteStore {
	t.Helper()
	fs, err := store.NewSqliteStore("", store.Snapshot{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestNewClientValidationAndSecurity(t *testing.T) {
	if _, err := newClient(collectorcfg.Config{}, []string{"group.core.requests"}, "group-a", "client"); err == nil || err.Error() == "" {
		t.Fatal("expected broker validation error")
	}
	if _, err := newClient(collectorcfg.Config{AgentOpsBrokers: []string{"localhost:9092"}}, nil, "group-a", "client"); err == nil {
		t.Fatal("expected topics validation error")
	}
	if _, err := newClient(collectorcfg.Config{AgentOpsBrokers: []string{"localhost:9092"}}, []string{"group.core.requests"}, "", "client"); err == nil {
		t.Fatal("expected group validation error")
	}
	if _, err := newClient(collectorcfg.Config{
		AgentOpsBrokers:          []string{"localhost:9092"},
		AgentOpsSecurityProtocol: "UNSUPPORTED",
	}, []string{"group.core.requests"}, "group-a", "client"); err == nil {
		t.Fatal("expected unsupported protocol error")
	}
	client, err := newClient(collectorcfg.Config{
		AgentOpsBrokers: []string{"localhost:9092"},
	}, []string{"group.core.requests"}, "group-a", "client")
	if err != nil {
		t.Fatalf("expected plaintext client to build: %v", err)
	}
	client.Close()
	for _, cfg := range []collectorcfg.Config{
		{
			AgentOpsBrokers:               []string{"localhost:9092"},
			AgentOpsSecurityProtocol:      "SSL",
			AgentOpsTLSInsecureSkipVerify: true,
		},
		{
			AgentOpsBrokers:               []string{"localhost:9092"},
			AgentOpsSecurityProtocol:      "SASL_SSL",
			AgentOpsTLSInsecureSkipVerify: true,
			AgentOpsUsername:              "user",
			AgentOpsPassword:              "pass",
			AgentOpsSASLMechanism:         "PLAIN",
		},
		{
			AgentOpsBrokers:          []string{"localhost:9092"},
			AgentOpsSecurityProtocol: "SASL_PLAINTEXT",
			AgentOpsUsername:         "user",
			AgentOpsPassword:         "pass",
			AgentOpsSASLMechanism:    "SCRAM-SHA-512",
		},
	} {
		client, err := newClient(cfg, []string{"group.core.requests"}, "group-a", "client")
		if err != nil {
			t.Fatalf("expected client for protocol %q: %v", cfg.AgentOpsSecurityProtocol, err)
		}
		client.Close()
	}
}

func TestSASLMechanismValidation(t *testing.T) {
	if _, err := saslMechanism(collectorcfg.Config{AgentOpsUsername: "user"}); err == nil {
		t.Fatal("expected missing password validation error")
	}
	for _, mech := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		if _, err := saslMechanism(collectorcfg.Config{
			AgentOpsUsername:      "user",
			AgentOpsPassword:      "pass",
			AgentOpsSASLMechanism: mech,
		}); err != nil {
			t.Fatalf("expected mechanism %s to work: %v", mech, err)
		}
	}
	if _, err := saslMechanism(collectorcfg.Config{
		AgentOpsUsername:      "user",
		AgentOpsPassword:      "pass",
		AgentOpsSASLMechanism: "BAD",
	}); err == nil {
		t.Fatal("expected unsupported mechanism error")
	}
}

func TestFirstFatalErrorIgnoresContextErrors(t *testing.T) {
	if err := firstFatalError(nil); err != nil {
		t.Fatalf("expected nil fetches to return nil, got %v", err)
	}
	if err := firstFatalError(fetchesWithErr(context.Canceled)); err != nil {
		t.Fatalf("expected canceled context to be ignored, got %v", err)
	}
	if err := firstFatalError(fetchesWithErr(context.DeadlineExceeded)); err != nil {
		t.Fatalf("expected deadline to be ignored, got %v", err)
	}
	if err := firstFatalError(fetchesWithErr(kgo.ErrClientClosed)); !errors.Is(err, kgo.ErrClientClosed) {
		t.Fatalf("expected client closed to propagate, got %v", err)
	}
	want := errors.New("boom")
	if err := firstFatalError(fetchesWithErr(want)); !errors.Is(err, want) {
		t.Fatalf("expected fatal error, got %v", err)
	}
}

func TestHelperFunctions(t *testing.T) {
	rec := &kgo.Record{}
	if got := recordTimestamp(rec); got == "" {
		t.Fatal("expected zero timestamp fallback")
	}
	if got := previewForPayload(json.RawMessage(`{"a":1}`)); got != `{"a":1}` {
		t.Fatalf("unexpected preview %q", got)
	}
	longPayload := json.RawMessage(`"` + strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8) + `"`)
	if got := previewForPayload(longPayload); len(got) <= 180 || got[len(got)-3:] != "..." {
		t.Fatalf("expected truncated preview, got %q", got)
	}
	if got := compactJSON(json.RawMessage(` { "b": 2 } `)); got != `{"b":2}` {
		t.Fatalf("unexpected compact JSON %q", got)
	}
	if got := compactJSON(json.RawMessage(`no-json`)); got != "no-json" {
		t.Fatalf("unexpected raw fallback %q", got)
	}
	if got := compactJSON(nil); got != "" {
		t.Fatalf("expected empty compactJSON, got %q", got)
	}
	if got := previewForPayload(nil); got != "" {
		t.Fatalf("expected empty preview, got %q", got)
	}
	if got := appendUnique([]string{"a"}, "a"); len(got) != 1 {
		t.Fatalf("expected duplicate suppression, got %#v", got)
	}
	if got := appendUnique([]string{"a"}, "b"); len(got) != 2 {
		t.Fatalf("expected append, got %#v", got)
	}
	if got := firstNonEmpty("", "a", "b"); got != "a" {
		t.Fatalf("unexpected firstNonEmpty %q", got)
	}
	if got := fallbackID("", "fallback"); got != "fallback" {
		t.Fatalf("unexpected fallbackID %q", got)
	}
	if got := max(1, 2); got != 2 {
		t.Fatalf("unexpected max %d", got)
	}
	if got := newReplayGroupID("", time.Date(2026, 4, 10, 12, 30, 0, 0, time.UTC)); !strings.HasPrefix(got, "kafsiem-agentops-replay-") {
		t.Fatalf("unexpected replay prefix fallback %q", got)
	}
}

func TestRunProcessesRejectedRecordsAndPersistsHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:     true,
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsClientID:    "client-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
			KafkaPollTimeoutMS:  1,
			UIMode:              "AGENTOPS",
			Profile:             "agentops-default",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
	}
	mock := &mockAgentOpsClient{
		polls: []kgo.Fetches{
			fetchesWithRecords(&kgo.Record{
				Topic:     "group.core.requests",
				Partition: 0,
				Offset:    1,
				Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
				Value:     []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","payload":{"task_id":"task-1","description":"Investigate"}}`),
			}, &kgo.Record{
				Topic:     "group.core.requests",
				Partition: 0,
				Offset:    2,
				Timestamp: time.Date(2026, 4, 10, 12, 0, 1, 0, time.UTC),
				Value:     []byte(`{`),
			}),
			nil,
		},
		onPoll: func(n int) {
			if n >= 2 {
				cancel()
			}
		},
	}
	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return mock, nil
	}

	if err := svc.run(ctx); err != nil {
		t.Fatalf("expected graceful shutdown, got %v", err)
	}
	doc := svc.file.Snapshot()
	if doc.Health.AcceptedCount != 1 || doc.Health.RejectedCount != 1 {
		t.Fatalf("unexpected health counts %#v", doc.Health)
	}
	if doc.Health.MirroredCount != 1 || doc.Health.MirrorFailedCount != 0 {
		t.Fatalf("unexpected mirror counters %#v", doc.Health)
	}
	if doc.Health.RejectedByReason["invalid_envelope"] != 1 {
		t.Fatalf("expected invalid_envelope reject count, got %#v", doc.Health.RejectedByReason)
	}
	if len(mock.committed) != 2 {
		t.Fatalf("expected both records committed, got %d", len(mock.committed))
	}
	if len(mock.produced) != 1 || mock.produced[0].Topic != "group.core.agentops.rejects" {
		t.Fatalf("expected mirrored reject publish, got %#v", mock.produced)
	}
}

func TestRunPersistsHealthCountsAndEffectiveTopics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:     true,
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsClientID:    "client-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
			KafkaPollTimeoutMS:  1,
			UIMode:              "AGENTOPS",
			Profile:             "agentops-default",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests", "group.core.responses"},
		file:   mustSqliteStore(t),
	}
	mock := &mockAgentOpsClient{
		polls: []kgo.Fetches{
			fetchesWithRecords(&kgo.Record{
				Topic:     "group.core.requests",
				Partition: 0,
				Offset:    1,
				Value:     []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","payload":{"task_id":"task-1","description":"Investigate"}}`),
			}, &kgo.Record{
				Topic:     "group.core.responses",
				Partition: 0,
				Offset:    2,
				Value:     []byte(`{`),
			}),
			nil,
		},
		onPoll: func(n int) {
			if n >= 2 {
				cancel()
			}
		},
	}
	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return mock, nil
	}
	if err := svc.run(ctx); err != nil {
		t.Fatalf("expected graceful shutdown, got %v", err)
	}
	doc := svc.file.Snapshot()
	if len(doc.Health.EffectiveTopics) == 0 || doc.Health.AcceptedCount != 1 || doc.Health.RejectedCount != 1 || doc.Health.MirroredCount != 1 {
		t.Fatalf("unexpected health snapshot %#v", doc.Health)
	}
}

func TestHandleRecordReturnsStoreErrorWhenDBClosed(t *testing.T) {
	fs, err := store.NewSqliteStore("", store.Snapshot{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Close(); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg:    collectorcfg.Config{AgentOpsGroupName: "core"},
		policy: agentcfg.DefaultPolicy("core"),
		file:   fs,
	}
	rec := &kgo.Record{
		Topic:     "group.core.requests",
		Partition: 0,
		Offset:    1,
		Value:     []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","payload":{"task_id":"task-1","description":"Investigate"}}`),
	}
	if reason, ok := svc.handleRecord(rec); ok || reason != "store_update_failed" {
		t.Fatalf("expected store_update_failed, got ok=%v reason=%q", ok, reason)
	}
}

func TestBootstrapFromStoreRestoresTraceAndTaskState(t *testing.T) {
	svc := &Service{file: mustSqliteStore(t)}
	svc.bootstrapFromStore(store.Snapshot{
		Flows:    []store.Flow{{ID: "corr-1"}},
		Traces:   []store.Trace{{ID: "trace-1"}},
		Tasks:    []store.Task{{ID: "task-1"}},
		Messages: []store.Message{{ID: "msg-1"}},
	})
	doc := svc.file.Snapshot()
	if len(doc.Flows) != 1 || len(doc.Traces) != 1 || len(doc.Tasks) != 1 || len(doc.Messages) != 1 {
		t.Fatalf("bootstrap did not restore all state: %#v", doc)
	}
}

func TestRunHandlesFatalPollError(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:     true,
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsClientID:    "client-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
			KafkaPollTimeoutMS:  1,
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{polls: []kgo.Fetches{fetchesWithErr(errors.New("poll failed"))}}, nil
		},
	}
	err := svc.run(context.Background())
	if err == nil || err.Error() != "poll failed" {
		t.Fatalf("expected poll failure, got %v", err)
	}
	if got := svc.file.Snapshot().Health.LastReject; got != "poll failed" {
		t.Fatalf("expected health last reject update, got %q", got)
	}
}

func TestRunUsesDefaultClientFactoryWhenNil(t *testing.T) {
	originalFactory := defaultClientFactory
	t.Cleanup(func() {
		defaultClientFactory = originalFactory
	})
	defaultFactoryCalled := false
	defaultClientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		defaultFactoryCalled = true
		return nil, errors.New("default factory used")
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:     true,
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsClientID:    "client-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
		},
		policy:        agentcfg.DefaultPolicy("core"),
		topics:        []string{"group.core.requests"},
		file:          mustSqliteStore(t),
		clientFactory: nil,
	}
	if err := svc.run(context.Background()); err == nil || err.Error() != "default factory used" {
		t.Fatalf("expected nil clientFactory fallback error, got %v", err)
	}
	if !defaultFactoryCalled {
		t.Fatal("expected default client factory to be used")
	}
}

func TestPersistInitializesRejectedReasonMap(t *testing.T) {
	fs, err := store.NewSqliteStore("", store.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
			UIMode:              "AGENTOPS",
			Profile:             "agentops-default",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   fs,
	}
	if err := svc.persist(); err != nil {
		t.Fatal(err)
	}
	if svc.file.Snapshot().Health.RejectedByReason == nil {
		t.Fatal("expected rejected reason map to be initialized")
	}
}

func TestRunReturnsCommitError(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:     true,
			AgentOpsGroupName:   "core",
			AgentOpsGroupID:     "group-a",
			AgentOpsClientID:    "client-a",
			AgentOpsRejectTopic: "group.core.agentops.rejects",
			KafkaPollTimeoutMS:  1,
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{
				polls: []kgo.Fetches{fetchesWithRecords(&kgo.Record{
					Topic:     "group.core.requests",
					Partition: 0,
					Offset:    1,
					Value:     []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","payload":{"task_id":"task-1","description":"Investigate"}}`),
				})},
				commitErr: errors.New("commit failed"),
			}, nil
		},
	}
	err := svc.run(context.Background())
	if err == nil || err.Error() != "commit failed" {
		t.Fatalf("expected commit failure, got %v", err)
	}
}

func TestRunReplayFailureAndCompletion(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsReplayPrefix:  "replay-core",
			AgentOpsReplayEnabled: true,
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
	}
	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		if strings.HasSuffix(clientID, "-replay") {
			return nil, errors.New("dial failed")
		}
		return &mockAgentOpsClient{}, nil
	}
	session, err := svc.startReplay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitForReplayStatus(t, svc.file, session.ID, "failed")

	svc.clientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return &mockAgentOpsClient{
			polls: []kgo.Fetches{
				fetchesWithRecords(&kgo.Record{
					Topic:     "group.core.requests",
					Partition: 0,
					Offset:    1,
					Value:     []byte(`{"type":"request","correlation_id":"corr-5","sender_id":"worker-a","payload":{"task_id":"task-5","description":"Replay me"}}`),
				}),
				nil,
				nil,
				nil,
			},
		}, nil
	}
	session, err = svc.startReplay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitForReplayStatus(t, svc.file, session.ID, "completed")
}

func TestReplayFlowEndToEndWithDedicatedGroup(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-live",
			AgentOpsClientID:      "client-a",
			AgentOpsReplayPrefix:  "replay-core",
			AgentOpsReplayEnabled: true,
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{
				polls: []kgo.Fetches{
					fetchesWithRecords(&kgo.Record{
						Topic:     "group.core.requests",
						Partition: 0,
						Offset:    1,
						Value:     []byte(`{"type":"request","correlation_id":"corr-e2e","sender_id":"worker-a","payload":{"task_id":"task-e2e","description":"Replay me"}}`),
					}),
					nil, nil, nil,
				},
			}, nil
		},
	}
	session, err := svc.startReplay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.GroupID == "group-live" {
		t.Fatalf("replay group must differ from live group: %#v", session)
	}
	waitForReplayStatus(t, svc.file, session.ID, "completed")
	doc := svc.file.Snapshot()
	if len(doc.Messages) == 0 || doc.ReplaySessions[0].GroupID == "group-live" {
		t.Fatalf("unexpected replay state %#v", doc)
	}
}

func TestRunReplayUsesNilFactoryFallbackAndFatalFetchError(t *testing.T) {
	originalFactory := defaultClientFactory
	t.Cleanup(func() {
		defaultClientFactory = originalFactory
	})
	defaultFactoryCalled := false
	defaultClientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		defaultFactoryCalled = true
		return &mockAgentOpsClient{polls: []kgo.Fetches{fetchesWithErr(errors.New("replay poll failed"))}}, nil
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsReplayEnabled: true,
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
	}
	svc.runReplay(context.Background(), store.ReplaySession{ID: "session-1", GroupID: "group-replay"})
	if !defaultFactoryCalled {
		t.Fatal("expected default replay client factory to be used")
	}
	if got := svc.file.Snapshot().Health.LastReject; got != "replay poll failed" {
		t.Fatalf("expected replay fetch error to be surfaced, got %q", got)
	}
}

func TestRunReplayProcessedCap(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsReplayEnabled: true,
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.Policy{
			Version:   1,
			GroupName: "core",
			Grouping:  agentcfg.Grouping{FlowKey: "correlation_id", ReplayMaxRecords: 50},
		},
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			records := make([]*kgo.Record, 0, 52)
			for i := 0; i < 52; i++ {
				records = append(records, &kgo.Record{
					Topic:     "group.core.requests",
					Partition: 0,
					Offset:    int64(i),
					Value:     []byte(fmt.Sprintf(`{"type":"request","correlation_id":"corr-%d","sender_id":"worker-a","payload":{"task_id":"task-%d","description":"x"}}`, i, i)),
				})
			}
			return &mockAgentOpsClient{polls: []kgo.Fetches{fetchesWithRecords(records...)}, commitErr: nil}, nil
		},
	}
	if err := svc.file.Update(func(doc *store.Snapshot) {
		doc.ReplaySessions = []store.ReplaySession{{ID: "session-limit", GroupID: "group-replay", Status: "running"}}
	}); err != nil {
		t.Fatal(err)
	}
	svc.runReplay(context.Background(), store.ReplaySession{ID: "session-limit", GroupID: "group-replay"})
	doc := svc.file.Snapshot()
	var session store.ReplaySession
	for _, item := range doc.ReplaySessions {
		if item.ID == "session-limit" {
			session = item
			break
		}
	}
	if session.Status != "completed" || session.MessageCount != 50 {
		t.Fatalf("expected replay processed cap at 50, got %#v", session)
	}
}

func TestStartReplayReturnsUpdateError(t *testing.T) {
	fs, err := store.NewSqliteStore("", store.Snapshot{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Close(); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsReplayEnabled: true,
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   fs,
	}
	if _, err := svc.startReplay(context.Background()); err == nil {
		t.Fatal("expected replay session update error")
	}
}

func TestStartReplayWithoutPrefixAndGlobalReplay(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
			AgentOpsReplayEnabled: true,
			AgentOpsRejectTopic:   "group.core.agentops.rejects",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustSqliteStore(t),
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{polls: []kgo.Fetches{nil, nil, nil}}, nil
		},
	}
	setCurrentService(svc)
	defer setCurrentService(nil)

	session, err := StartReplay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(session.GroupID, "kafsiem-agentops-replay-") {
		t.Fatalf("unexpected default replay group id %q", session.GroupID)
	}
	waitForReplayStatus(t, svc.file, session.ID, "completed")
}

func TestStartReplayTrimsHistoryToTenSessions(t *testing.T) {
	sessions := make([]store.ReplaySession, 10)
	for i := range sessions {
		sessions[i] = store.ReplaySession{ID: fmt.Sprintf("old-%d", i), Status: "completed"}
	}
	fs, err := store.NewSqliteStore("", store.Snapshot{
		ReplaySessions: sessions,
		Health:         store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:       true,
			AgentOpsReplayEnabled: true,
			AgentOpsGroupName:     "core",
			AgentOpsGroupID:       "group-a",
			AgentOpsClientID:      "client-a",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   fs,
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{polls: []kgo.Fetches{nil, nil, nil}}, nil
		},
	}
	session, err := svc.startReplay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitForReplayStatus(t, svc.file, session.ID, "completed")
	doc := svc.file.Snapshot()
	if len(doc.ReplaySessions) != 10 {
		t.Fatalf("expected replay history to be trimmed to 10, got %d", len(doc.ReplaySessions))
	}
}

func TestLoadOperatorStateReturnsGroupsAndReplayIDs(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-live",
			AgentOpsClientID:  "client-a",
		},
		file: mustSqliteStore(t),
		operatorClientFactory: func(cfg collectorcfg.Config, clientID string) (operatorClient, error) {
			return &mockOperatorClient{
				groups: []store.ConsumerGroup{
					{GroupID: "group-live", State: "Stable", ProtocolType: "consumer", Protocol: "range"},
					{GroupID: "group-replay", State: "Empty", ProtocolType: "consumer", Protocol: "range"},
				},
			}, nil
		},
	}
	if err := svc.file.Update(func(doc *store.Snapshot) {
		doc.ReplaySessions = []store.ReplaySession{{ID: "replay-1", GroupID: "group-replay", Status: "completed"}}
	}); err != nil {
		t.Fatal(err)
	}
	state, err := svc.loadOperatorState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Supported || state.LiveGroupID != "group-live" || len(state.ReplayGroupIDs) != 1 || state.ReplayGroupIDs[0] != "group-replay" {
		t.Fatalf("unexpected operator state %#v", state)
	}
	if len(state.Groups) != 2 {
		t.Fatalf("expected groups in operator state, got %#v", state.Groups)
	}
}

func TestLoadOperatorStateReturnsUnsupportedOnAdminFailure(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-live",
			AgentOpsClientID:  "client-a",
		},
		file: mustSqliteStore(t),
		operatorClientFactory: func(cfg collectorcfg.Config, clientID string) (operatorClient, error) {
			return nil, errors.New("unsupported admin api")
		},
	}
	state, err := svc.loadOperatorState(context.Background())
	if err == nil || err.Error() != "unsupported admin api" {
		t.Fatalf("expected operator error, got %v", err)
	}
	if state.Supported {
		t.Fatalf("expected unsupported state, got %#v", state)
	}
}

type mockAgentOpsClient struct {
	polls      []kgo.Fetches
	pollIndex  int
	committed  []*kgo.Record
	produced   []*kgo.Record
	commitErr  error
	produceErr error
	closed     bool
	onPoll     func(int)
}

type mockOperatorClient struct {
	groups []store.ConsumerGroup
	err    error
}

func (m *mockAgentOpsClient) PollFetches(context.Context) kgo.Fetches {
	m.pollIndex++
	if m.onPoll != nil {
		m.onPoll(m.pollIndex)
	}
	if m.pollIndex-1 >= len(m.polls) {
		return nil
	}
	return m.polls[m.pollIndex-1]
}

func (m *mockAgentOpsClient) CommitRecords(_ context.Context, recs ...*kgo.Record) error {
	m.committed = append(m.committed, recs...)
	return m.commitErr
}

func (m *mockAgentOpsClient) ProduceSync(_ context.Context, rec *kgo.Record) error {
	m.produced = append(m.produced, rec)
	return m.produceErr
}

func (m *mockAgentOpsClient) Close() {
	m.closed = true
}

func (m *mockOperatorClient) ListGroups(context.Context) ([]store.ConsumerGroup, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.groups, nil
}

func (m *mockOperatorClient) Close() {}

func fetchesWithRecords(records ...*kgo.Record) kgo.Fetches {
	return kgo.Fetches{
		{
			Topics: []kgo.FetchTopic{
				{
					Topic: "group.core.requests",
					Partitions: []kgo.FetchPartition{
						{Partition: 0, Records: records},
					},
				},
			},
		},
	}
}

func fetchesWithErr(err error) kgo.Fetches {
	return kgo.Fetches{
		{
			Topics: []kgo.FetchTopic{
				{
					Topic: "group.core.requests",
					Partitions: []kgo.FetchPartition{
						{Partition: 0, Err: err},
					},
				},
			},
		},
	}
}

func waitForReplayStatus(t *testing.T, fs *store.SqliteStore, id string, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		doc := fs.Snapshot()
		for _, session := range doc.ReplaySessions {
			if session.ID == id && session.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for replay status %q", want)
}
