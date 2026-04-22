import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "@/App";

const agentOpsState = {
  generated_at: "2026-04-16T10:05:00Z",
  enabled: true,
  ui_mode: "AGENTOPS",
  profile: "agentops-default",
  group_name: "agent-runtime-core",
  topics: ["group.core.requests", "group.core.responses"],
  flow_count: 1,
  trace_count: 1,
  task_count: 1,
  message_count: 2,
  health: {
    connected: true,
    effective_topics: ["group.core.requests", "group.core.responses"],
    group_id: "agentops-core-live",
    accepted_count: 2,
    rejected_count: 1,
    mirrored_count: 1,
    mirror_failed_count: 0,
    rejected_by_reason: { invalid_envelope: 1 },
    replay_active: 0,
    replay_last_record_count: 14,
    topic_health: [
      {
        topic: "group.core.requests",
        messages_per_hour: 44,
        message_density: "high",
        active_agents: 3,
        is_stale: false,
        last_message_at: "2026-04-16T10:04:10Z",
      },
    ],
  },
  replay_sessions: [
    {
      id: "replay-001",
      group_id: "agentops-replay-001",
      status: "completed",
      started_at: "2026-04-16T09:57:40Z",
      finished_at: "2026-04-16T09:58:00Z",
      message_count: 14,
      topics: ["group.core.requests"],
    },
  ],
  flows: [
    {
      id: "corr-cve-002",
      topic_count: 2,
      sender_count: 2,
      topics: ["group.core.requests", "group.core.responses"],
      senders: ["orchestrator", "cyber-agent"],
      trace_ids: ["trace-cve-2"],
      task_ids: ["task-cve-2"],
      first_seen: "2026-04-16T10:01:00Z",
      last_seen: "2026-04-16T10:04:40Z",
      latest_status: "in_progress",
      message_count: 2,
      latest_preview: "Map vulnerable F5 assets against current advisory coverage.",
    },
  ],
  traces: [
    {
      id: "trace-cve-2",
      span_count: 2,
      agents: ["orchestrator", "cyber-agent"],
      span_types: ["REQUEST", "TRACE"],
      latest_title: "F5 asset coverage",
      started_at: "2026-04-16T10:01:00Z",
      ended_at: "2026-04-16T10:04:32Z",
      duration_ms: 212000,
    },
  ],
  tasks: [
    {
      id: "task-cve-2",
      requester_id: "orchestrator",
      responder_id: "cyber-agent",
      status: "in_progress",
      description: "Map vulnerable F5 assets.",
      last_summary: "Cyber-agent is correlating F5 inventory with current external advisories.",
      first_seen: "2026-04-16T10:01:00Z",
      last_seen: "2026-04-16T10:04:32Z",
    },
  ],
  messages: [
    {
      id: "msg-005",
      topic: "group.core.requests",
      topic_family: "requests",
      partition: 0,
      offset: 124,
      timestamp: "2026-04-16T10:01:00Z",
      envelope_type: "request",
      sender_id: "orchestrator",
      correlation_id: "corr-cve-002",
      trace_id: "trace-cve-2",
      task_id: "task-cve-2",
      preview: "Map vulnerable F5 assets against current advisory coverage.",
      content:
        "{\"category\":\"cyber_advisory\",\"event_country_code\":\"DE\",\"vendor\":\"F5\",\"product\":\"BIG-IP\",\"cve\":\"CVE-2026-12345\",\"request\":\"Map vulnerable F5 assets against current advisory coverage.\"}",
    },
    {
      id: "msg-008",
      topic: "group.core.responses",
      topic_family: "responses",
      partition: 0,
      offset: 127,
      timestamp: "2026-04-16T10:04:40Z",
      envelope_type: "response",
      sender_id: "cyber-agent",
      correlation_id: "corr-cve-002",
      trace_id: "trace-cve-2",
      task_id: "task-cve-2",
      preview: "LFS-backed asset export pointer",
      lfs: {
        bucket: "agentops",
        key: "exports/f5-assets-20260416.ndjson",
        size: 19084,
        sha256: "8adfe45b",
        content_type: "application/x-ndjson",
        path: "s3://agentops/exports/f5-assets-20260416.ndjson",
      },
    },
  ],
};

const alerts = [
  {
    alert_id: "advisory-f5-2026-12345",
    source_id: "cert-bund",
    source: {
      source_id: "cert-bund",
      authority_name: "CERT-Bund",
      country: "Germany",
      country_code: "DE",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://www.bsi.bund.de",
    },
    title: "Critical F5 BIG-IP advisory for CVE-2026-12345",
    canonical_url: "https://www.bsi.bund.de/advisory/f5-2026-12345",
    first_seen: "2026-04-16T09:30:00Z",
    last_seen: "2026-04-16T09:45:00Z",
    status: "active",
    category: "cyber_advisory",
    severity: "high",
    region_tag: "EU",
    lat: 52.52,
    lng: 13.405,
    freshness_hours: 1,
    triage: {
      relevance_score: 0.91,
      disposition: "active",
    },
  },
];

const operator = {
  supported: true,
  live_group_id: "agentops-core-live",
  replay_group_ids: ["agentops-replay-001"],
  groups: [
    {
      group_id: "agentops-core-live",
      state: "Stable",
      protocol_type: "consumer",
      protocol: "range",
      members: [],
    },
  ],
};

beforeEach(() => {
  window.history.replaceState({}, "", "/?demo=agentops");
});

afterEach(() => {
  vi.unstubAllGlobals();
  window.history.replaceState({}, "", "/");
});

test("boots the real app into the AgentOps dashboard with demo-backed mocked Kafka traffic", async () => {
  const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/demo/agentops-state.json")) {
      return { ok: true, json: async () => agentOpsState };
    }
    if (url.includes("/demo/alerts.json")) {
      return { ok: true, json: async () => alerts };
    }
    if (url.includes("/demo/agentops-groups.json")) {
      return { ok: true, json: async () => operator };
    }
    if (url.includes("/api/demo/agentops/replay") && init?.method === "POST") {
      return { ok: true, json: async () => ({ status: "accepted" }) };
    }
    throw new Error(`unexpected fetch ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<App />);

  expect(await screen.findByText("Operations Desk")).toBeTruthy();
  expect(await screen.findByText("agent-runtime-core")).toBeTruthy();
  expect(await screen.findByText("Investigation Workspace")).toBeTruthy();
  expect(await screen.findByText("s3://agentops/exports/f5-assets-20260416.ndjson")).toBeTruthy();
  expect(screen.queryByText("Critical F5 BIG-IP advisory for CVE-2026-12345")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: /start from earliest/i }));
  expect(screen.getByText("Replay request queued.")).toBeTruthy();

  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith("/api/demo/agentops/replay", { method: "POST" }),
  );
  expect(await screen.findByText("Replay accepted.")).toBeTruthy();
});
