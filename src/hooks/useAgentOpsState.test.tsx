import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useAgentOpsState } from "@/hooks/useAgentOpsState";

function installStorage(entries: Array<[string, string]> = []) {
  const store = new Map<string, string>(entries);
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => {
        store.set(key, value);
      },
      clear: () => {
        store.clear();
      },
    },
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  window.history.replaceState({}, "", "/");
  installStorage();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.history.replaceState({}, "", "/");
});

test("uses persisted shell state without fetching outside demo mode", async () => {
  installStorage([
    ["kafsiem.ui_mode", "AGENTOPS"],
    ["kafsiem.profile", "agentops-default"],
  ]);
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);

  const { result } = renderHook(() => useAgentOpsState());

  await act(async () => {
    await Promise.resolve();
  });

  expect(result.current.isLoading).toBe(false);
  expect(result.current.state.ui_mode).toBe("AGENTOPS");
  expect(result.current.state.profile).toBe("agentops-default");
  expect(fetchMock).not.toHaveBeenCalled();
});

test("does not fetch legacy AgentOps JSON demo state", async () => {
  window.history.replaceState({}, "", "/?demo=agentops");
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);

  const { result } = renderHook(() => useAgentOpsState());

  await act(async () => {
    await Promise.resolve();
  });

  expect(result.current.isLoading).toBe(false);
  expect(result.current.state.ui_mode).toBe("OSINT");
  expect(fetchMock).not.toHaveBeenCalled();
});
