import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useAgentOpsState } from "@/hooks/useAgentOpsState";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

test("loads AgentOps state and polls again on the refresh interval", async () => {
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

test("falls back to OSINT defaults on fetch failure", async () => {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("boom")));

  const { result } = renderHook(() => useAgentOpsState());

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  expect(result.current.isLoading).toBe(false);
  expect(result.current.state.ui_mode).toBe("OSINT");
  expect(result.current.state.enabled).toBe(false);
});

test("derives profile defaults from ui_mode when profile is missing", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ ui_mode: "HYBRID", enabled: true, group_name: "core" }),
    }),
  );

  const { result } = renderHook(() => useAgentOpsState());

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });

  expect(result.current.state.ui_mode).toBe("HYBRID");
  expect(result.current.state.profile).toBe("hybrid-ops");
});

test("preserves persisted mode and profile on fetch failure", async () => {
  const store = new Map<string, string>([
    ["euosint.ui_mode", "AGENTOPS"],
    ["euosint.profile", "agentops-default"],
  ]);
  vi.stubGlobal("window", {
    ...window,
    localStorage: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => {
        store.set(key, value);
      },
    },
  });
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("boom")));

  const { result } = renderHook(() => useAgentOpsState());

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });

  expect(result.current.state.ui_mode).toBe("AGENTOPS");
  expect(result.current.state.profile).toBe("agentops-default");
  expect(result.current.state.enabled).toBe(false);
});
