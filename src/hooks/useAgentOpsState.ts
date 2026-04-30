import { useEffect, useState } from "react";
import { loadPersistedShell, normalizeState, persistShell } from "@/agentops/lib/state";
import type { AgentOpsState } from "@/agentops/types";

function fallbackState(): AgentOpsState {
  return normalizeState(loadPersistedShell());
}

export function useAgentOpsState() {
  const [state, setState] = useState<AgentOpsState>(() => fallbackState());
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    const shell = fallbackState();
    persistShell(shell.ui_mode, shell.profile);
    setState(shell);
    setIsLoading(false);
    return undefined;
  }, []);

  return { state, isLoading };
}
