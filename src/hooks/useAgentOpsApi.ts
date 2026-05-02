import { useEffect, useRef, useState } from "react";
import { agentOpsDataSource } from "@/agentops/lib/api-source";
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
const QUERY_DEP_UNDEFINED = "__kafsiem_query_dep_undefined__";

type QueryState<T> = {
  data: T;
  isLoading: boolean;
  error: string | null;
  refresh: () => void;
};

function serializeQueryDeps(deps: readonly unknown[]): string {
  return JSON.stringify(deps, (_key, value) => (value === undefined ? QUERY_DEP_UNDEFINED : value));
}

function usePolledQuery<T>(load: (signal: AbortSignal) => Promise<T>, initial: T, deps: readonly unknown[]): QueryState<T> {
  const [data, setData] = useState<T>(initial);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const queryKey = serializeQueryDeps(deps);
  const refreshRef = useRef<(() => void) | null>(null);
  const loadRef = useRef(load);
  const errorRef = useRef<string | null>(null);
  const requestIdRef = useRef(0);

  useEffect(() => {
    loadRef.current = load;
  }, [load]);

  useEffect(() => {
    errorRef.current = error;
  }, [error]);

  useEffect(() => {
    let controller: AbortController | null = null;

    const run = () => {
      controller?.abort();
      controller = new AbortController();
      const requestId = requestIdRef.current + 1;
      requestIdRef.current = requestId;
      setIsLoading((current) => current && errorRef.current === null);
      void loadRef.current(controller.signal)
        .then((next) => {
          if (requestIdRef.current !== requestId) return;
          setData(next);
          setError(null);
          setIsLoading(false);
        })
        .catch((err: unknown) => {
          if (requestIdRef.current !== requestId || (err instanceof DOMException && err.name === "AbortError")) return;
          setError(err instanceof Error ? err.message : "request failed");
          setIsLoading(false);
        });
    };

    refreshRef.current = run;
    run();
    const interval = window.setInterval(run, POLL_MS);
    const onFocus = () => run();
    const onVisible = () => {
      if (document.visibilityState === "visible") run();
    };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      requestIdRef.current += 1;
      refreshRef.current = null;
      controller?.abort();
      window.clearInterval(interval);
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [queryKey]);

  return {
    data,
    isLoading,
    error,
    refresh: () => refreshRef.current?.(),
  };
}

export function useFlows(filter: { after?: Cursor; limit?: number; topic?: string; sender?: string; status?: string; q?: string } = {}) {
  const { data, ...rest } = usePolledQuery(
    (signal) => agentOpsDataSource().listFlows(filter, { signal }),
    { items: [] as Flow[], next: null as Cursor },
    [filter.after, filter.limit, filter.q, filter.sender, filter.status, filter.topic],
  );
  return { flows: data.items, next: data.next, ...rest };
}

export function useFlow(id: string | null) {
  const { data, ...rest } = usePolledQuery<Flow | null>(
    (signal) => (id ? agentOpsDataSource().getFlow(id, { signal }) : Promise.resolve(null)),
    null,
    [id],
  );
  return { flow: data, ...rest };
}

export function useEntityProfile(type: string | null, id: string | null) {
  const { data, ...rest } = usePolledQuery<Profile | null>(
    (signal) => (type && id ? agentOpsDataSource().getEntity(type, id, { signal }) : Promise.resolve(null)),
    null,
    [type, id],
  );
  return { profile: data, ...rest };
}

export function useEntityNeighborhood(type: string | null, id: string | null, params: { depth?: number; types?: string; window?: string } = {}) {
  const { data, ...rest } = usePolledQuery<NeighborhoodResponse>(
    (signal) => (type && id ? agentOpsDataSource().getEntityNeighborhood(type, id, params, { signal }) : Promise.resolve({ entities: [], edges: [] })),
    { entities: [], edges: [] },
    [type, id, params.depth, params.types, params.window],
  );
  return { neighborhood: data, ...rest };
}

export function useEntityTimeline(type: string | null, id: string | null, page: { after?: Cursor; limit?: number } = {}) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (type && id ? agentOpsDataSource().listEntityTimeline(type, id, page, { signal }) : Promise.resolve({ items: [] as Message[], next: null as Cursor })),
    { items: [] as Message[], next: null as Cursor },
    [type, id, page.after, page.limit],
  );
  return { messages: data.items, next: data.next, ...rest };
}

export function useEntityProvenance(type: string | null, id: string | null) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (type && id ? agentOpsDataSource().listEntityProvenance(type, id, { signal }) : Promise.resolve({ items: [] as Provenance[], next: null as Cursor })),
    { items: [] as Provenance[], next: null as Cursor },
    [type, id],
  );
  return { provenance: data.items, ...rest };
}

export function useSearchEntities(q: string) {
  const query = q.trim();
  const { data, ...rest } = usePolledQuery(
    (signal) => (query ? agentOpsDataSource().searchEntities(query, { signal }) : Promise.resolve({ items: [] as SearchResult[], next: null as Cursor })),
    { items: [] as SearchResult[], next: null as Cursor },
    [query],
  );
  return { results: data.items, next: data.next, ...rest };
}

export function useFlowMessages(id: string | null, page: { after?: Cursor; limit?: number } = {}) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (id ? agentOpsDataSource().listFlowMessages(id, page, { signal }) : Promise.resolve({ items: [] as Message[], next: null as Cursor })),
    { items: [] as Message[], next: null as Cursor },
    [id, page.after, page.limit],
  );
  return { messages: data.items, next: data.next, ...rest };
}

export function useFlowTasks(id: string | null) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (id ? agentOpsDataSource().listFlowTasks(id, { signal }) : Promise.resolve({ items: [] as Task[], next: null as Cursor })),
    { items: [] as Task[], next: null as Cursor },
    [id],
  );
  return { tasks: data.items, ...rest };
}

export function useFlowTraces(id: string | null) {
  const { data, ...rest } = usePolledQuery(
    (signal) => (id ? agentOpsDataSource().listFlowTraces(id, { signal }) : Promise.resolve({ items: [] as Trace[], next: null as Cursor })),
    { items: [] as Trace[], next: null as Cursor },
    [id],
  );
  return { traces: data.items, ...rest };
}

export function useTopicHealth() {
  const { data, ...rest } = usePolledQuery(
    (signal) => agentOpsDataSource().listTopicHealth({ signal }),
    { items: [] as TopicHealth[], next: null as Cursor },
    [],
  );
  return { topicHealth: data.items, ...rest };
}

export function useHealth() {
  const { data, ...rest } = usePolledQuery<Health | null>((signal) => agentOpsDataSource().getHealth({ signal }), null, []);
  return { health: data, ...rest };
}

export function useReplaySessions(limit = 20) {
  const { data, ...rest } = usePolledQuery(
    (signal) => agentOpsDataSource().listReplays(limit, { signal }),
    { items: [] as ReplaySession[], next: null as Cursor },
    [limit],
  );
  return { replaySessions: data.items, ...rest };
}

export function useMapLayers() {
  const { data, ...rest } = usePolledQuery(
    (signal) => agentOpsDataSource().listMapLayers({ signal }),
    { items: [] as MapLayer[], next: null as Cursor },
    [],
  );
  return { mapLayers: data.items, ...rest };
}

export function useOntologyPacks() {
  const { data, ...rest } = usePolledQuery(
    (signal) => agentOpsDataSource().getOntologyPacks({ signal }),
    { items: [] as Pack[], next: null as Cursor },
    [],
  );
  return { packs: data.items, ...rest };
}

export function useMapFeatures(params: { bbox: string; types?: string; window?: string }) {
  const { data, ...rest } = usePolledQuery<FeatureCollection>(
    (signal) => agentOpsDataSource().listMapFeatures(params, { signal }),
    { type: "FeatureCollection", features: [] },
    [params.bbox, params.types, params.window],
  );
  return { featureCollection: data, ...rest };
}
