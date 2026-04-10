import { AgentOpsApp } from "@/agentops/AgentOpsApp";
import { useAgentOpsState } from "@/hooks/useAgentOpsState";
import OsintApp from "@/osint/OsintApp";

export default function App() {
  const { state } = useAgentOpsState();

  if (state.ui_mode === "AGENTOPS" || state.ui_mode === "HYBRID") {
    return <AgentOpsApp state={state} mode={state.ui_mode} />;
  }

  return <OsintApp />;
}
