import { useEffect, useState } from "react";
import { EMPTY_OPERATOR_STATE } from "@/agentops/lib/operator";
import type { AgentOpsOperatorState } from "@/agentops/types";

export function useAgentOpsOperator(enabled: boolean) {
  const [state, setState] = useState<AgentOpsOperatorState>(EMPTY_OPERATOR_STATE);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      if (!enabled) {
        setState(EMPTY_OPERATOR_STATE);
        return;
      }
      try {
        const response = await fetch("/api/agentops/groups", { cache: "no-store" });
        if (!response.ok) {
          const data = (await response.json()) as { state?: AgentOpsOperatorState; error?: string };
          if (!cancelled) {
            setState({ ...EMPTY_OPERATOR_STATE, ...(data.state ?? {}), last_error: data.error || data.state?.last_error });
          }
          return;
        }
        const data = (await response.json()) as AgentOpsOperatorState;
        if (!cancelled) {
          setState({ ...EMPTY_OPERATOR_STATE, ...data });
        }
      } catch {
        if (!cancelled) {
          setState(EMPTY_OPERATOR_STATE);
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  return state;
}
