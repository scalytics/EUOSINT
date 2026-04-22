import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useAgentOpsState } from "@/hooks/useAgentOpsState";

beforeEach(() => {
  vi.useFakeTimers();
  window.history.replaceState({}, "", "/");
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.history.replaceState({}, "", "/");
});

test("uses persisted shell state without fetching outside demo mode", async () => {
  const store = new Map<string, string>([
    ["kafsiem.ui_mode", "AGENTOPS"],
    ["kafsiem.profile", "agentops-default"],
  ]);
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => {
        store.set(key, value);
      },
    },
  });
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

test("loads demo state and polls again on the refresh interval", async () => {
  window.history.replaceState({}, "", "/?demo=agentops");
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ui_mode: "AGENTOPS", enabled: true, group_name: "core" }),
    })
    .mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ui_mode: "HYBRID", enabled: true, group_name: "core" }),
    });
  vi.stubGlobal("fetch", fetchMock);

  const { result } = renderHook(() => useAgentOpsState());

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  expect(result.current.isLoading).toBe(false);
  expect(result.current.state.ui_mode).toBe("AGENTOPS");

  await act(async () => {
    vi.advanceTimersByTime(15000);
    await Promise.resolve();
    await Promise.resolve();
  });

  expect(result.current.state.ui_mode).toBe("HYBRID");
  expect(fetchMock).toHaveBeenCalledTimes(2);
});
