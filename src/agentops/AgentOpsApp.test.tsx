import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { AgentOpsApp } from "@/agentops/AgentOpsApp";
import type { AgentOpsOperatorState, AgentOpsState } from "@/types/agentops";
import type { Alert } from "@/types/alert";

const mockedUseAgentOpsOperator = vi.fn<() => AgentOpsOperatorState>(() => ({
  supported: true,
  live_group_id: "euosint-agentops",
  replay_group_ids: ["replay-1"],
  groups: [{ group_id: "euosint-agentops", state: "Stable", protocol_type: "consumer", protocol: "range", members: [] }],
}));

vi.mock("@/hooks/useAgentOpsOperator", () => ({
  useAgentOpsOperator: () => mockedUseAgentOpsOperator(),
}));

const mockedUseAlerts = vi.fn<() => { alerts: Alert[]; isLive: boolean; isLoading: boolean; sourceCount: number; refetch: () => void }>(() => ({
  alerts: [],
  isLive: false,
  isLoading: false,
  sourceCount: 0,
  refetch: vi.fn(),
}));

vi.mock("@/hooks/useAlerts", () => ({
  useAlerts: () => mockedUseAlerts(),
}));

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
    mirror_failed_count: 0,
    rejected_by_reason: {},
    replay_active: 0,
    replay_last_record_count: 0,
    topic_health: [
      {
        topic: "group.core.requests",
        messages_per_hour: 2,
        message_density: "low",
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
  mockedUseAgentOpsOperator.mockReset();
  mockedUseAgentOpsOperator.mockReturnValue({
    supported: true,
    live_group_id: "euosint-agentops",
    replay_group_ids: ["replay-1"],
    groups: [{ group_id: "euosint-agentops", state: "Stable", protocol_type: "consumer", protocol: "range", members: [] }],
  });
  mockedUseAlerts.mockReset();
  mockedUseAlerts.mockReturnValue({
    alerts: [],
    isLive: false,
    isLoading: false,
    sourceCount: 0,
    refetch: vi.fn(),
  });
});

test("renders flow desk panels with decoded content and LFS pointer metadata", () => {
  render(<AgentOpsApp state={baseState} mode="AGENTOPS" />);

  expect(screen.getByText("Flow Desk")).toBeTruthy();
  expect(screen.getByText("Run Queue")).toBeTruthy();
  expect(screen.getByText("Conversation Timeline")).toBeTruthy();
  expect(screen.getByText("Run Context")).toBeTruthy();
  expect(screen.getByText("Topic Health")).toBeTruthy();
  expect(screen.getByText("Replay Panel")).toBeTruthy();
  expect(screen.getByText("Kafscale Operator")).toBeTruthy();
  expect(screen.getAllByText("Investigate outage").length).toBeGreaterThan(0);
  expect(screen.getByText("Missing response")).toBeTruthy();
  expect(screen.getByText("s3://ops/core/requests/2")).toBeTruthy();
  expect(screen.getAllByText("LFS-backed payload").length).toBeGreaterThan(0);
});

test("renders hybrid fusion shell without mixing lanes", () => {
  mockedUseAlerts.mockReturnValue({
    alerts: [
      {
        alert_id: "a1",
        source_id: "cert-ua",
        source: {
          source_id: "cert-ua",
          authority_name: "CERT-UA",
          country: "Ukraine",
          country_code: "UA",
          region: "Europe",
          authority_type: "cert",
          base_url: "https://example.test",
        },
        title: "Advisory for CVE-2026-12345 affecting energy sector",
        canonical_url: "https://example.test/a1",
        first_seen: "2026-04-10T10:00:00Z",
        last_seen: "2026-04-10T11:00:00Z",
        status: "active",
        category: "cyber_advisory",
        severity: "high",
        region_tag: "EU",
        lat: 0,
        lng: 0,
        freshness_hours: 1,
      },
    ],
    isLive: true,
    isLoading: false,
    sourceCount: 1,
    refetch: vi.fn(),
  });
  const hybridState = {
    ...baseState,
    ui_mode: "HYBRID" as const,
    messages: [
      {
        ...baseState.messages[0],
        content: JSON.stringify({ category: "cyber_advisory", event_country_code: "ua", sector: "energy", cve: "CVE-2026-12345" }),
      },
    ],
  };
  render(<AgentOpsApp state={hybridState} mode="HYBRID" />);

  expect(screen.getByText("Fusion Desk")).toBeTruthy();
  expect(screen.getByText("Agent Flow")).toBeTruthy();
  expect(screen.getByText("Fusion Timeline")).toBeTruthy();
  expect(screen.getByText("External Intel Context")).toBeTruthy();
  expect(screen.getByText("Advisory for CVE-2026-12345 affecting energy sector")).toBeTruthy();
  expect(screen.getByText("category:cyber_advisory")).toBeTruthy();
});

test("renders unsupported operator state without exposing unavailable actions", () => {
  mockedUseAgentOpsOperator.mockReturnValue({
    supported: false,
    live_group_id: "euosint-agentops",
    replay_group_ids: [],
    groups: [],
    last_error: "unsupported admin api",
  });

  render(<AgentOpsApp state={baseState} mode="AGENTOPS" />);

  expect(screen.getByText("limited")).toBeTruthy();
  expect(screen.getByText("unsupported admin api")).toBeTruthy();
});

test("renders explicit no-match fallback in hybrid mode", () => {
  render(<AgentOpsApp state={{ ...baseState, ui_mode: "HYBRID" }} mode="HYBRID" />);

  expect(screen.getByText(/No OSINT fusion match for the selected flow/)).toBeTruthy();
});

test("triggers replay through the AgentOps API", async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true });
  vi.stubGlobal("fetch", fetchMock);

  render(<AgentOpsApp state={baseState} mode="AGENTOPS" />);
  fireEvent.click(screen.getByRole("button", { name: /start from earliest/i }));

  expect(fetchMock).toHaveBeenCalledWith("/api/agentops/replay", { method: "POST" });
  expect(screen.getByText("Replay request queued.")).toBeTruthy();
  await waitFor(() => expect(screen.getByText("Replay accepted.")).toBeTruthy());
  expect(screen.getByText("accepted")).toBeTruthy();
});
