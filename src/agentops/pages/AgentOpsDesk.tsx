import { useEffect, useMemo, useState, useTransition } from "react";
import { Activity, AlertTriangle, CheckCircle2, GitBranch, PlayCircle, RadioTower, ShieldAlert, TimerReset, Workflow } from "lucide-react";
import { EmptyState, MetricCard, Panel, StatusRow, Tag } from "@/agentops/components/Chrome";
import { agentOpsReplayURL } from "@/agentops/lib/demo";
import { buildFailureBuckets } from "@/agentops/lib/failures";
import { MessageCard } from "@/agentops/components/MessageCard";
import { buildFusionMatches } from "@/agentops/lib/hybrid";
import { buildConversationTimeline, buildRunSummary, groupRunsForQueue, sortFlowsForQueue } from "@/agentops/lib/investigation";
import { loadAnomaliesOnly, loadQueueFilter, loadSelectedRunId, persistAnomaliesOnly, persistQueueFilter, persistSelectedRunId, type RunQueueFilter } from "@/agentops/lib/preferences";
import { displayModeName } from "@/agentops/lib/state";
import { formatTime } from "@/agentops/lib/view";
import { useAgentOpsOperator } from "@/hooks/useAgentOpsOperator";
import { useAlerts } from "@/hooks/useAlerts";
import type { AgentOpsMode, AgentOpsState } from "@/agentops/types";

interface Props {
  state: AgentOpsState;
  mode: AgentOpsMode;
}

export function AgentOpsDesk({ state, mode }: Props) {
  const modeName = displayModeName(mode);
  const operator = useAgentOpsOperator(mode !== "OSINT");
  const { alerts } = useAlerts();
  const [selectedFlowId, setSelectedFlowId] = useState<string | null>(() => loadSelectedRunId() ?? state.flows[0]?.id ?? null);
  const [queueFilter, setQueueFilter] = useState<RunQueueFilter>(() => loadQueueFilter());
  const [anomaliesOnly, setAnomaliesOnly] = useState<boolean>(() => loadAnomaliesOnly());
  const [isPending, startTransition] = useTransition();
  const [replayNotice, setReplayNotice] = useState<string>("");
  const [optimisticReplayCount, setOptimisticReplayCount] = useState(0);
  const [optimisticReplaySessions, setOptimisticReplaySessions] = useState(state.replay_sessions);
  const [workspaceTab, setWorkspaceTab] = useState<"replay" | "failures" | "raw" | "operator">("replay");
  const [selectedMessageId, setSelectedMessageId] = useState<string | null>(null);
  const [replayScope, setReplayScope] = useState<"run" | "trace" | "task" | "topics">("run");

  useEffect(() => {
    setOptimisticReplaySessions(state.replay_sessions);
  }, [state.replay_sessions]);

  useEffect(() => {
    if (!state.flows.some((flow) => flow.id === selectedFlowId)) {
      const next = sortFlowsForQueue(state.flows, state.messages, state.tasks, state.traces, state.health)[0]?.id ?? null;
      setSelectedFlowId(next);
      persistSelectedRunId(next);
    }
  }, [selectedFlowId, state.flows, state.messages, state.tasks, state.traces, state.health]);

  useEffect(() => {
    if (replayNotice) {
      const id = window.setTimeout(() => setReplayNotice(""), 3200);
      return () => window.clearTimeout(id);
    }
    return undefined;
  }, [replayNotice]);
  const selectedFlow = useMemo(
    () => state.flows.find((flow) => flow.id === selectedFlowId) ?? state.flows[0] ?? null,
    [selectedFlowId, state.flows],
  );
  const queueFlows = useMemo(() => sortFlowsForQueue(state.flows, state.messages, state.tasks, state.traces, state.health), [state.flows, state.messages, state.tasks, state.traces, state.health]);
  const queueSections = useMemo(() => {
    const filtered = queueFlows.filter((flow) => {
      const flowMessages = state.messages.filter((message) => message.correlation_id === flow.id);
      const flowTasks = state.tasks.filter((task) => flow.task_ids.includes(task.id));
      const flowTraces = state.traces.filter((trace) => flow.trace_ids.includes(trace.id));
      const summary = buildRunSummary(flow, flowMessages, flowTasks, flowTraces, state.health);
      if (anomaliesOnly && summary.anomalyCount === 0) return false;
      switch (queueFilter) {
        case "active":
          return !/completed|done/i.test(flow.latest_status || "");
        case "completed":
          return /completed|done/i.test(flow.latest_status || "");
        case "attention":
          return summary.anomalyCount > 0;
        case "all":
        default:
          return true;
      }
    });
    return groupRunsForQueue(filtered, state.messages, state.tasks, state.traces, state.health);
  }, [anomaliesOnly, queueFilter, queueFlows, state.health, state.messages, state.tasks, state.traces]);
  const relatedMessages = useMemo(() => {
    if (!selectedFlow) return state.messages.slice(0, 16);
    return state.messages.filter((message) => message.correlation_id === selectedFlow.id).slice(0, 24);
  }, [selectedFlow, state.messages]);
  useEffect(() => {
    setSelectedMessageId((current) => current ?? relatedMessages[0]?.id ?? null);
  }, [relatedMessages]);
  const relatedTraces = useMemo(() => {
    if (!selectedFlow) return state.traces.slice(0, 8);
    const traceSet = new Set(selectedFlow.trace_ids);
    return state.traces.filter((trace) => traceSet.has(trace.id)).slice(0, 8);
  }, [selectedFlow, state.traces]);
  const relatedTasks = useMemo(() => {
    if (!selectedFlow) return state.tasks.slice(0, 8);
    const taskSet = new Set(selectedFlow.task_ids);
    return state.tasks.filter((task) => taskSet.has(task.id)).slice(0, 8);
  }, [selectedFlow, state.tasks]);
  const runSummary = useMemo(() => buildRunSummary(selectedFlow, relatedMessages, relatedTasks, relatedTraces, state.health), [selectedFlow, relatedMessages, relatedTasks, relatedTraces, state.health]);
  const timeline = useMemo(() => buildConversationTimeline(selectedFlow, relatedMessages, relatedTasks, relatedTraces), [selectedFlow, relatedMessages, relatedTasks, relatedTraces]);
  const failureBuckets = useMemo(() => buildFailureBuckets(selectedFlow, relatedMessages, relatedTasks, state.health), [selectedFlow, relatedMessages, relatedTasks, state.health]);
  const selectedMessage = useMemo(() => relatedMessages.find((message) => message.id === selectedMessageId) ?? relatedMessages[0] ?? null, [relatedMessages, selectedMessageId]);
  const fusionMatches = useMemo(() => (mode === "HYBRID" ? buildFusionMatches(selectedFlow, state.messages, alerts) : []), [alerts, mode, selectedFlow, state.messages]);

  function triggerReplay() {
    startTransition(() => {
      const nextCount = optimisticReplayCount + 1;
      const sessionId = `demo-replay-${nextCount}`;
      const groupId = `${state.health.group_id || "agentops"}-replay-${nextCount}`;
      setOptimisticReplayCount(nextCount);
      setOptimisticReplaySessions((current) => [
        {
          id: sessionId,
          group_id: groupId,
          status: "starting",
          started_at: new Date().toISOString(),
          message_count: 0,
          topics: selectedFlow?.topics ?? state.topics,
        },
        ...current,
      ]);
      setReplayNotice("Replay request queued.");
      void fetch(agentOpsReplayURL(), { method: "POST" })
        .then(async (response) => {
          if (!response.ok) {
            throw new Error(`replay failed: ${response.status}`);
          }
          setOptimisticReplaySessions((current) =>
            current.map((session) => (session.id === sessionId ? { ...session, status: "accepted", message_count: relatedMessages.length } : session)),
          );
          setReplayNotice("Replay accepted.");
        })
        .catch(() => {
          setOptimisticReplaySessions((current) =>
            current.map((session) => (session.id === sessionId ? { ...session, status: "failed", last_error: "replay request failed" } : session)),
          );
          setReplayNotice("Replay request failed.");
        });
    });
  }

  function selectFlow(id: string) {
    setSelectedFlowId(id);
    persistSelectedRunId(id);
    setWorkspaceTab("replay");
  }

  function updateQueueFilter(filter: RunQueueFilter) {
    setQueueFilter(filter);
    persistQueueFilter(filter);
  }

  function toggleAnomaliesOnly() {
    setAnomaliesOnly((current) => {
      const next = !current;
      persistAnomaliesOnly(next);
      return next;
    });
  }

  return (
    <main className="h-dvh overflow-hidden bg-siem-bg text-siem-text">
      <div className="mx-auto flex h-full max-w-[1800px] flex-col gap-5 overflow-hidden px-4 py-5 md:px-6">
        <section className="rounded-[28px] border border-siem-border bg-[linear-gradient(135deg,rgba(24,40,53,0.94),rgba(5,10,17,0.96))] p-5 shadow-[0_22px_70px_rgba(0,0,0,0.34)]">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="space-y-3">
              <div className="inline-flex items-center gap-2 rounded-full border border-siem-accent/30 bg-siem-accent/10 px-3 py-1 text-[11px] uppercase tracking-[0.24em] text-siem-accent">
                <Workflow size={12} />
                {mode === "HYBRID" ? "Fusion Desk" : "Operations Desk"}
              </div>
              <div>
                <h1 className="text-3xl font-semibold tracking-[0.04em]">{state.group_name || "AgentOps"}</h1>
                <p className="mt-2 max-w-3xl text-sm text-siem-muted">
                  {modeName}-mode workflow over KafClaw topics. Normal messages are decoded from Kafka values. LFS-backed
                  records stay pointer-only and are surfaced as S3 paths.
                </p>
              </div>
            </div>
            <div className="flex flex-wrap gap-3">
              <MetricCard icon={RadioTower} label="Topics" value={String(state.topics.length)} hint={state.health.connected ? "tracking live" : "disconnected"} />
              <MetricCard icon={GitBranch} label="Flows" value={String(state.flow_count)} hint={`${state.trace_count} traces`} />
              <MetricCard icon={ShieldAlert} label="Messages" value={String(state.message_count)} hint={`${state.health.rejected_count} rejected`} />
              <button
                type="button"
                onClick={() => void triggerReplay()}
                className="inline-flex min-w-[180px] items-center justify-between rounded-2xl border border-siem-accent/30 bg-siem-accent/12 px-4 py-3 text-left transition hover:border-siem-accent/50 hover:bg-siem-accent/18"
              >
                <span>
                  <span className="block text-[11px] uppercase tracking-[0.22em] text-siem-muted">Replay</span>
                  <span className="mt-1 block text-sm font-semibold text-siem-text">Start from earliest</span>
                </span>
                {isPending ? <TimerReset size={18} className="animate-spin text-siem-accent" /> : <PlayCircle size={18} className="text-siem-accent" />}
              </button>
            </div>
          </div>
          {replayNotice ? (
            <div className="mt-4 inline-flex items-center gap-2 rounded-2xl border border-siem-accent/30 bg-siem-accent/10 px-3 py-2 text-sm text-siem-text">
              <CheckCircle2 size={15} className="text-siem-accent" />
              {replayNotice}
            </div>
          ) : null}
        </section>

        <section className="grid min-h-0 flex-1 gap-4 overflow-hidden xl:grid-cols-[1.1fr_1fr_0.95fr]">
          <Panel title={mode === "HYBRID" ? "Fusion Queue" : "Operations Queue"} icon={Activity} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-4">
              <div className="flex flex-wrap gap-2">
                {(["attention", "active", "completed", "all"] as RunQueueFilter[]).map((filter) => (
                  <button
                    key={filter}
                    type="button"
                    onClick={() => updateQueueFilter(filter)}
                    className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${
                      queueFilter === filter ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"
                    }`}
                  >
                    {filter}
                  </button>
                ))}
                <button
                  type="button"
                  onClick={toggleAnomaliesOnly}
                  className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${
                    anomaliesOnly ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"
                  }`}
                >
                  anomalies only
                </button>
              </div>
              {queueSections.length === 0 ? <EmptyState text="No runs match the current queue filter." /> : null}
              {queueSections.map((section) => (
                <div key={section.key} className="space-y-3">
                  <div className="text-[11px] uppercase tracking-[0.22em] text-siem-muted">{section.title}</div>
                  {section.flowIds.map((flowId) => {
                    const flow = queueFlows.find((entry) => entry.id === flowId);
                    if (!flow) return null;
                const active = selectedFlow?.id === flow.id;
                const flowMessages = state.messages.filter((message) => message.correlation_id === flow.id);
                const flowTasks = state.tasks.filter((task) => flow.task_ids.includes(task.id));
                const flowTraces = state.traces.filter((trace) => flow.trace_ids.includes(trace.id));
                const summary = buildRunSummary(flow, flowMessages, flowTasks, flowTraces, state.health);
                return (
                  <button
                    key={flow.id}
                    type="button"
                    onClick={() => selectFlow(flow.id)}
                    className={`w-full rounded-2xl border p-3 text-left transition ${
                      active
                        ? "border-siem-accent/45 bg-siem-accent/12"
                        : "border-siem-border bg-siem-panel/55 hover:border-siem-accent/25 hover:bg-siem-panel-strong"
                    }`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate font-semibold">{summary.title}</div>
                        <div className="mt-1 font-mono text-[11px] text-siem-muted">{flow.id}</div>
                        <div className="mt-2 text-xs text-siem-muted">{flow.latest_preview || "No decoded preview available"}</div>
                      </div>
                      <span className="rounded-full border border-siem-border px-2 py-1 text-[11px] uppercase tracking-[0.18em] text-siem-muted">
                        {flow.latest_status || "active"}
                      </span>
                    </div>
                    <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                      <Tag>{summary.participantCount} participants</Tag>
                      <Tag>{summary.durationLabel}</Tag>
                      <Tag>{summary.requestCount}/{summary.responseCount} req/resp</Tag>
                      <Tag>{summary.confidence}</Tag>
                      <Tag>{formatTime(flow.last_seen)}</Tag>
                      {summary.anomalyCount > 0 ? <Tag>{summary.anomalyCount} anomalies</Tag> : null}
                      {active ? <Tag>selected</Tag> : null}
                    </div>
                  </button>
                );
                  })}
                </div>
              ))}
            </div>
          </Panel>

          <Panel title={mode === "HYBRID" ? "Fusion Timeline" : "Conversation Timeline"} icon={GitBranch} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-4">
              {selectedFlow ? (
                <>
                  <div className="rounded-2xl border border-siem-border bg-siem-panel/60 p-4">
                    <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Selected run</div>
                    <div className="mt-2 text-lg font-semibold">{runSummary.title}</div>
                    <div className="mt-1 font-mono text-xs text-siem-muted">{selectedFlow.id}</div>
                    <div className="mt-2 grid gap-2 text-sm text-siem-muted">
                      <div>First seen: {formatTime(selectedFlow.first_seen)}</div>
                      <div>Last seen: {formatTime(selectedFlow.last_seen)}</div>
                      <div>Participants: {selectedFlow.senders.join(", ") || "none"}</div>
                      <div>Confidence: {runSummary.confidence}</div>
                    </div>
                  </div>
                  <div className="space-y-3">
                    {mode === "HYBRID" && fusionMatches.length === 0 ? (
                      <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4 text-sm text-siem-muted">
                        No OSINT fusion match for the selected flow. Hybrid stays quiet until an explicit category, geography, sector, vendor/product, CVE, or time-window rule matches.
                      </div>
                    ) : null}
                    {mode === "HYBRID"
                      ? fusionMatches.slice(0, 5).map((match) => (
                          <div key={match.alert_id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                            <div className="flex items-start justify-between gap-3">
                              <div>
                                <div className="font-semibold">{match.title}</div>
                                <div className="mt-1 text-xs text-siem-muted">{match.source}</div>
                              </div>
                              <Tag>{match.severity}</Tag>
                            </div>
                            <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                              {match.match_reasons.map((reason) => (
                                <Tag key={reason}>{reason}</Tag>
                              ))}
                            </div>
                          </div>
                        ))
                      : null}
                    {timeline.map((item) =>
                      item.kind === "gap" ? (
                        <div key={item.id} className="rounded-2xl border border-dashed border-siem-border bg-siem-bg/30 px-4 py-3 text-sm text-siem-muted">
                          <div className="inline-flex items-center gap-2">
                            <AlertTriangle size={14} className="text-siem-accent" />
                            {item.detail}
                          </div>
                        </div>
                      ) : (
                        <button
                          key={item.id}
                          type="button"
                          onClick={() => {
                            if (item.sourceMessageId) {
                              setSelectedMessageId(item.sourceMessageId);
                              setWorkspaceTab("raw");
                            }
                          }}
                          className="block w-full rounded-2xl border border-siem-border bg-siem-panel/55 p-4 text-left"
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <div className="font-semibold">{item.title}</div>
                              <div className="mt-1 text-xs text-siem-muted">{item.detail}</div>
                            </div>
                            <Tag>{item.family || "event"}</Tag>
                          </div>
                          <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                            <Tag>{formatTime(item.at)}</Tag>
                            {item.sender ? <Tag>{item.sender}</Tag> : null}
                            {item.status ? <Tag>{item.status}</Tag> : null}
                            {item.sourceMessageId ? <Tag>open raw</Tag> : null}
                          </div>
                        </button>
                      ),
                    )}
                  </div>
                </>
              ) : (
                <EmptyState text="No AgentOps flow data yet. Once Kafka tracking is enabled and messages are consumed, flows and traces appear here." />
              )}
            </div>
          </Panel>

          <div className="grid min-h-0 gap-4 overflow-hidden">
            <Panel title={mode === "HYBRID" ? "External Intel Context" : "Run Context"} icon={ShieldAlert} bodyClassName="overflow-y-auto pr-1">
              <div className="grid gap-4">
                <div className="grid gap-3">
                  <StatusRow label="Tracking group" value={state.health.group_id || "unconfigured"} />
                  <StatusRow label="Connected" value={state.health.connected ? "yes" : "no"} />
                  <StatusRow label="Accepted" value={String(state.health.accepted_count)} />
                  <StatusRow label="Rejected" value={String(state.health.rejected_count)} />
                  <StatusRow label="Mirrored" value={String(state.health.mirrored_count)} />
                  {mode === "HYBRID" ? <StatusRow label="Fusion matches" value={String(fusionMatches.length)} /> : null}
                  <StatusRow label="Last poll" value={formatTime(state.health.last_poll_at || "")} />
                </div>
                {selectedFlow ? (
                  <>
                    <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                      <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Run Summary</div>
                      <div className="mt-3 grid gap-2 text-sm text-siem-muted">
                        <div>Replayable: {runSummary.replayable ? "yes" : "no"}</div>
                        <div>Duration: {runSummary.durationLabel}</div>
                        <div>Participants: {runSummary.participantCount}</div>
                        <div>Requests / Responses: {runSummary.requestCount} / {runSummary.responseCount}</div>
                        <div>Confidence: {runSummary.confidence}</div>
                      </div>
                    </div>
                    <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                      <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Participants</div>
                      <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                        {selectedFlow.senders.map((sender) => (
                          <Tag key={sender}>{sender}</Tag>
                        ))}
                      </div>
                    </div>
                    <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                      <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Anomalies</div>
                      <div className="mt-3 space-y-2">
                        {runSummary.anomalies.length === 0 ? (
                          <EmptyState text="No run anomalies detected. This run looks complete from the current tracker state." />
                        ) : (
                          runSummary.anomalies.map((anomaly) => (
                            <div key={anomaly.id} className="rounded-2xl border border-siem-border bg-siem-bg/45 p-3">
                              <div className="flex items-start justify-between gap-3">
                                <div>
                                  <div className="font-semibold">{anomaly.label}</div>
                                  <div className="mt-1 text-xs text-siem-muted">{anomaly.detail}</div>
                                </div>
                                <Tag>{anomaly.severity}</Tag>
                              </div>
                            </div>
                          ))
                        )}
                      </div>
                    </div>
                    <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                      <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Replay Scope</div>
                      <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                        {selectedFlow.topics.map((topic) => (
                          <Tag key={topic}>{topic}</Tag>
                        ))}
                      </div>
                    </div>
                  </>
                ) : null}
              </div>
            </Panel>

            <Panel title="Topic Health" icon={RadioTower} bodyClassName="overflow-y-auto pr-1">
              <div className="space-y-2">
                {state.health.topic_health.slice(0, 8).map((topic) => (
                  <div key={topic.topic} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                    <div className="font-mono text-xs text-siem-text">{topic.topic}</div>
                    <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                      <Tag>{topic.active_agents} agents</Tag>
                      <Tag>{topic.messages_per_hour.toFixed(0)} msg/h</Tag>
                      <Tag>{topic.message_density}</Tag>
                      <Tag>{topic.is_stale ? "stale" : "active"}</Tag>
                    </div>
                  </div>
                ))}
                {state.health.topic_health.length === 0 ? <EmptyState text="No topic health yet. Health snapshots appear after the first successful poll." /> : null}
              </div>
            </Panel>

            <Panel title="Replay Panel" icon={TimerReset} bodyClassName="overflow-y-auto pr-1">
              <div className="space-y-2">
                {(optimisticReplaySessions.length === 0
                  ? [{ id: "none", status: "idle", group_id: "", started_at: "", message_count: 0 }]
                  : optimisticReplaySessions
                ).map((session) => (
                  <div key={session.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="text-xs font-semibold">{session.id === "none" ? "No replay sessions yet" : session.group_id}</div>
                      <Tag>{session.status}</Tag>
                    </div>
                    {session.id !== "none" ? (
                      <div className="mt-2 text-[11px] text-siem-muted">
                        Started {formatTime(session.started_at)} · {session.message_count} messages
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            </Panel>
          </div>
        </section>

        <section className="grid min-h-0 flex-[1.05] gap-4 overflow-hidden xl:grid-cols-[1.1fr_1fr]">
          <Panel title="Message Detail" icon={RadioTower} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-3">
              {relatedMessages.map((message) => (
                <button
                  key={message.id}
                  type="button"
                  onClick={() => {
                    setSelectedMessageId(message.id);
                    setWorkspaceTab("raw");
                  }}
                  className={`block w-full text-left ${selectedMessageId === message.id ? "rounded-[18px] ring-1 ring-siem-accent/45" : ""}`}
                >
                  <MessageCard message={message} />
                </button>
              ))}
              {relatedMessages.length === 0 ? <EmptyState text="No decoded messages for the selected flow yet." /> : null}
            </div>
          </Panel>
          <Panel title="Investigation Workspace" icon={TimerReset} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-4 text-sm text-siem-muted">
              <div className="flex flex-wrap gap-2">
                {[
                  ["replay", "Replay Studio"],
                  ["failures", "Failure Workbench"],
                  ["raw", "Raw Messages"],
                  ["operator", "Operator"],
                ].map(([id, label]) => (
                  <button
                    key={id}
                    type="button"
                    onClick={() => setWorkspaceTab(id as typeof workspaceTab)}
                    className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${
                      workspaceTab === id ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>

              {workspaceTab === "replay" ? (
                <div className="space-y-3">
                  <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                    <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Replay Scope</div>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {([
                        ["run", "Full run"],
                        ["trace", "Selected trace"],
                        ["task", "Task chain"],
                        ["topics", "Topic families"],
                      ] as const).map(([scope, label]) => (
                        <button
                          key={scope}
                          type="button"
                          onClick={() => setReplayScope(scope)}
                          className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${
                            replayScope === scope ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"
                          }`}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                    <div className="mt-4 grid gap-2">
                      <StatusRow label="Replay group" value={`${state.health.group_id || "agentops"}-replay-${optimisticReplayCount + 1}`} />
                      <StatusRow label="Expected records" value={String(relatedMessages.length)} />
                      <StatusRow label="Scope" value={replayScope} />
                    </div>
                    <div className="mt-4 rounded-2xl border border-dashed border-siem-border bg-siem-bg/35 p-3 text-xs">
                      Replay is isolated from the live tracking group and scoped from the selected run context.
                    </div>
                  </div>
                  <div className="space-y-2">
                    {(optimisticReplaySessions.length === 0
                      ? [{ id: "none", status: "idle", group_id: "", started_at: "", message_count: 0 }]
                      : optimisticReplaySessions
                    ).map((session) => (
                      <div key={session.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                        <div className="flex items-center justify-between gap-3">
                          <div className="font-semibold">{session.id === "none" ? "No replay sessions yet" : session.group_id}</div>
                          <Tag>{session.status}</Tag>
                        </div>
                        {session.id !== "none" ? (
                          <div className="mt-2 text-[11px] text-siem-muted">
                            {session.message_count} messages · {session.topics?.join(", ") || "all topics"}
                          </div>
                        ) : null}
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}

              {workspaceTab === "failures" ? (
                <div className="space-y-3">
                  {failureBuckets.length === 0 ? (
                    <EmptyState text="No failure buckets for the selected run. Transport and conversation look complete from the current state." />
                  ) : (
                    failureBuckets.map((bucket) => (
                      <div key={bucket.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <div className="font-semibold">{bucket.title}</div>
                            <div className="mt-1 text-xs text-siem-muted">{bucket.detail}</div>
                          </div>
                          <Tag>{bucket.count}</Tag>
                        </div>
                        <div className="mt-3 flex flex-wrap gap-2">
                          <Tag>{bucket.severity}</Tag>
                          {bucket.messageId ? (
                            <button
                              type="button"
                              onClick={() => {
                                setSelectedMessageId(bucket.messageId || null);
                                setWorkspaceTab("raw");
                              }}
                              className="rounded-full border border-siem-border px-2 py-1 text-[11px]"
                            >
                              inspect raw
                            </button>
                          ) : null}
                        </div>
                      </div>
                    ))
                  )}
                </div>
              ) : null}

              {workspaceTab === "raw" ? (
                selectedMessage ? (
                  <div className="space-y-3">
                    <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                      <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Selected Message</div>
                      <div className="mt-2 font-mono text-xs text-siem-text">{selectedMessage.id}</div>
                      <div className="mt-3 text-xs text-siem-muted">
                        {selectedMessage.topic} · p{selectedMessage.partition} · o{selectedMessage.offset}
                      </div>
                    </div>
                    <MessageCard message={selectedMessage} />
                  </div>
                ) : (
                  <EmptyState text="Select a timeline or message entry to inspect the raw transport artifact." />
                )
              ) : null}

              {workspaceTab === "operator" ? (
                <>
                  <StatusRow label="Admin surface" value={operator.supported ? "supported" : "limited"} />
                  <StatusRow label="Live group" value={operator.live_group_id || state.health.group_id || "-"} />
                  <StatusRow label="Replay groups" value={String(operator.replay_group_ids.length)} />
                  {operator.last_error ? <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3 text-xs">{operator.last_error}</div> : null}
                  {operator.groups.length === 0 ? (
                    <EmptyState text="No consumer groups returned yet. When Kafscale group visibility is available, live and replay groups appear here." />
                  ) : (
                    operator.groups.slice(0, 8).map((group) => (
                      <div key={group.group_id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                        <div className="flex items-center justify-between gap-3">
                          <div className="font-mono text-xs text-siem-text">{group.group_id}</div>
                          <Tag>{group.state || "unknown"}</Tag>
                        </div>
                        <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                          <Tag>{group.protocol_type || "consumer"}</Tag>
                          {group.protocol ? <Tag>{group.protocol}</Tag> : null}
                          <Tag>{group.members.length} members</Tag>
                        </div>
                      </div>
                    ))
                  )}
                </>
              ) : null}
            </div>
          </Panel>
        </section>
      </div>
    </main>
  );
}
