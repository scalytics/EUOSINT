import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  useEntityNeighborhood: vi.fn(),
  useEntityTimeline: vi.fn(),
  useEntityProvenance: vi.fn(),
  useSearchEntities: vi.fn(),
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

vi.mock("@/agentops/components/GraphCanvas", () => ({
  GraphCanvas: ({ entities, edges }: { entities: unknown[]; edges: unknown[] }) => <div>Graph Canvas Stub {entities.length}/{edges.length}</div>,
}));

vi.mock("@/agentops/components/ProvenanceDrawer", () => ({
  ProvenanceDrawer: ({ subject }: { subject: { type: string; id: string } | null }) => subject ? <div>Provenance Drawer Stub {subject.type}:{subject.id}</div> : null,
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
  useEntityNeighborhood: (...args: unknown[]) => mockedHooks.useEntityNeighborhood(...args),
  useEntityTimeline: (...args: unknown[]) => mockedHooks.useEntityTimeline(...args),
  useEntityProvenance: (...args: unknown[]) => mockedHooks.useEntityProvenance(...args),
  useSearchEntities: (...args: unknown[]) => mockedHooks.useSearchEntities(...args),
}));

beforeEach(() => {
  window.history.replaceState({}, "", "/");
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
  mockedHooks.useEntityNeighborhood.mockReturnValue({
    neighborhood: {
      entities: [{ id: "correlation:corr-1", type: "correlation", canonical_id: "corr-1", first_seen: "2026-04-10T12:00:00Z", last_seen: "2026-04-10T12:00:01Z" }],
      edges: [],
    },
  });
  mockedHooks.useEntityTimeline.mockReturnValue({ messages: [], next: null });
  mockedHooks.useEntityProvenance.mockReturnValue({ provenance: [] });
  mockedHooks.useSearchEntities.mockImplementation((q: string) => ({
    results: q ? [{ kind: "entity", id: "platform:auv-7", type: "platform", canonical_id: "auv-7", display_name: "AUV-7", score: 1 }] : [],
    isLoading: false,
  }));
  mockedHooks.useOntologyPacks.mockReturnValue({
    packs: [{
      name: "drones",
      version: "0.1.0",
      entity_types: ["platform"],
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

test("renders command search, notes, and topology surfaces in the runtime desk", async () => {
  render(<AgentOpsRuntimeDesk mode="AGENTOPS" />);

  expect(screen.getByPlaceholderText("platform:auv-07 window:24h pack:drones")).toBeTruthy();
  expect(screen.getByText("Notes")).toBeTruthy();
  fireEvent.change(screen.getByPlaceholderText("platform:auv-07 window:24h pack:drones"), { target: { value: "platform:auv-7 window:1h" } });
  fireEvent.submit(screen.getByPlaceholderText("platform:auv-07 window:24h pack:drones").closest("form") as HTMLFormElement);

  await waitFor(() => expect(mockedHooks.useSearchEntities).toHaveBeenLastCalledWith("platform:auv-7 window:1h"));
  expect(screen.getAllByText("AUV-7").length).toBeGreaterThan(0);

  expect(screen.getByText("Graph Canvas Stub 1/0")).toBeTruthy();
  expect(mockedHooks.useEntityNeighborhood).toHaveBeenCalledWith("correlation", "corr-1", { depth: 2 });
});

test("marks mocked ontology demo streams in the console chrome", () => {
  window.history.replaceState({}, "", "/?demo=ontology");

  render(<AgentOpsRuntimeDesk mode="AGENTOPS" />);

  expect(screen.getByText("Ontology demo")).toBeTruthy();
});

test("opens the provenance drawer from a why affordance", async () => {
  mockedHooks.useFlowMessages.mockReturnValue({
    messages: [{ id: "msg-1", topic: "group.core.requests", topic_family: "requests", partition: 0, offset: 1, timestamp: "2026-04-10T12:00:00Z", correlation_id: "corr-1", sender_id: "worker-a" }],
  });
  render(<AgentOpsRuntimeDesk mode="AGENTOPS" />);

  fireEvent.click(screen.getAllByRole("button", { name: /why/i })[0]);
  await waitFor(() => expect(screen.getByText(/Provenance Drawer Stub/)).toBeTruthy());
});
