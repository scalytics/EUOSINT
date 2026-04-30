import { AgentOpsApiClient } from "@/agentops/lib/api-client";
import { isAgentOpsDemo } from "@/agentops/lib/demo";
import { mockAgentOpsApiClient, type AgentOpsDataSource } from "@/agentops/lib/mock-streams";

const liveAgentOpsApiClient = new AgentOpsApiClient();

export function agentOpsDataSource(): AgentOpsDataSource {
  return isAgentOpsDemo() ? mockAgentOpsApiClient : liveAgentOpsApiClient;
}
