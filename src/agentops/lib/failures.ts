import type { AgentOpsFlow, AgentOpsHealth, AgentOpsMessage, AgentOpsTask } from "@/agentops/types";

export interface FailureBucket {
  id: string;
  title: string;
  severity: "info" | "medium" | "high";
  count: number;
  detail: string;
  messageId?: string;
}

export function buildFailureBuckets(
  flow: AgentOpsFlow | null,
  messages: AgentOpsMessage[],
  tasks: AgentOpsTask[],
  health: AgentOpsHealth,
): FailureBucket[] {
  if (!flow) return [];

  const buckets: FailureBucket[] = [];
  const requestCount = messages.filter((message) => message.topic_family === "requests").length;
  const responseCount = messages.filter((message) => message.topic_family === "responses").length;

  if (requestCount > responseCount) {
    buckets.push({
      id: "missing-response",
      title: "Missing response chain",
      severity: "high",
      count: requestCount - responseCount,
      detail: "The selected run has more request events than response events.",
      messageId: messages.find((message) => message.topic_family === "requests")?.id,
    });
  }

  const stalledTasks = tasks.filter((task) => task.status === "in_progress");
  if (stalledTasks.length > 0) {
    buckets.push({
      id: "stalled-task",
      title: "Stalled tasks",
      severity: "medium",
      count: stalledTasks.length,
      detail: "Task state remains in progress without a terminal completion signal.",
    });
  }

  if (health.rejected_count > 0) {
    buckets.push({
      id: "rejected-records",
      title: "Rejected records",
      severity: "medium",
      count: health.rejected_count,
      detail: health.last_reject || "Malformed or unsupported records were rejected by the tracker.",
    });
  }

  if (health.mirrored_count > 0) {
    buckets.push({
      id: "mirrored-rejects",
      title: "Mirrored rejects",
      severity: "info",
      count: health.mirrored_count,
      detail: "Rejected records were mirrored to the reject topic for later inspection.",
    });
  }

  return buckets;
}
