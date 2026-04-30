import { useEffect } from "react";
import { AgentOpsApp } from "@/agentops/AgentOpsApp";
import { useAgentOpsState } from "@/hooks/useAgentOpsState";
import OsintApp from "@/osint/OsintApp";

export default function App() {
  const { state } = useAgentOpsState();

  useEffect(() => {
    switch (state.ui_mode) {
      case "AGENTOPS":
        document.title = "Operations Console";
        break;
      case "HYBRID":
        document.title = "Fusion Console";
        break;
      case "OSINT":
      default:
        document.title = "OSINT";
        break;
    }
  }, [state.ui_mode]);

  if (state.ui_mode === "AGENTOPS" || state.ui_mode === "HYBRID") {
    return <AgentOpsApp mode={state.ui_mode} />;
  }

  return <OsintApp />;
}
