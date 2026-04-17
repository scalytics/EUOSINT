export type RunQueueFilter = "all" | "attention" | "active" | "completed";

const SELECTED_RUN_KEY = "kafsiem.agentops.selected_run";
const FILTER_KEY = "kafsiem.agentops.queue_filter";
const ANOMALIES_ONLY_KEY = "kafsiem.agentops.queue_anomalies_only";

export function loadSelectedRunId(): string | null {
  const storage = safeStorage();
  if (!storage) return null;
  return storage.getItem(SELECTED_RUN_KEY);
}

export function persistSelectedRunId(id: string | null) {
  const storage = safeStorage();
  if (!storage) return;
  if (!id) {
    storage.removeItem(SELECTED_RUN_KEY);
    return;
  }
  storage.setItem(SELECTED_RUN_KEY, id);
}

export function loadQueueFilter(): RunQueueFilter {
  const storage = safeStorage();
  if (!storage) return "attention";
  const raw = (storage.getItem(FILTER_KEY) || "").trim().toLowerCase();
  switch (raw) {
    case "all":
    case "active":
    case "completed":
    case "attention":
      return raw;
    default:
      return "attention";
  }
}

export function persistQueueFilter(filter: RunQueueFilter) {
  const storage = safeStorage();
  if (!storage) return;
  storage.setItem(FILTER_KEY, filter);
}

export function loadAnomaliesOnly(): boolean {
  const storage = safeStorage();
  if (!storage) return false;
  return storage.getItem(ANOMALIES_ONLY_KEY) === "true";
}

export function persistAnomaliesOnly(value: boolean) {
  const storage = safeStorage();
  if (!storage) return;
  storage.setItem(ANOMALIES_ONLY_KEY, value ? "true" : "false");
}

function safeStorage(): Storage | null {
  if (typeof window === "undefined" || !("localStorage" in window)) {
    return null;
  }
  const storage = window.localStorage;
  if (!storage || typeof storage.getItem !== "function" || typeof storage.setItem !== "function") {
    return null;
  }
  return storage;
}
