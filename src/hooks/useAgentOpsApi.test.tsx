import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { useFlowMessages, useFlows, useHealth, useMapFeatures, useReplaySessions, useTopicHealth } from "@/hooks/useAgentOpsApi";

afterEach(() => {
  vi.unstubAllGlobals();
});

test("loads typed flow and health resources from the api", async () => {
  const fetchMock = vi.fn(async (input: string | URL) => {
    const url = String(input);
    if (url.includes("/api/v1/flows?")) {
      return { ok: true, json: async () => ({ items: [{ id: "corr-1", topic_count: 1, sender_count: 1, topics: ["a"], senders: ["b"], trace_ids: [], task_ids: [], first_seen: "2026-04-10T12:00:00Z", last_seen: "2026-04-10T12:00:01Z", message_count: 1 }], next: null }) };
    }
    if (url.endsWith("/api/v1/health")) {
      return { ok: true, json: async () => ({ connected: true, effective_topics: ["a"], group_id: "group-a", accepted_count: 1, rejected_count: 0, mirrored_count: 0, mirror_failed_count: 0, rejected_by_reason: {}, replay_active: 0, replay_last_record_count: 0, topic_health: [] }) };
    }
    if (url.endsWith("/api/v1/topic-health")) {
      return { ok: true, json: async () => ({ items: [{ topic: "a", messages_per_hour: 12, message_density: "low", active_agents: 1, is_stale: false }], next: null }) };
    }
    if (url.includes("/api/v1/replays")) {
      return { ok: true, json: async () => ({ items: [{ id: "replay-1", group_id: "group-a-replay", status: "accepted", started_at: "2026-04-10T12:00:00Z", message_count: 1 }], next: null }) };
    }
    if (url.includes("/api/v1/map/features")) {
      return { ok: true, json: async () => ({ type: "FeatureCollection", features: [] }) };
    }
    throw new Error(`unexpected fetch ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);

  const flows = renderHook(() => useFlows({ limit: 10 }));
  const health = renderHook(() => useHealth());
  const topicHealth = renderHook(() => useTopicHealth());
  const replays = renderHook(() => useReplaySessions());
  const map = renderHook(() => useMapFeatures({ bbox: "14.40,35.80,14.60,36.00" }));

  await waitFor(() => expect(flows.result.current.flows).toHaveLength(1));
  await waitFor(() => expect(health.result.current.health?.group_id).toBe("group-a"));
  await waitFor(() => expect(topicHealth.result.current.topicHealth).toHaveLength(1));
  await waitFor(() => expect(replays.result.current.replaySessions).toHaveLength(1));
  await waitFor(() => expect(map.result.current.featureCollection.type).toBe("FeatureCollection"));
});

test("loads paged flow messages", async () => {
  const fetchMock = vi.fn(async (input: string | URL) => {
    const url = String(input);
    if (url.includes("/api/v1/flows/corr-1/messages")) {
      return { ok: true, json: async () => ({ items: [{ id: "msg-1", topic: "group.core.requests", topic_family: "requests", partition: 0, offset: 1, timestamp: "2026-04-10T12:00:00Z" }], next: null }) };
    }
    throw new Error(`unexpected fetch ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);

  const { result } = renderHook(() => useFlowMessages("corr-1", { limit: 20 }));

  await waitFor(() => expect(result.current.messages).toHaveLength(1));

  expect(fetchMock).toHaveBeenCalled();
});
