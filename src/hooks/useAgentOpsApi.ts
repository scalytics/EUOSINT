import { useEffect, useRef, useState } from "react";
import { AgentOpsApiClient } from "@/agentops/lib/api-client";
import type {
  Cursor,
  FeatureCollection,
  Flow,
  Health,
  MapLayer,
  Message,
  NeighborhoodResponse,
  Pack,
  Profile,
  Provenance,
  ReplaySession,
  SearchResult,
  Task,
  TopicHealth,
  Trace,
} from "@/agentops/lib/api-client/types";

const POLL_MS = 5000;
const client = new AgentOpsApiClient();

type QueryState<T> = {
  data: T;
  isLoading: boolean;
  error: string | null;
  refresh: () => void;
};

function usePolledQuery<T>(load: (signal: AbortSignal) => Promise<T>, initial: T, deps: readonly unknown[]): QueryState<T> {
  const [data, setData] = useState<T>(initial);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    let cancelled = false;
    let controller: AbortController | null = null;

    const run = () => {
      controller?.abort();
      controller = new AbortController();
      setIsLoading((current) => current && error === null);
      void load(controller.signal)
        .then((next) => {
          if (cancelled) return;
          setData(next);
          setError(null);
          setIsLoading(false);
        })
        .catch((err: unknown) => {
          if (cancelled || (err instanceof DOMException && err.name === "AbortError")) return;
          setError(err instanceof Error ? err.message : "request failed");
          setIsLoading(false);
        });
    };

    loadRef.current = run;
    run();
    const interval = window.setInterval(run, POLL_MS);
    const onFocus = () => run();
    const onVisible = () => {
      if (document.visibilityState === "visible") run();
    };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      cancelled = true;
      controller?.abort();
      window.clearInterval(interval);
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, deps);

  return {
    data,
    isLoading,
    error,
    refresh: () => loadRef.current?.(),
  };
}

export function useFlows(filter: { after?: Cursor; limit?: number; topic?: string; sender?: string; status?: string; q?: string } = {}) {
  const { data, ...rest } = usePolledQuery(
    (signal) => client.listFlows(filter, { signal }),
    { items: [] as Flow[], next: null as Cursor },
    [filter.after, filter.limit, filter.q, filter.sender, filter.status, filter.topic],
  );
  return { flows: data.items, next: data.next, ...rest };
}

export function useFlow(id: string | null) {
  const { data, ...rest } = usePolledQuery<Flow | null>(
    (signal) => (id ? client.getFlow(id, { signal }) : Promise.resolve(null)),
    null,
    [id],
  );
  return { flow: data, ...rest };
}

export function useEntityProfile(type: string | null, id: string | null) {
  const { data, ...rest } = usePolledQuery<Profile | null>(
    (signal) => (type && id ? client.getEntity(type, id, { signal }) : Promise.resolve(null)),
    null,
    [type, id],
  );
  return { profile: data, ...rest };
}

export function useEntityNeighborhood(type: string | null, id: string | null, params: { depth?: number; types?: string; window?: string } = {}) {
  const { data, ...rest } = usePolledQuery<NeighborhoodResponse>(
    (signal) => (type && id ? client.getEntityNeighborhood(type, id, params, { signal }) : Promise.resolve({ entities: [], edges: [] })),
    { entities: [], edges: [] },
    [type, id, params.depth, params.types, params.window],
  );
  return { neighborhood: data, ...rest };
}

export function useEntityTimeline(type: string | null, id: string | null, page: { after?: Cursor; limit?: number } = {}) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (type && id ? client.listEntityTimeline(type, id, page, { signal }) : Promise.resolve({ items: [] as Message[], next: null as Cursor })),
    { items: [] as Message[], next: null as Cursor },
    [type, id, page.after, page.limit],
  );
  return { messages: data.items, next: data.next, ...rest };
}

export function useEntityProvenance(type: string | null, id: string | null) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (type && id ? client.listEntityProvenance(type, id, { signal }) : Promise.resolve({ items: [] as Provenance[], next: null as Cursor })),
    { items: [] as Provenance[], next: null as Cursor },
    [type, id],
  );
  return { provenance: data.items, ...rest };
}

export function useSearchEntities(q: string) {
  const query = q.trim();
  const { data, ...rest } = usePolledQuery(
    (signal) => (query ? client.searchEntities(query, { signal }) : Promise.resolve({ items: [] as SearchResult[], next: null as Cursor })),
    { items: [] as SearchResult[], next: null as Cursor },
    [query],
  );
  return { results: data.items, next: data.next, ...rest };
}

export function useFlowMessages(id: string | null, page: { after?: Cursor; limit?: number } = {}) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (id ? client.listFlowMessages(id, page, { signal }) : Promise.resolve({ items: [] as Message[], next: null as Cursor })),
    { items: [] as Message[], next: null as Cursor },
    [id, page.after, page.limit],
  );
  return { messages: data.items, next: data.next, ...rest };
}

export function useFlowTasks(id: string | null) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (id ? client.listFlowTasks(id, { signal }) : Promise.resolve({ items: [] as Task[], next: null as Cursor })),
    { items: [] as Task[], next: null as Cursor },
    [id],
  );
  return { tasks: data.items, ...rest };
}

export function useFlowTraces(id: string | null) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (id ? client.listFlowTraces(id, { signal }) : Promise.resolve({ items: [] as Trace[], next: null as Cursor })),
    { items: [] as Trace[], next: null as Cursor },
    [id],
  );
  return { traces: data.items, ...rest };
}

export function useTopicHealth() {
  const { data, ...rest } = usePolledQuery(
    (signal) => client.listTopicHealth({ signal }),
    { items: [] as TopicHealth[], next: null as Cursor },
    [],
  );
  return { topicHealth: data.items, ...rest };
}

export function useHealth() {
  const { data, ...rest } = usePolledQuery<Health | null>((signal) => client.getHealth({ signal }), null, []);
  return { health: data, ...rest };
}

export function useReplaySessions(limit = 20) {
  const { data, ...rest } = usePolledQuery(
    (signal) => client.listReplays(limit, { signal }),
    { items: [] as ReplaySession[], next: null as Cursor },
    [limit],
  );
  return { replaySessions: data.items, ...rest };
}

export function useMapLayers() {
  const { data, ...rest } = usePolledQuery(
    (signal) => client.listMapLayers({ signal }),
    { items: [] as MapLayer[], next: null as Cursor },
    [],
  );
  return { mapLayers: data.items, ...rest };
}

export function useOntologyPacks() {
  const { data, ...rest } = usePolledQuery(
    (signal) => client.getOntologyPacks({ signal }),
    { items: [] as Pack[], next: null as Cursor },
    [],
  );
  return { packs: data.items, ...rest };
}

export function useMapFeatures(params: { bbox: string; types?: string; window?: string }) {
  const { data, ...rest } = usePolledQuery<FeatureCollection>(
    (signal) => client.listMapFeatures(params, { signal }),
    { type: "FeatureCollection", features: [] },
    [params.bbox, params.types, params.window],
  );
  return { featureCollection: data, ...rest };
}
