package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcfg "github.com/scalytics/euosint/internal/agentops/config"
	"github.com/scalytics/euosint/internal/agentops/contract"
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

func TestHandleRecordStatusAuditUnknownAndDuplicateBranches(t *testing.T) {
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
	task := svc.internal.tasks["task-3"]
	if task == nil || task.Status != "waiting" || task.LastSummary != "queued" {
		t.Fatalf("unexpected task state %#v", task)
	}
	msg := svc.internal.msgs["group.core.observe.audit:0:2"]
	if msg.Preview == "" {
		t.Fatalf("expected audit preview, got %#v", msg)
	}
}

func TestUpdateTraceAndTaskGuardBranches(t *testing.T) {
	svc := &Service{
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	svc.updateTrace("2026-04-10T12:00:00Z", contract.TracePayload{}, "worker-a")
	svc.updateTask("2026-04-10T12:00:00Z", "", "", "", "", "", "", "", "")
	if len(svc.internal.traces) != 0 || len(svc.internal.tasks) != 0 {
		t.Fatalf("expected guard branches to avoid state mutation, got %#v", svc.internal)
	}
}

func TestStartDisabledReturnsNil(t *testing.T) {
	if err := Start(context.Background(), collectorcfg.Config{}); err != nil {
		t.Fatalf("expected disabled AgentOps start to return nil, got %v", err)
	}
}

func TestStartValidatesPolicyAndStore(t *testing.T) {
	cfg := collectorcfg.Config{
		AgentOpsEnabled:    true,
		AgentOpsGroupName:  "core",
		AgentOpsPolicyPath: filepath.Join(t.TempDir(), "missing.yaml"),
	}
	if err := Start(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "agentops policy") {
		t.Fatalf("expected policy error, got %v", err)
	}

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "agentops-state.json")
	if err := os.WriteFile(outputPath, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg = collectorcfg.Config{
		AgentOpsEnabled:    true,
		AgentOpsGroupName:  "core",
		AgentOpsGroupID:    "group-a",
		AgentOpsBrokers:    []string{"localhost:9092"},
		AgentOpsOutputPath: outputPath,
	}
	if err := Start(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "agentops store") {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestStartUsesDefaultClientFactoryAndBootstrapsStoredState(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "agentops-state.json")
	initial := `{
	  "generated_at":"2026-04-10T12:00:00Z",
	  "flows":[{"id":"corr-1","first_seen":"2026-04-10T12:00:00Z","last_seen":"2026-04-10T12:00:00Z","message_count":1}],
	  "messages":[{"id":"group.core.requests:0:1","topic":"group.core.requests","topic_family":"requests","partition":0,"offset":1,"timestamp":"2026-04-10T12:00:00Z"}]
	}`
	if err := os.WriteFile(outputPath, []byte(initial), 0o644); err != nil {
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
		AgentOpsEnabled:    true,
		AgentOpsGroupName:  "core",
		AgentOpsGroupID:    "group-a",
		AgentOpsBrokers:    []string{"localhost:9092"},
		AgentOpsOutputPath: outputPath,
		UIMode:             "AGENTOPS",
		Profile:            "agentops-default",
	}
	if err := Start(ctx, cfg); err != nil {
		t.Fatalf("expected Start to succeed with injected client: %v", err)
	}
	currentMu.RLock()
	current := currentService
	currentMu.RUnlock()
	if current == nil || current.internal.flows["corr-1"] == nil || current.internal.msgs["group.core.requests:0:1"].ID == "" {
		t.Fatalf("expected stored state bootstrap, got %#v", current)
	}
}

func TestRunReturnsInitialStatePersistError(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	statePath := filepath.Join(stateDir, "agentops-state.json")
	fs, err := store.NewFileStore(statePath, store.Document{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateDir, []byte("x"), 0o644); err != nil {
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
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	if err := svc.run(context.Background()); err == nil {
		t.Fatal("expected initial persist failure")
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
	if got := newReplayGroupID("", time.Date(2026, 4, 10, 12, 30, 0, 0, time.UTC)); !strings.HasPrefix(got, "euosint-agentops-replay-") {
		t.Fatalf("unexpected replay prefix fallback %q", got)
	}
}

func TestRunProcessesRejectedRecordsAndPersistsHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:    true,
			AgentOpsGroupName:  "core",
			AgentOpsGroupID:    "group-a",
			AgentOpsClientID:   "client-a",
			KafkaPollTimeoutMS: 1,
			UIMode:             "AGENTOPS",
			Profile:            "agentops-default",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
	if doc.Health.RejectedByReason["invalid_envelope"] != 1 {
		t.Fatalf("expected invalid_envelope reject count, got %#v", doc.Health.RejectedByReason)
	}
	if len(mock.committed) != 2 {
		t.Fatalf("expected both records committed, got %d", len(mock.committed))
	}
}

func TestRunReturnsPersistErrorAfterCommit(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	statePath := filepath.Join(stateDir, "agentops-state.json")
	fs, err := store.NewFileStore(statePath, store.Document{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:    true,
			AgentOpsGroupName:  "core",
			AgentOpsGroupID:    "group-a",
			AgentOpsClientID:   "client-a",
			KafkaPollTimeoutMS: 1,
			UIMode:             "AGENTOPS",
			Profile:            "agentops-default",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   fs,
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			return &mockAgentOpsClient{
				polls: []kgo.Fetches{fetchesWithRecords(&kgo.Record{
					Topic:     "group.core.requests",
					Partition: 0,
					Offset:    1,
					Value:     []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","payload":{"task_id":"task-1","description":"Investigate"}}`),
				})},
			}, nil
		},
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.run(context.Background()); err == nil {
		t.Fatal("expected persist error after processing")
	}
}

func TestBootstrapFromStoreRestoresTraceAndTaskState(t *testing.T) {
	svc := &Service{
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	svc.bootstrapFromStore(store.Document{
		Flows:    []store.Flow{{ID: "corr-1"}},
		Traces:   []store.Trace{{ID: "trace-1"}},
		Tasks:    []store.Task{{ID: "task-1"}},
		Messages: []store.Message{{ID: "msg-1"}},
	})
	if svc.internal.flows["corr-1"] == nil || svc.internal.traces["trace-1"] == nil || svc.internal.tasks["task-1"] == nil || svc.internal.msgs["msg-1"].ID == "" {
		t.Fatalf("bootstrap did not restore all state: %#v", svc.internal)
	}
}

func TestRunHandlesFatalPollError(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:    true,
			AgentOpsGroupName:  "core",
			AgentOpsGroupID:    "group-a",
			AgentOpsClientID:   "client-a",
			KafkaPollTimeoutMS: 1,
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-a",
			AgentOpsClientID:  "client-a",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
	fs, err := store.NewFileStore("", store.Document{})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-a",
			UIMode:            "AGENTOPS",
			Profile:           "agentops-default",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   fs,
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
			AgentOpsEnabled:    true,
			AgentOpsGroupName:  "core",
			AgentOpsGroupID:    "group-a",
			AgentOpsClientID:   "client-a",
			KafkaPollTimeoutMS: 1,
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
			AgentOpsEnabled:      true,
			AgentOpsGroupName:    "core",
			AgentOpsGroupID:      "group-a",
			AgentOpsClientID:     "client-a",
			AgentOpsReplayPrefix: "replay-core",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-a",
			AgentOpsClientID:  "client-a",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	svc.runReplay(context.Background(), store.ReplaySession{ID: "session-1", GroupID: "group-replay"})
	if !defaultFactoryCalled {
		t.Fatal("expected default replay client factory to be used")
	}
	if got := svc.file.Snapshot().Health.LastReject; got != "replay poll failed" {
		t.Fatalf("expected replay fetch error to be surfaced, got %q", got)
	}
}

func TestRunReplayLimitFallbackAndProcessedCap(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-a",
			AgentOpsClientID:  "client-a",
		},
		policy: agentcfg.Policy{
			Version:   1,
			GroupName: "core",
			Grouping:  agentcfg.Grouping{FlowKey: "correlation_id", ReplayMaxRecords: 0},
		},
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
		clientFactory: func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
			records := make([]*kgo.Record, 0, 5002)
			for i := 0; i < 5002; i++ {
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
	if err := svc.file.Update(func(doc *store.Document) {
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
	if session.Status != "completed" || session.MessageCount != 5000 {
		t.Fatalf("expected replay processed cap at 5000, got %#v", session)
	}
}

func TestStartReplayReturnsUpdateError(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	statePath := filepath.Join(stateDir, "agentops-state.json")
	fs, err := store.NewFileStore(statePath, store.Document{
		Health: store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
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
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.startReplay(context.Background()); err == nil {
		t.Fatal("expected replay session update error")
	}
}

func TestStartReplayWithoutPrefixAndGlobalReplay(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{
			AgentOpsEnabled:   true,
			AgentOpsGroupName: "core",
			AgentOpsGroupID:   "group-a",
			AgentOpsClientID:  "client-a",
		},
		policy: agentcfg.DefaultPolicy("core"),
		topics: []string{"group.core.requests"},
		file:   mustFileStore(t),
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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
	if !strings.HasPrefix(session.GroupID, "euosint-agentops-replay-") {
		t.Fatalf("unexpected default replay group id %q", session.GroupID)
	}
	waitForReplayStatus(t, svc.file, session.ID, "completed")
}

func TestStartReplayTrimsHistoryToTenSessions(t *testing.T) {
	sessions := make([]store.ReplaySession, 10)
	for i := range sessions {
		sessions[i] = store.ReplaySession{ID: fmt.Sprintf("old-%d", i), Status: "completed"}
	}
	fs, err := store.NewFileStore("", store.Document{
		ReplaySessions: sessions,
		Health:         store.Health{RejectedByReason: map[string]int{}},
	})
	if err != nil {
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
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
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

type mockAgentOpsClient struct {
	polls     []kgo.Fetches
	pollIndex int
	committed []*kgo.Record
	commitErr error
	closed    bool
	onPoll    func(int)
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

func (m *mockAgentOpsClient) Close() {
	m.closed = true
}

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

func waitForReplayStatus(t *testing.T, fs *store.FileStore, id string, want string) {
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
