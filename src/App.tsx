import { useEffect } from "react";
import { AgentOpsApp } from "@/agentops/AgentOpsApp";
import { useAgentOpsState } from "@/hooks/useAgentOpsState";
import OsintApp from "@/osint/OsintApp";

export default function App() {
  const { state } = useAgentOpsState();

  useEffect(() => {
    switch (state.ui_mode) {
      case "AGENTOPS":
        document.title = "Agent Flow Desk";
        break;
      case "HYBRID":
        document.title = "Hybrid Flow Desk";
        break;
      case "OSINT":
      default:
        document.title = "OSINT";
        break;
    }
  }, [state.ui_mode]);

  if (state.ui_mode === "AGENTOPS" || state.ui_mode === "HYBRID") {
    return <AgentOpsApp state={state} mode={state.ui_mode} />;
  }

  return <OsintApp />;
}
