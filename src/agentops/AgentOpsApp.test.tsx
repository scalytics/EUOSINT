import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { AgentOpsApp } from "@/agentops/AgentOpsApp";
import type { AgentOpsState } from "@/types/agentops";

const baseState: AgentOpsState = {
  generated_at: "2026-04-10T12:00:00Z",
  enabled: true,
  ui_mode: "AGENTOPS",
  profile: "agentops-default",
  group_name: "core",
  topics: ["group.core.requests"],
  flow_count: 1,
  trace_count: 1,
  task_count: 1,
  message_count: 2,
  health: {
    connected: true,
    effective_topics: ["group.core.requests"],
    group_id: "euosint-agentops",
    accepted_count: 2,
    rejected_count: 0,
    mirrored_count: 0,
    rejected_by_reason: {},
    topic_health: [
      {
        topic: "group.core.requests",
        messages_per_hour: 2,
        active_agents: 1,
        is_stale: false,
        last_message_at: "2026-04-10T12:00:00Z",
      },
    ],
  },
  replay_sessions: [],
  flows: [
    {
      id: "corr-1",
      topic_count: 1,
      sender_count: 1,
      topics: ["group.core.requests"],
      senders: ["worker-a"],
      trace_ids: ["trace-1"],
      task_ids: ["task-1"],
      first_seen: "2026-04-10T12:00:00Z",
      last_seen: "2026-04-10T12:00:00Z",
      latest_status: "in_progress",
      message_count: 2,
      latest_preview: "Investigate outage",
    },
  ],
  traces: [
    {
      id: "trace-1",
      span_count: 1,
      agents: ["worker-a"],
      span_types: ["TOOL"],
      latest_title: "trace title",
      started_at: "2026-04-10T12:00:00Z",
      ended_at: "2026-04-10T12:00:01Z",
      duration_ms: 1000,
    },
  ],
  tasks: [
    {
      id: "task-1",
      requester_id: "worker-a",
      responder_id: "worker-b",
      status: "in_progress",
      description: "Investigate outage",
      first_seen: "2026-04-10T12:00:00Z",
      last_seen: "2026-04-10T12:00:01Z",
    },
  ],
  messages: [
    {
      id: "msg-1",
      topic: "group.core.requests",
      topic_family: "requests",
      partition: 0,
      offset: 1,
      timestamp: "2026-04-10T12:00:00Z",
      correlation_id: "corr-1",
      content: "{\"task_id\":\"task-1\"}",
      preview: "Investigate outage",
      sender_id: "worker-a",
      task_id: "task-1",
    },
    {
      id: "msg-2",
      topic: "group.core.requests",
      topic_family: "requests",
      partition: 0,
      offset: 2,
      timestamp: "2026-04-10T12:00:01Z",
      correlation_id: "corr-1",
      preview: "LFS-backed payload",
      lfs: {
        bucket: "ops",
        key: "core/requests/2",
        size: 88,
        sha256: "abc",
        path: "s3://ops/core/requests/2",
      },
    },
  ],
};

beforeEach(() => {
  vi.restoreAllMocks();
});

test("renders flow desk panels with decoded content and LFS pointer metadata", () => {
  render(<AgentOpsApp state={baseState} mode="AGENTOPS" />);

  expect(screen.getByText("Flow Desk")).toBeTruthy();
  expect(screen.getByText("Flow Queue")).toBeTruthy();
  expect(screen.getByText("Trace Graph")).toBeTruthy();
  expect(screen.getByText("Agent Context")).toBeTruthy();
  expect(screen.getByText("Topic Health")).toBeTruthy();
  expect(screen.getByText("Replay Panel")).toBeTruthy();
  expect(screen.getAllByText("Investigate outage").length).toBeGreaterThan(0);
  expect(screen.getByText("s3://ops/core/requests/2")).toBeTruthy();
  expect(screen.getByText("LFS-backed payload")).toBeTruthy();
});

test("renders hybrid fusion shell without mixing lanes", () => {
  render(<AgentOpsApp state={{ ...baseState, ui_mode: "HYBRID" }} mode="HYBRID" />);

  expect(screen.getByText("Fusion Desk")).toBeTruthy();
  expect(screen.getByText("Agent Flow")).toBeTruthy();
  expect(screen.getByText("Fusion Summary")).toBeTruthy();
  expect(screen.getByText("External Intel Context")).toBeTruthy();
});

test("triggers replay through the AgentOps API", async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true });
  vi.stubGlobal("fetch", fetchMock);

  render(<AgentOpsApp state={baseState} mode="AGENTOPS" />);
  fireEvent.click(screen.getByRole("button", { name: /start from earliest/i }));

  expect(fetchMock).toHaveBeenCalledWith("/api/agentops/replay", { method: "POST" });
});
