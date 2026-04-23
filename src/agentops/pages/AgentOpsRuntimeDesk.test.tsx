import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { AgentOpsRuntimeDesk } from "@/agentops/pages/AgentOpsRuntimeDesk";
import type { AgentOpsOperatorState } from "@/types/agentops";
import type { Alert } from "@/types/alert";

const mockedUseAgentOpsOperator = vi.fn<() => AgentOpsOperatorState>(() => ({
  supported: true,
  live_group_id: "kafsiem-agentops",
  replay_group_ids: [],
  groups: [],
}));

const mockedUseAlerts = vi.fn<() => { alerts: Alert[]; isLive: boolean; isLoading: boolean; sourceCount: number; refetch: () => void }>(() => ({
  alerts: [],
  isLive: false,
  isLoading: false,
  sourceCount: 0,
  refetch: vi.fn(),
}));

const mockedHooks = {
  useFlows: vi.fn(),
  useHealth: vi.fn(),
  useTopicHealth: vi.fn(),
  useReplaySessions: vi.fn(),
  useMapLayers: vi.fn(),
  useMapFeatures: vi.fn(),
  useFlow: vi.fn(),
  useFlowMessages: vi.fn(),
  useFlowTasks: vi.fn(),
  useFlowTraces: vi.fn(),
  useOntologyPacks: vi.fn(),
  useEntityProfile: vi.fn(),
};

vi.mock("@/hooks/useAgentOpsOperator", () => ({
  useAgentOpsOperator: () => mockedUseAgentOpsOperator(),
}));

vi.mock("@/hooks/useAlerts", () => ({
  useAlerts: () => mockedUseAlerts(),
}));

vi.mock("@/agentops/components/RuntimeMap", () => ({
  RuntimeMap: () => <div>Map Surface Stub</div>,
}));

vi.mock("@/hooks/useAgentOpsApi", () => ({
  useFlows: (...args: unknown[]) => mockedHooks.useFlows(...args),
  useHealth: (...args: unknown[]) => mockedHooks.useHealth(...args),
  useTopicHealth: (...args: unknown[]) => mockedHooks.useTopicHealth(...args),
  useReplaySessions: (...args: unknown[]) => mockedHooks.useReplaySessions(...args),
  useMapLayers: (...args: unknown[]) => mockedHooks.useMapLayers(...args),
  useMapFeatures: (...args: unknown[]) => mockedHooks.useMapFeatures(...args),
  useFlow: (...args: unknown[]) => mockedHooks.useFlow(...args),
  useFlowMessages: (...args: unknown[]) => mockedHooks.useFlowMessages(...args),
  useFlowTasks: (...args: unknown[]) => mockedHooks.useFlowTasks(...args),
  useFlowTraces: (...args: unknown[]) => mockedHooks.useFlowTraces(...args),
  useOntologyPacks: (...args: unknown[]) => mockedHooks.useOntologyPacks(...args),
  useEntityProfile: (...args: unknown[]) => mockedHooks.useEntityProfile(...args),
}));

beforeEach(() => {
  window.history.pushState({}, "", "/?view=entity&type=platform&id=auv-7");
  mockedUseAgentOpsOperator.mockReset();
  mockedUseAgentOpsOperator.mockReturnValue({
    supported: true,
    live_group_id: "kafsiem-agentops",
    replay_group_ids: [],
    groups: [],
  });
  mockedUseAlerts.mockReset();
  mockedUseAlerts.mockReturnValue({
    alerts: [],
    isLive: false,
    isLoading: false,
    sourceCount: 0,
    refetch: vi.fn(),
  });

  mockedHooks.useFlows.mockReturnValue({
    flows: [{ id: "corr-1", topic_count: 1, sender_count: 1, topics: ["group.core.requests"], senders: ["worker-a"], trace_ids: [], task_ids: [], first_seen: "2026-04-10T12:00:00Z", last_seen: "2026-04-10T12:00:01Z", message_count: 1, latest_preview: "Investigate outage" }],
  });
  mockedHooks.useHealth.mockReturnValue({
    health: { connected: true, effective_topics: ["group.core.requests"], group_id: "kafsiem-agentops", accepted_count: 1, rejected_count: 0, mirrored_count: 0, mirror_failed_count: 0, rejected_by_reason: {}, replay_active: 0, replay_last_record_count: 0, topic_health: [], last_poll_at: "2026-04-10T12:00:01Z" },
  });
  mockedHooks.useTopicHealth.mockReturnValue({ topicHealth: [] });
  mockedHooks.useReplaySessions.mockReturnValue({ replaySessions: [] });
  mockedHooks.useMapLayers.mockReturnValue({ mapLayers: [] });
  mockedHooks.useMapFeatures.mockReturnValue({ featureCollection: { type: "FeatureCollection", features: [] } });
  mockedHooks.useFlow.mockReturnValue({ flow: { id: "corr-1", topic_count: 1, sender_count: 1, topics: ["group.core.requests"], senders: ["worker-a"], trace_ids: [], task_ids: [], first_seen: "2026-04-10T12:00:00Z", last_seen: "2026-04-10T12:00:01Z", message_count: 1, latest_preview: "Investigate outage" } });
  mockedHooks.useFlowMessages.mockReturnValue({ messages: [] });
  mockedHooks.useFlowTasks.mockReturnValue({ tasks: [] });
  mockedHooks.useFlowTraces.mockReturnValue({ traces: [] });
  mockedHooks.useOntologyPacks.mockReturnValue({
    packs: [{
      name: "drones",
      version: "0.1.0",
      views: [{
        id: "platform",
        entity_type: "platform",
        title: "Platform",
        fields: [
          { id: "serial", label: "Serial" },
          { id: "callsign", label: "Callsign" },
          { id: "readiness", label: "Readiness" },
          { id: "autonomy_version", label: "Autonomy Version" },
        ],
        source: "pack/drones",
      }],
    }],
  });
  mockedHooks.useEntityProfile.mockReturnValue({
    profile: {
      entity: {
        id: "platform:auv-7",
        type: "platform",
        canonical_id: "auv-7",
        display_name: "AUV-7",
        first_seen: "2026-04-10T12:00:00Z",
        last_seen: "2026-04-10T12:00:01Z",
        attrs: {
          serial: "SN-007",
          callsign: "TRITON-7",
          readiness: "Green",
          autonomy_version: "7.4.2",
        },
      },
      first_seen: "2026-04-10T12:00:00Z",
      last_seen: "2026-04-10T12:00:01Z",
      edge_counts: { assigned_to: 2 },
      top_neighbors: [{ entity_id: "mission:sea-trial-12", entity_type: "mission", weight: 1 }],
    },
  });
});

test("renders a drones pack entity profile inside the runtime desk", () => {
  render(<AgentOpsRuntimeDesk mode="AGENTOPS" />);

  expect(screen.getByText("Entity View")).toBeTruthy();
  expect(screen.getByText("drones pack")).toBeTruthy();
  expect(screen.getByText("AUV-7")).toBeTruthy();
  expect(screen.getByText("Serial")).toBeTruthy();
  expect(screen.getByText("SN-007")).toBeTruthy();
  expect(screen.getByText("Autonomy Version")).toBeTruthy();
  expect(screen.getByText("7.4.2")).toBeTruthy();
  expect(screen.getByText("mission:sea-trial-12")).toBeTruthy();
  expect(screen.getAllByText("platform:auv-7").length).toBeGreaterThan(0);
});
