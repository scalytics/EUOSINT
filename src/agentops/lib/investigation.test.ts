import { describe, expect, test } from "vitest";
import { buildConversationTimeline, buildRunSummary, groupRunsForQueue, sortFlowsForQueue } from "@/agentops/lib/investigation";
import type { AgentOpsFlow, AgentOpsHealth, AgentOpsMessage, AgentOpsTask, AgentOpsTrace } from "@/agentops/types";

const health: AgentOpsHealth = {
  connected: true,
  effective_topics: ["group.core.requests"],
  group_id: "group",
  accepted_count: 2,
  rejected_count: 1,
  mirrored_count: 0,
  mirror_failed_count: 0,
  rejected_by_reason: {},
  replay_active: 0,
  replay_last_record_count: 0,
  topic_health: [],
};

const flow: AgentOpsFlow = {
  id: "corr-1",
  topic_count: 2,
  sender_count: 2,
  topics: ["group.core.requests", "group.core.responses"],
  senders: ["orchestrator", "worker-a"],
  trace_ids: ["trace-1"],
  task_ids: ["task-1"],
  first_seen: "2026-04-10T12:00:00Z",
  last_seen: "2026-04-10T12:04:30Z",
  latest_status: "in_progress",
  message_count: 2,
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
    sender_id: "orchestrator",
    preview: "Investigate outage",
  },
  {
    id: "msg-2",
    topic: "group.core.responses",
    topic_family: "responses",
    partition: 0,
    offset: 2,
    timestamp: "2026-04-10T12:04:30Z",
    correlation_id: "corr-1",
    sender_id: "worker-a",
    preview: "Outage summary",
  },
];

const tasks: AgentOpsTask[] = [
  {
    id: "task-1",
    requester_id: "orchestrator",
    responder_id: "worker-a",
    status: "in_progress",
    description: "Investigate outage",
    first_seen: "2026-04-10T12:00:00Z",
    last_seen: "2026-04-10T12:03:00Z",
  },
];

const traces: AgentOpsTrace[] = [
  {
    id: "trace-1",
    span_count: 2,
    agents: ["worker-a"],
    span_types: ["REQUEST", "TRACE"],
    latest_title: "Trace title",
    started_at: "2026-04-10T12:01:00Z",
    ended_at: "2026-04-10T12:02:00Z",
    duration_ms: 60000,
  },
];

describe("investigation helpers", () => {
  test("buildRunSummary surfaces anomalies and confidence", () => {
    const summary = buildRunSummary(flow, messages, tasks, traces, health);
    expect(summary.title).toBe("Investigate outage");
    expect(summary.durationLabel).toBe("4m");
    expect(summary.confidence).toBe("malformed");
    expect(summary.anomalyCount).toBeGreaterThan(0);
    expect(summary.anomalies.map((item) => item.id)).toContain("stalled-task");
  });

  test("buildConversationTimeline inserts dead air markers", () => {
    const timeline = buildConversationTimeline(flow, messages, tasks, traces);
    expect(timeline.some((item) => item.kind === "gap")).toBe(true);
    expect(timeline[0]?.title).toBe("Request dispatched");
  });

  test("sortFlowsForQueue prioritizes higher-risk runs first", () => {
    const completed: AgentOpsFlow = { ...flow, id: "corr-2", latest_status: "completed", last_seen: "2026-04-10T12:05:00Z" };
    const ordered = sortFlowsForQueue([completed, flow], messages, tasks, traces, health);
    expect(ordered[0]?.id).toBe("corr-1");
  });

  test("groupRunsForQueue buckets runs by attention and completion", () => {
    const completed: AgentOpsFlow = { ...flow, id: "corr-2", latest_status: "completed" };
    const sections = groupRunsForQueue([flow, completed], messages, tasks, traces, health);
    expect(sections[0]?.title).toBe("Needs Attention");
    expect(sections[0]?.flowIds).toContain("corr-1");
  });
});
