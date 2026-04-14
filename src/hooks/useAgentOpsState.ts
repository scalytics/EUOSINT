import { useEffect, useState } from "react";
import { loadPersistedShell, normalizeState, persistShell } from "@/agentops/lib/state";
import { appURL } from "@/lib/app-url";
import type { AgentOpsState } from "@/agentops/types";

const AGENTOPS_STATE_URL = appURL("agentops-state.json");
const POLL_MS = 15000;

function fallbackState(): AgentOpsState {
  return normalizeState(loadPersistedShell());
}

export function useAgentOpsState() {
  const [state, setState] = useState<AgentOpsState>(() => fallbackState());
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const response = await fetch(`${AGENTOPS_STATE_URL}?t=${Date.now()}`, { cache: "no-store" });
        if (!response.ok) {
          throw new Error(`agentops state fetch failed: ${response.status}`);
        }
        const data = normalizeState((await response.json()) as Partial<AgentOpsState>);
        if (!cancelled) {
          persistShell(data.ui_mode, data.profile);
          setState(data);
          setIsLoading(false);
        }
      } catch {
        if (!cancelled) {
          setState((current) => normalizeState(current.generated_at ? current : fallbackState()));
          setIsLoading(false);
        }
      }
    }

    load();
    const id = window.setInterval(load, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  return { state, isLoading };
}
