import { isAgentOpsDemo } from "@/agentops/lib/demo";
import { readEntityRoute } from "@/agentops/lib/routes";
import { LegacyAgentOpsDesk } from "@/agentops/pages/AgentOpsDesk";
import { AgentOpsRuntimeDesk } from "@/agentops/pages/AgentOpsRuntimeDesk";
import { EntityProfilePage } from "@/agentops/pages/EntityProfilePage";
import type { AgentOpsMode, AgentOpsState } from "@/agentops/types";

interface Props {
  mode: AgentOpsMode;
  state?: AgentOpsState;
}

function hasLegacyData(state: AgentOpsState | undefined): boolean {
  if (!state) return false;
  return state.flows.length > 0 || state.messages.length > 0 || state.tasks.length > 0 || state.traces.length > 0 || state.replay_sessions.length > 0;
}

export function AgentOpsApp({ mode, state }: Props) {
  if (readEntityRoute()) {
    return <EntityProfilePage mode={mode} />;
  }
  if (isAgentOpsDemo() || hasLegacyData(state)) {
    return <LegacyAgentOpsDesk state={state ?? {
      generated_at: "",
      enabled: false,
      ui_mode: mode,
      profile: "",
      group_name: "",
      topics: [],
      flow_count: 0,
      trace_count: 0,
      task_count: 0,
      message_count: 0,
      health: {
        connected: false,
        effective_topics: [],
        group_id: "",
        accepted_count: 0,
        rejected_count: 0,
        mirrored_count: 0,
        mirror_failed_count: 0,
        rejected_by_reason: {},
        replay_active: 0,
        replay_last_record_count: 0,
        topic_health: [],
      },
      replay_sessions: [],
      flows: [],
      traces: [],
      tasks: [],
      messages: [],
    }} mode={mode} />;
  }
  return <AgentOpsRuntimeDesk mode={mode} />;
}
