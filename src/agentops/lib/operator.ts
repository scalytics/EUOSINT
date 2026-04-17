import type { AgentOpsOperatorState } from "@/agentops/types";

export const EMPTY_OPERATOR_STATE: AgentOpsOperatorState = {
  supported: false,
  replay_group_ids: [],
  groups: [],
};
