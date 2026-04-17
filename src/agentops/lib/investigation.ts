import type { AgentOpsFlow, AgentOpsHealth, AgentOpsMessage, AgentOpsTask, AgentOpsTrace } from "@/agentops/types";

const GAP_THRESHOLD_MS = 90_000;

export interface RunAnomaly {
  id: string;
  label: string;
  severity: "info" | "medium" | "high";
  detail: string;
}

export interface RunSummary {
  title: string;
  durationLabel: string;
  participantCount: number;
  requestCount: number;
  responseCount: number;
  anomalyCount: number;
  replayable: boolean;
  confidence: "complete" | "partial" | "malformed";
  anomalies: RunAnomaly[];
}

export interface QueueSection {
  key: string;
  title: string;
  flowIds: string[];
}

export interface TimelineItem {
  id: string;
  kind: "event" | "gap";
  at: string;
  title: string;
  detail: string;
  sender?: string;
  family?: string;
  status?: string;
}

export function buildRunSummary(
  flow: AgentOpsFlow | null,
  messages: AgentOpsMessage[],
  tasks: AgentOpsTask[],
  traces: AgentOpsTrace[],
  health: AgentOpsHealth,
): RunSummary {
  if (!flow) {
    return {
      title: "No run selected",
      durationLabel: "-",
      participantCount: 0,
      requestCount: 0,
      responseCount: 0,
      anomalyCount: 0,
      replayable: false,
      confidence: "partial",
      anomalies: [],
    };
  }

  const requestCount = messages.filter((message) => message.topic_family === "requests").length;
  const responseCount = messages.filter((message) => message.topic_family === "responses").length;
  const flowTasks = tasks.filter((task) => flow.task_ids.includes(task.id));
  const flowTraces = traces.filter((trace) => flow.trace_ids.includes(trace.id));
  const anomalies = deriveAnomalies(flow, messages, flowTasks, flowTraces, health, requestCount, responseCount);
  const confidence: RunSummary["confidence"] =
    anomalies.some((item) => item.id === "malformed")
      ? "malformed"
      : anomalies.length > 0
        ? "partial"
        : "complete";

  return {
    title: flowTasks[0]?.description || flow.latest_preview || flowTraces[0]?.latest_title || flow.id,
    durationLabel: formatDuration(flow.first_seen, flow.last_seen),
    participantCount: flow.senders.length,
    requestCount,
    responseCount,
    anomalyCount: anomalies.length,
    replayable: flow.message_count > 0,
    confidence,
    anomalies,
  };
}

export function buildConversationTimeline(
  flow: AgentOpsFlow | null,
  messages: AgentOpsMessage[],
  tasks: AgentOpsTask[],
  traces: AgentOpsTrace[],
): TimelineItem[] {
  if (!flow) return [];
  const taskSet = new Set(flow.task_ids);
  const traceSet = new Set(flow.trace_ids);
  const messageItems: TimelineItem[] = messages.map((message) => ({
    id: message.id,
    kind: "event",
    at: message.timestamp,
    title: titleForMessage(message),
    detail: message.preview || message.content || "No preview available",
    sender: message.sender_id,
    family: message.topic_family,
    status: message.status,
  }));
  const taskItems: TimelineItem[] = tasks
    .filter((task) => taskSet.has(task.id))
    .map((task) => ({
      id: `task-${task.id}`,
      kind: "event",
      at: task.last_seen,
      title: `Task ${task.status || "update"}`,
      detail: task.last_summary || task.description || task.id,
      sender: task.responder_id || task.requester_id,
      family: "tasks.status",
      status: task.status,
    }));
  const traceItems: TimelineItem[] = traces
    .filter((trace) => traceSet.has(trace.id))
    .map((trace) => ({
      id: `trace-${trace.id}`,
      kind: "event",
      at: trace.started_at || trace.ended_at || "",
      title: "Trace chain",
      detail: `${trace.latest_title || trace.id} · ${trace.span_count} spans`,
      sender: trace.agents[0],
      family: "traces",
    }));

  const ordered = [...messageItems, ...taskItems, ...traceItems]
    .filter((item) => item.at)
    .sort((left, right) => Date.parse(left.at) - Date.parse(right.at));

  const out: TimelineItem[] = [];
  for (let i = 0; i < ordered.length; i += 1) {
    const current = ordered[i];
    if (i > 0) {
      const previous = ordered[i - 1];
      const gap = Date.parse(current.at) - Date.parse(previous.at);
      if (gap > GAP_THRESHOLD_MS) {
        out.push({
          id: `gap-${previous.id}-${current.id}`,
          kind: "gap",
          at: current.at,
          title: "Dead air",
          detail: `${Math.round(gap / 60000)} minutes without communication`,
        });
      }
    }
    out.push(current);
  }
  return out;
}

export function sortFlowsForQueue(
  flows: AgentOpsFlow[],
  messages: AgentOpsMessage[],
  tasks: AgentOpsTask[],
  traces: AgentOpsTrace[],
  health: AgentOpsHealth,
): AgentOpsFlow[] {
  return [...flows].sort((left, right) => {
    const leftSummary = buildRunSummary(left, relatedMessages(left, messages), tasks, traces, health);
    const rightSummary = buildRunSummary(right, relatedMessages(right, messages), tasks, traces, health);
    return (
      compareStatus(left.latest_status, right.latest_status) ||
      rightSummary.anomalyCount - leftSummary.anomalyCount ||
      Date.parse(right.last_seen) - Date.parse(left.last_seen)
    );
  });
}

export function groupRunsForQueue(
  flows: AgentOpsFlow[],
  messages: AgentOpsMessage[],
  tasks: AgentOpsTask[],
  traces: AgentOpsTrace[],
  health: AgentOpsHealth,
): QueueSection[] {
  const sections: QueueSection[] = [
    { key: "attention", title: "Needs Attention", flowIds: [] },
    { key: "active", title: "Active", flowIds: [] },
    { key: "completed", title: "Completed", flowIds: [] },
  ];
  for (const flow of flows) {
    const summary = buildRunSummary(flow, relatedMessages(flow, messages), tasks, traces, health);
    const bucket =
      summary.anomalyCount > 0
        ? sections[0]
        : /completed|done/i.test(flow.latest_status || "")
          ? sections[2]
          : sections[1];
    bucket.flowIds.push(flow.id);
  }
  return sections.filter((section) => section.flowIds.length > 0);
}

function relatedMessages(flow: AgentOpsFlow, messages: AgentOpsMessage[]): AgentOpsMessage[] {
  return messages.filter((message) => message.correlation_id === flow.id);
}

function deriveAnomalies(
  flow: AgentOpsFlow,
  messages: AgentOpsMessage[],
  tasks: AgentOpsTask[],
  _traces: AgentOpsTrace[],
  health: AgentOpsHealth,
  requestCount: number,
  responseCount: number,
): RunAnomaly[] {
  const anomalies: RunAnomaly[] = [];
  if (requestCount > responseCount) {
    anomalies.push({
      id: "missing-response",
      label: "Missing response",
      severity: "high",
      detail: `Requests exceed responses (${requestCount}/${responseCount}).`,
    });
  }
  if (tasks.some((task) => task.status === "in_progress")) {
    anomalies.push({
      id: "stalled-task",
      label: "Task still in progress",
      severity: "medium",
      detail: "The latest task state remains in_progress.",
    });
  }
  if (health.rejected_count > 0) {
    anomalies.push({
      id: "malformed",
      label: "Rejected transport records",
      severity: "medium",
      detail: `${health.rejected_count} rejected records observed by the tracker.`,
    });
  }
  if (flow.message_count === 0) {
    anomalies.push({
      id: "empty-run",
      label: "Empty run",
      severity: "info",
      detail: "The run exists without message content.",
    });
  }
  return anomalies;
}

function titleForMessage(message: AgentOpsMessage): string {
  switch (message.topic_family) {
    case "requests":
      return "Request dispatched";
    case "responses":
      return "Response returned";
    case "traces":
      return "Trace note";
    case "tasks.status":
      return "Task status updated";
    case "observe.audit":
      return "Audit event";
    default:
      return message.topic_family || "Message";
  }
}

function formatDuration(start: string, end: string): string {
  const startMs = Date.parse(start);
  const endMs = Date.parse(end);
  if (Number.isNaN(startMs) || Number.isNaN(endMs) || endMs < startMs) return "-";
  const minutes = Math.floor((endMs - startMs) / 60000);
  if (minutes < 1) return "<1m";
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remainder = minutes % 60;
  return remainder > 0 ? `${hours}h ${remainder}m` : `${hours}h`;
}

function compareStatus(left: string | undefined, right: string | undefined): number {
  const rank = (value: string | undefined): number => {
    switch ((value || "").toLowerCase()) {
      case "failed":
      case "error":
        return 0;
      case "in_progress":
      case "running":
        return 1;
      case "completed":
      case "done":
        return 2;
      default:
        return 3;
    }
  };
  return rank(left) - rank(right);
}
