import { afterEach, beforeEach, describe, expect, test } from "vitest";
import {
  loadAnomaliesOnly,
  loadQueueFilter,
  loadSelectedRunId,
  persistAnomaliesOnly,
  persistQueueFilter,
  persistSelectedRunId,
} from "@/agentops/lib/preferences";

function installStorage() {
  const store = new Map<string, string>();
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => {
        store.set(key, value);
      },
      removeItem: (key: string) => {
        store.delete(key);
      },
      clear: () => {
        store.clear();
      },
    },
  });
}

describe("preferences", () => {
  beforeEach(() => installStorage());
  afterEach(() => window.localStorage.clear());

  test("persists and loads selected run id", () => {
    expect(loadSelectedRunId()).toBeNull();
    persistSelectedRunId("corr-1");
    expect(loadSelectedRunId()).toBe("corr-1");
    persistSelectedRunId(null);
    expect(loadSelectedRunId()).toBeNull();
  });

  test("defaults queue filter to attention and restores valid values", () => {
    expect(loadQueueFilter()).toBe("attention");
    persistQueueFilter("completed");
    expect(loadQueueFilter()).toBe("completed");
  });

  test("persists anomalies-only flag", () => {
    expect(loadAnomaliesOnly()).toBe(false);
    persistAnomaliesOnly(true);
    expect(loadAnomaliesOnly()).toBe(true);
  });
});
