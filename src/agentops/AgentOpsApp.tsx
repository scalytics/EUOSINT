import { readEntityRoute } from "@/agentops/lib/routes";
import { AgentOpsRuntimeDesk } from "@/agentops/pages/AgentOpsRuntimeDesk";
import { EntityProfilePage } from "@/agentops/pages/EntityProfilePage";
import type { AgentOpsMode } from "@/agentops/types";

interface Props {
  mode: AgentOpsMode;
}

export function AgentOpsApp({ mode }: Props) {
  if (readEntityRoute()) {
    return <EntityProfilePage mode={mode} />;
  }
  return <AgentOpsRuntimeDesk mode={mode} />;
}
