import { expect, test } from "vitest";
import { buildFailureBuckets } from "@/agentops/lib/failures";
import type { AgentOpsFlow, AgentOpsHealth, AgentOpsMessage, AgentOpsTask } from "@/agentops/types";

test("buildFailureBuckets surfaces missing responses and transport failures", () => {
  const flow: AgentOpsFlow = {
    id: "corr-1",
    topic_count: 1,
    sender_count: 1,
    topics: ["group.core.requests"],
    senders: ["worker-a"],
    trace_ids: [],
    task_ids: ["task-1"],
    first_seen: "2026-04-10T12:00:00Z",
    last_seen: "2026-04-10T12:01:00Z",
    latest_status: "in_progress",
    message_count: 1,
    latest_preview: "Investigate outage",
  };
  const messages: AgentOpsMessage[] = [
    {
      id: "msg-1",
      topic: "group.core.requests",
      topic_family: "requests",
      partition: 0,
      offset: 1,
      timestamp: "2026-04-10T12:00:00Z",
      correlation_id: "corr-1",
      preview: "Investigate outage",
    },
  ];
  const tasks: AgentOpsTask[] = [
    {
      id: "task-1",
      status: "in_progress",
      first_seen: "2026-04-10T12:00:00Z",
      last_seen: "2026-04-10T12:01:00Z",
    },
  ];
  const health: AgentOpsHealth = {
    connected: true,
    effective_topics: [],
    group_id: "group",
    accepted_count: 1,
    rejected_count: 2,
    mirrored_count: 1,
    mirror_failed_count: 0,
    rejected_by_reason: {},
    last_reject: "bad envelope",
    replay_active: 0,
    replay_last_record_count: 0,
    topic_health: [],
  };

  const buckets = buildFailureBuckets(flow, messages, tasks, health);
  expect(buckets.map((bucket) => bucket.id)).toEqual(
    expect.arrayContaining(["missing-response", "stalled-task", "rejected-records", "mirrored-rejects"]),
  );
});
