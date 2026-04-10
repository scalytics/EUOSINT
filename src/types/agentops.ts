export type AgentOpsMode = "OSINT" | "AGENTOPS" | "HYBRID";

export interface AgentOpsPointer {
  bucket: string;
  key: string;
  size: number;
  sha256: string;
  content_type?: string;
  created_at?: string;
  proxy_id?: string;
  path: string;
}

export interface AgentOpsMessage {
  id: string;
  topic: string;
  topic_family: string;
  partition: number;
  offset: number;
  timestamp: string;
  envelope_type?: string;
  sender_id?: string;
  correlation_id?: string;
  trace_id?: string;
  task_id?: string;
  parent_task_id?: string;
  status?: string;
  preview?: string;
  content?: string;
  lfs?: AgentOpsPointer;
}

export interface AgentOpsFlow {
  id: string;
  topic_count: number;
  sender_count: number;
  topics: string[];
  senders: string[];
  trace_ids: string[];
  task_ids: string[];
  first_seen: string;
  last_seen: string;
  latest_status?: string;
  message_count: number;
  latest_preview?: string;
}

export interface AgentOpsTrace {
  id: string;
  span_count: number;
  agents: string[];
  span_types: string[];
  latest_title?: string;
  started_at?: string;
  ended_at?: string;
  duration_ms?: number;
}

export interface AgentOpsTask {
  id: string;
  parent_task_id?: string;
  delegation_depth?: number;
  requester_id?: string;
  responder_id?: string;
  original_requester_id?: string;
  status?: string;
  description?: string;
  last_summary?: string;
  first_seen: string;
  last_seen: string;
}

export interface AgentOpsTopicHealth {
  topic: string;
  messages_per_hour: number;
  active_agents: number;
  is_stale: boolean;
  last_message_at?: string;
}

export interface AgentOpsHealth {
  connected: boolean;
  effective_topics: string[];
  group_id: string;
  accepted_count: number;
  rejected_count: number;
  mirrored_count: number;
  rejected_by_reason: Record<string, number>;
  last_reject?: string;
  last_poll_at?: string;
  topic_health: AgentOpsTopicHealth[];
}

export interface AgentOpsReplaySession {
  id: string;
  group_id: string;
  status: string;
  started_at: string;
  finished_at?: string;
  message_count: number;
}

export interface AgentOpsState {
  generated_at: string;
  enabled: boolean;
  ui_mode: AgentOpsMode;
  profile: string;
  group_name: string;
  topics: string[];
  flow_count: number;
  trace_count: number;
  task_count: number;
  message_count: number;
  health: AgentOpsHealth;
  replay_sessions: AgentOpsReplaySession[];
  flows: AgentOpsFlow[];
  traces: AgentOpsTrace[];
  tasks: AgentOpsTask[];
  messages: AgentOpsMessage[];
}
