import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { EntityProfilePage } from "@/agentops/pages/EntityProfilePage";

const mockedHooks = {
  useEntityProfile: vi.fn(),
  useEntityNeighborhood: vi.fn(),
  useEntityTimeline: vi.fn(),
  useEntityProvenance: vi.fn(),
  useFlows: vi.fn(),
  useOntologyPacks: vi.fn(),
};

vi.mock("@/agentops/components/GraphCanvas", () => ({
  GraphCanvas: ({ entities, edges }: { entities: unknown[]; edges: unknown[] }) => <div>Graph Canvas Stub {entities.length}/{edges.length}</div>,
}));

vi.mock("@/hooks/useAgentOpsApi", () => ({
  useEntityProfile: (...args: unknown[]) => mockedHooks.useEntityProfile(...args),
  useEntityNeighborhood: (...args: unknown[]) => mockedHooks.useEntityNeighborhood(...args),
  useEntityTimeline: (...args: unknown[]) => mockedHooks.useEntityTimeline(...args),
  useEntityProvenance: (...args: unknown[]) => mockedHooks.useEntityProvenance(...args),
  useFlows: (...args: unknown[]) => mockedHooks.useFlows(...args),
  useOntologyPacks: (...args: unknown[]) => mockedHooks.useOntologyPacks(...args),
}));

beforeEach(() => {
  window.history.pushState({}, "", "/?view=entity&type=platform&id=auv-7");
  const store = new Map<string, string>();
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value),
      removeItem: (key: string) => store.delete(key),
      clear: () => store.clear(),
    },
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
  mockedHooks.useEntityNeighborhood.mockReturnValue({
    neighborhood: {
      entities: [{ id: "platform:auv-7", type: "platform", canonical_id: "auv-7", first_seen: "2026-04-10T12:00:00Z", last_seen: "2026-04-10T12:00:01Z" }],
      edges: [],
    },
  });
  mockedHooks.useEntityTimeline.mockReturnValue({
    messages: [{ id: "msg-1", topic: "group.core.requests", topic_family: "requests", partition: 0, offset: 1, timestamp: "2026-04-10T12:00:00Z", sender_id: "agent-a" }],
    next: null,
  });
  mockedHooks.useEntityProvenance.mockReturnValue({
    provenance: [{ subject_kind: "entity", subject_id: "platform:auv-7", stage: "graph", decision: "inserted", produced_at: "2026-04-10T12:00:00Z" }],
  });
  mockedHooks.useFlows.mockReturnValue({
    flows: [{ id: "corr-1", topic_count: 1, sender_count: 1, topics: ["group.core.requests"], senders: ["agent-a"], trace_ids: [], task_ids: [], first_seen: "2026-04-10T12:00:00Z", last_seen: "2026-04-10T12:00:01Z", message_count: 1, latest_preview: "Inspect platform" }],
  });
  mockedHooks.useOntologyPacks.mockReturnValue({
    packs: [{
      name: "drones",
      version: "0.1.0",
      edge_types: ["assigned_to"],
      views: [{
        id: "platform",
        entity_type: "platform",
        title: "Platform",
        fields: [
          { id: "serial", label: "Serial" },
          { id: "autonomy_version", label: "Autonomy Version" },
        ],
        source: "pack/drones",
      }],
    }],
  });
});

test("renders the routed pack-aware entity profile", () => {
  render(<EntityProfilePage mode="AGENTOPS" />);

  expect(screen.getAllByText("AUV-7").length).toBeGreaterThan(0);
  expect(screen.getByText("drones pack")).toBeTruthy();
  expect(screen.getByText("Serial")).toBeTruthy();
  expect(screen.getByText("SN-007")).toBeTruthy();
  expect(screen.getByText("Autonomy Version")).toBeTruthy();
  expect(screen.getByText("7.4.2")).toBeTruthy();
  expect(screen.getByText("Graph Canvas Stub 1/0")).toBeTruthy();
  expect(screen.getByText("Inspect platform")).toBeTruthy();
  expect(screen.getByText("Provenance")).toBeTruthy();
  expect(mockedHooks.useEntityTimeline).toHaveBeenCalledWith("platform", "auv-7", { limit: 50 });
});
