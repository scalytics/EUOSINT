import { useEffect, useMemo, useState, useTransition } from "react";
import { Activity, AlertTriangle, CheckCircle2, GitBranch, Globe2, PlayCircle, RadioTower, ShieldAlert, TimerReset, Workflow } from "lucide-react";
import { EmptyState, MetricCard, Panel, StatusRow, Tag } from "@/agentops/components/Chrome";
import { MessageCard } from "@/agentops/components/MessageCard";
import { RuntimeMap } from "@/agentops/components/RuntimeMap";
import { buildFailureBuckets } from "@/agentops/lib/failures";
import { buildFusionMatches } from "@/agentops/lib/hybrid";
import { buildConversationTimeline, buildRunSummary } from "@/agentops/lib/investigation";
import { loadAnomaliesOnly, loadQueueFilter, loadSelectedRunId, persistAnomaliesOnly, persistQueueFilter, persistSelectedRunId, type RunQueueFilter } from "@/agentops/lib/preferences";
import { displayModeName } from "@/agentops/lib/state";
import { formatTime } from "@/agentops/lib/view";
import { useAgentOpsOperator } from "@/hooks/useAgentOpsOperator";
import {
  useFlow,
  useFlowMessages,
  useFlowTasks,
  useFlowTraces,
  useFlows,
  useHealth,
  useMapFeatures,
  useMapLayers,
  useReplaySessions,
  useTopicHealth,
} from "@/hooks/useAgentOpsApi";
import { useAlerts } from "@/hooks/useAlerts";
import type { AgentOpsMode, ReplaySession } from "@/agentops/types";
import { AgentOpsApiClient } from "@/agentops/lib/api-client";

const api = new AgentOpsApiClient();

interface Props {
  mode: AgentOpsMode;
}

function anomalyHint(status?: string, messageCount = 0): boolean {
  if (messageCount === 0) return true;
  return /(failed|error|rejected|timeout|stalled)/i.test(status || "");
}

export function AgentOpsRuntimeDesk({ mode }: Props) {
  const modeName = displayModeName(mode);
  const operator = useAgentOpsOperator(mode !== "OSINT");
  const { alerts } = useAlerts();
  const [selectedFlowId, setSelectedFlowId] = useState<string | null>(() => loadSelectedRunId());
  const [queueFilter, setQueueFilter] = useState<RunQueueFilter>(() => loadQueueFilter());
  const [anomaliesOnly, setAnomaliesOnly] = useState<boolean>(() => loadAnomaliesOnly());
  const [workspaceTab, setWorkspaceTab] = useState<"replay" | "failures" | "raw" | "operator">("replay");
  const [selectedMessageId, setSelectedMessageId] = useState<string | null>(null);
  const [replayScope, setReplayScope] = useState<"run" | "trace" | "task" | "topics">("run");
  const [replayNotice, setReplayNotice] = useState("");
  const [optimisticReplaySessions, setOptimisticReplaySessions] = useState<ReplaySession[]>([]);
  const [mapBBox, setMapBBox] = useState("14.40,35.80,14.60,36.00");
  const [mapTypes, setMapTypes] = useState<string[]>([]);
  const [isPending, startTransition] = useTransition();

  const { flows } = useFlows({ limit: 50, status: queueFilter === "all" || queueFilter === "attention" ? undefined : queueFilter });
  const { health } = useHealth();
  const { topicHealth } = useTopicHealth();
  const { replaySessions } = useReplaySessions();
  const { mapLayers } = useMapLayers();
  const { featureCollection } = useMapFeatures({ bbox: mapBBox, types: mapTypes.length > 0 ? mapTypes.join(",") : undefined });

  useEffect(() => {
    setOptimisticReplaySessions(replaySessions);
  }, [replaySessions]);

  useEffect(() => {
    if (replayNotice) {
      const timer = window.setTimeout(() => setReplayNotice(""), 3200);
      return () => window.clearTimeout(timer);
    }
    return undefined;
  }, [replayNotice]);

  useEffect(() => {
    if (flows.length === 0) return;
    if (!selectedFlowId || !flows.some((flow) => flow.id === selectedFlowId)) {
      const next = flows[0]?.id ?? null;
      setSelectedFlowId(next);
      persistSelectedRunId(next);
    }
  }, [flows, selectedFlowId]);

  const { flow: selectedFlow } = useFlow(selectedFlowId);
  const { messages: relatedMessages } = useFlowMessages(selectedFlowId, { limit: 100 });
  const { tasks: relatedTasks } = useFlowTasks(selectedFlowId);
  const { traces: relatedTraces } = useFlowTraces(selectedFlowId);

  useEffect(() => {
    setSelectedMessageId((current) => current ?? relatedMessages[0]?.id ?? null);
  }, [relatedMessages]);

  const runSummary = useMemo(
    () => buildRunSummary(selectedFlow, relatedMessages, relatedTasks, relatedTraces, health ?? {
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
    }),
    [health, relatedMessages, relatedTasks, relatedTraces, selectedFlow],
  );
  const timeline = useMemo(() => buildConversationTimeline(selectedFlow, relatedMessages, relatedTasks, relatedTraces), [relatedMessages, relatedTasks, relatedTraces, selectedFlow]);
  const failureBuckets = useMemo(() => buildFailureBuckets(selectedFlow, relatedMessages, relatedTasks, health ?? {
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
  }), [health, relatedMessages, relatedTasks, selectedFlow]);
  const selectedMessage = useMemo(() => relatedMessages.find((message) => message.id === selectedMessageId) ?? relatedMessages[0] ?? null, [relatedMessages, selectedMessageId]);
  const fusionMatches = useMemo(() => (mode === "HYBRID" ? buildFusionMatches(selectedFlow, relatedMessages, alerts) : []), [alerts, mode, relatedMessages, selectedFlow]);

  const queueFlows = useMemo(
    () =>
      flows.filter((flow) => {
        if (queueFilter === "completed" && !/completed|done/i.test(flow.latest_status || "")) return false;
        if (queueFilter === "active" && /completed|done/i.test(flow.latest_status || "")) return false;
        if (queueFilter === "attention" && !anomalyHint(flow.latest_status, flow.message_count)) return false;
        if (anomaliesOnly && !anomalyHint(flow.latest_status, flow.message_count)) return false;
        return true;
      }),
    [anomaliesOnly, flows, queueFilter],
  );

  const queueSections = useMemo(
    () => [
      {
        key: "attention",
        title: "Needs Attention",
        items: queueFlows.filter((flow) => anomalyHint(flow.latest_status, flow.message_count)),
      },
      {
        key: "active",
        title: "Active",
        items: queueFlows.filter((flow) => !anomalyHint(flow.latest_status, flow.message_count) && !/completed|done/i.test(flow.latest_status || "")),
      },
      {
        key: "completed",
        title: "Completed",
        items: queueFlows.filter((flow) => /completed|done/i.test(flow.latest_status || "")),
      },
    ].filter((section) => section.items.length > 0),
    [queueFlows],
  );

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

  function triggerReplay() {
    startTransition(() => {
      const optimistic: ReplaySession = {
        id: `replay-${Date.now()}`,
        group_id: `${health?.group_id || "agentops"}-replay`,
        status: "starting",
        started_at: new Date().toISOString(),
        message_count: relatedMessages.length,
        topics: selectedFlow?.topics ?? health?.effective_topics ?? [],
      };
      setOptimisticReplaySessions((current) => [optimistic, ...current]);
      setReplayNotice("Replay request queued.");
      void api
        .createReplayRequest(selectedFlow?.topics ?? [], {})
        .then(() => setReplayNotice("Replay accepted."))
        .catch(() => {
          setOptimisticReplaySessions((current) =>
            current.map((item) => (item.id === optimistic.id ? { ...item, status: "failed", last_error: "replay request failed" } : item)),
          );
          setReplayNotice("Replay request failed.");
        });
    });
  }

  const topicCount = health?.effective_topics.length ?? 0;

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
                <h1 className="text-3xl font-semibold tracking-[0.04em]">{health?.group_id || "AgentOps"}</h1>
                <p className="mt-2 max-w-3xl text-sm text-siem-muted">{modeName}-mode workflow over typed `/api/v1` resources with queue, graph, replay, and geospatial surfaces.</p>
              </div>
            </div>
            <div className="flex flex-wrap gap-3">
              <MetricCard icon={RadioTower} label="Topics" value={String(topicCount)} hint={health?.connected ? "tracking live" : "disconnected"} />
              <MetricCard icon={GitBranch} label="Flows" value={String(flows.length)} hint={`${relatedTraces.length} traces selected`} />
              <MetricCard icon={ShieldAlert} label="Messages" value={String(relatedMessages.length)} hint={`${health?.rejected_count ?? 0} rejected`} />
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
                    className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${queueFilter === filter ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"}`}
                  >
                    {filter}
                  </button>
                ))}
                <button
                  type="button"
                  onClick={toggleAnomaliesOnly}
                  className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${anomaliesOnly ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"}`}
                >
                  anomalies only
                </button>
              </div>
              {queueSections.length === 0 ? <EmptyState text="No runs match the current queue filter." /> : null}
              {queueSections.map((section) => (
                <div key={section.key} className="space-y-3">
                  <div className="text-[11px] uppercase tracking-[0.22em] text-siem-muted">{section.title}</div>
                  {section.items.map((flow) => {
                    const active = selectedFlow?.id === flow.id;
                    return (
                      <button
                        key={flow.id}
                        type="button"
                        onClick={() => selectFlow(flow.id)}
                        className={`w-full rounded-2xl border p-3 text-left transition ${active ? "border-siem-accent/45 bg-siem-accent/12" : "border-siem-border bg-siem-panel/55 hover:border-siem-accent/25 hover:bg-siem-panel-strong"}`}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <div className="truncate font-semibold">{flow.latest_preview || flow.id}</div>
                            <div className="mt-1 font-mono text-[11px] text-siem-muted">{flow.id}</div>
                            <div className="mt-2 text-xs text-siem-muted">{flow.topics.join(", ") || "No topics declared"}</div>
                          </div>
                          <span className="rounded-full border border-siem-border px-2 py-1 text-[11px] uppercase tracking-[0.18em] text-siem-muted">{flow.latest_status || "active"}</span>
                        </div>
                        <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                          <Tag>{flow.sender_count} participants</Tag>
                          <Tag>{flow.message_count} messages</Tag>
                          <Tag>{formatTime(flow.last_seen)}</Tag>
                          {anomalyHint(flow.latest_status, flow.message_count) ? <Tag>attention</Tag> : null}
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
                  {mode === "HYBRID" && fusionMatches.length > 0 ? (
                    fusionMatches.slice(0, 3).map((match) => (
                      <div key={match.alert_id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <div className="font-semibold">{match.title}</div>
                            <div className="mt-1 text-xs text-siem-muted">{match.source}</div>
                          </div>
                          <Tag>{match.severity}</Tag>
                        </div>
                        <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                          {match.match_reasons.map((reason) => <Tag key={reason}>{reason}</Tag>)}
                        </div>
                      </div>
                    ))
                  ) : null}
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
                        </div>
                      </button>
                    ),
                  )}
                </>
              ) : (
                <EmptyState text="No flow selected yet." />
              )}
            </div>
          </Panel>

          <div className="grid min-h-0 gap-4 overflow-hidden">
            <Panel title={mode === "HYBRID" ? "External Intel Context" : "Run Context"} icon={ShieldAlert} bodyClassName="overflow-y-auto pr-1">
              <div className="grid gap-4">
                <div className="grid gap-3">
                  <StatusRow label="Tracking group" value={health?.group_id || "unconfigured"} />
                  <StatusRow label="Connected" value={health?.connected ? "yes" : "no"} />
                  <StatusRow label="Accepted" value={String(health?.accepted_count ?? 0)} />
                  <StatusRow label="Rejected" value={String(health?.rejected_count ?? 0)} />
                  <StatusRow label="Mirrored" value={String(health?.mirrored_count ?? 0)} />
                  {mode === "HYBRID" ? <StatusRow label="Fusion matches" value={String(fusionMatches.length)} /> : null}
                  <StatusRow label="Last poll" value={formatTime(health?.last_poll_at || "")} />
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
                        {selectedFlow.senders.map((sender) => <Tag key={sender}>{sender}</Tag>)}
                      </div>
                    </div>
                  </>
                ) : null}
              </div>
            </Panel>

            <Panel title="Topic Health" icon={RadioTower} bodyClassName="overflow-y-auto pr-1">
              <div className="space-y-2">
                {topicHealth.map((topic) => (
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
                {topicHealth.length === 0 ? <EmptyState text="No topic health yet." /> : null}
              </div>
            </Panel>

            <Panel title="Replay Panel" icon={TimerReset} bodyClassName="overflow-y-auto pr-1">
              <div className="space-y-2">
                {(optimisticReplaySessions.length === 0 ? [{ id: "none", status: "idle", group_id: "", started_at: "", message_count: 0 }] : optimisticReplaySessions).map((session) => (
                  <div key={session.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="text-xs font-semibold">{session.id === "none" ? "No replay sessions yet" : session.group_id}</div>
                      <Tag>{session.status}</Tag>
                    </div>
                    {session.id !== "none" ? <div className="mt-2 text-[11px] text-siem-muted">Started {formatTime(session.started_at)} · {session.message_count} messages</div> : null}
                  </div>
                ))}
              </div>
            </Panel>
          </div>
        </section>

        <section className="grid min-h-0 flex-[1.05] gap-4 overflow-hidden xl:grid-cols-[1.1fr_1fr_1fr]">
          <Panel title="Message Detail" icon={RadioTower} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-3">
              {relatedMessages.map((message) => (
                <button key={message.id} type="button" onClick={() => { setSelectedMessageId(message.id); setWorkspaceTab("raw"); }} className={`block w-full text-left ${selectedMessageId === message.id ? "rounded-[18px] ring-1 ring-siem-accent/45" : ""}`}>
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
                  <button key={id} type="button" onClick={() => setWorkspaceTab(id as typeof workspaceTab)} className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${workspaceTab === id ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"}`}>
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
                        <button key={scope} type="button" onClick={() => setReplayScope(scope)} className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em] ${replayScope === scope ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"}`}>
                          {label}
                        </button>
                      ))}
                    </div>
                    <div className="mt-4 grid gap-2">
                      <StatusRow label="Replay group" value={`${health?.group_id || "agentops"}-replay`} />
                      <StatusRow label="Expected records" value={String(relatedMessages.length)} />
                      <StatusRow label="Scope" value={replayScope} />
                    </div>
                  </div>
                </div>
              ) : null}

              {workspaceTab === "failures" ? (
                <div className="space-y-3">
                  {failureBuckets.length === 0 ? <EmptyState text="No failure buckets for the selected run." /> : failureBuckets.map((bucket) => (
                    <div key={bucket.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <div className="font-semibold">{bucket.title}</div>
                          <div className="mt-1 text-xs text-siem-muted">{bucket.detail}</div>
                        </div>
                        <Tag>{bucket.count}</Tag>
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}

              {workspaceTab === "raw" ? selectedMessage ? (
                <div className="space-y-3">
                  <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                    <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Selected Message</div>
                    <div className="mt-2 font-mono text-xs text-siem-text">{selectedMessage.id}</div>
                    <div className="mt-3 text-xs text-siem-muted">{selectedMessage.topic} · p{selectedMessage.partition} · o{selectedMessage.offset}</div>
                  </div>
                  <MessageCard message={selectedMessage} />
                </div>
              ) : <EmptyState text="Select a timeline or message entry to inspect the raw transport artifact." /> : null}

              {workspaceTab === "operator" ? (
                <>
                  <StatusRow label="Admin surface" value={operator.supported ? "supported" : "limited"} />
                  <StatusRow label="Live group" value={operator.live_group_id || health?.group_id || "-"} />
                  <StatusRow label="Replay groups" value={String(operator.replay_group_ids.length)} />
                  {operator.last_error ? <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3 text-xs">{operator.last_error}</div> : null}
                  {operator.groups.length === 0 ? <EmptyState text="No consumer groups returned yet." /> : operator.groups.slice(0, 8).map((group) => (
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
                  ))}
                </>
              ) : null}
            </div>
          </Panel>

          <Panel title="Map Surface" icon={Globe2} bodyClassName="overflow-y-auto pr-1">
            <RuntimeMap bbox={mapBBox} layers={mapLayers} features={featureCollection} selectedTypes={mapTypes} onBBoxChange={setMapBBox} onTypesChange={setMapTypes} />
          </Panel>
        </section>
      </div>
    </main>
  );
}
