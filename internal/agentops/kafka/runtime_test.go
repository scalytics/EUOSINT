package kafka

import (
	"testing"
	"time"

	agentcfg "github.com/scalytics/euosint/internal/agentops/config"
	"github.com/scalytics/euosint/internal/agentops/store"
	collectorcfg "github.com/scalytics/euosint/internal/collector/config"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestHandleRecordRequestBuildsFlowAndTask(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{AgentOpsGroupName: "core"},
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
		Value: []byte(`{"type":"request","correlation_id":"corr-1","sender_id":"worker-a","timestamp":"2026-04-10T12:00:00Z","payload":{"task_id":"task-1","description":"Investigate outage","content":"check turbine","requester_id":"worker-a","parent_task_id":"root-1","delegation_depth":1,"original_requester_id":"orch-1"}}`),
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

func TestHandleRecordLFSPointerIsMetadataOnly(t *testing.T) {
	svc := &Service{
		cfg: collectorcfg.Config{AgentOpsGroupName: "core"},
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
		Value: []byte(`{"kfs_lfs":1,"bucket":"ops","key":"core/responses/7","size":88,"sha256":"abc","content_type":"application/json","created_at":"2026-04-10T12:05:00Z","proxy_id":"lfs-1"}`),
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
