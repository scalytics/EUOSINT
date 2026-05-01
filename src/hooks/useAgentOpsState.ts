import { useEffect, useState } from "react";
import { demoShellMode, isAgentOpsDemo } from "@/agentops/lib/demo";
import { loadPersistedShell, normalizeState, persistShell } from "@/agentops/lib/state";
import type { AgentOpsState } from "@/agentops/types";

function fallbackState(): AgentOpsState {
  const demoMode = demoShellMode();
  if (demoMode) {
    return normalizeState({
      generated_at: new Date().toISOString(),
      enabled: true,
      ui_mode: demoMode,
      profile: demoMode === "HYBRID" ? "hybrid-ops" : "agentops-default",
      group_name: "kafsiem-ontology-demo",
      topics: ["ops.drones.telemetry.v1", "ops.drones.ontology.edges.v1", "ot.scada.modbus.readings.v1", "ot.scada.ontology.edges.v1"],
    });
  }
  return normalizeState(loadPersistedShell());
}

export function useAgentOpsState() {
  const [state, setState] = useState<AgentOpsState>(() => fallbackState());
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    const shell = fallbackState();
    if (!isAgentOpsDemo()) {
      persistShell(shell.ui_mode, shell.profile);
    }
    setState(shell);
    setIsLoading(false);
    return undefined;
  }, []);

  return { state, isLoading };
}
