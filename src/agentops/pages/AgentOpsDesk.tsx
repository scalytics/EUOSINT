import { useMemo, useState, useTransition } from "react";
import { Activity, GitBranch, PlayCircle, RadioTower, ShieldAlert, TimerReset, Workflow } from "lucide-react";
import { EmptyState, MetricCard, Panel, StatusRow, Tag } from "@/agentops/components/Chrome";
import { MessageCard } from "@/agentops/components/MessageCard";
import { formatTime } from "@/agentops/lib/view";
import type { AgentOpsMode, AgentOpsState } from "@/agentops/types";

interface Props {
  state: AgentOpsState;
  mode: AgentOpsMode;
}

export function AgentOpsDesk({ state, mode }: Props) {
  const [selectedFlowId, setSelectedFlowId] = useState<string | null>(state.flows[0]?.id ?? null);
  const [isPending, startTransition] = useTransition();
  const selectedFlow = useMemo(
    () => state.flows.find((flow) => flow.id === selectedFlowId) ?? state.flows[0] ?? null,
    [selectedFlowId, state.flows],
  );
  const relatedMessages = useMemo(() => {
    if (!selectedFlow) return state.messages.slice(0, 16);
    return state.messages.filter((message) => message.correlation_id === selectedFlow.id).slice(0, 24);
  }, [selectedFlow, state.messages]);
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

  function triggerReplay() {
    startTransition(() => {
      void fetch("/api/agentops/replay", { method: "POST" }).catch(() => undefined);
    });
  }

  return (
    <main className="min-h-screen bg-siem-bg text-siem-text">
      <div className="mx-auto flex max-w-[1800px] flex-col gap-5 px-4 py-5 md:px-6">
        <section className="rounded-[28px] border border-siem-border bg-[linear-gradient(135deg,rgba(24,40,53,0.94),rgba(5,10,17,0.96))] p-5 shadow-[0_22px_70px_rgba(0,0,0,0.34)]">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="space-y-3">
              <div className="inline-flex items-center gap-2 rounded-full border border-siem-accent/30 bg-siem-accent/10 px-3 py-1 text-[11px] uppercase tracking-[0.24em] text-siem-accent">
                <Workflow size={12} />
                {mode === "HYBRID" ? "Fusion Desk" : "Flow Desk"}
              </div>
              <div>
                <h1 className="text-3xl font-semibold tracking-[0.04em]">{state.group_name || "AgentOps"}</h1>
                <p className="mt-2 max-w-3xl text-sm text-siem-muted">
                  Kafka-backed agent flow tracking over KafClaw topics. Normal messages are decoded from Kafka values.
                  LFS-backed records stay pointer-only and are surfaced as S3 paths.
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
        </section>

        <section className="grid gap-4 xl:grid-cols-[1.1fr_1fr_0.95fr]">
          <Panel title={mode === "HYBRID" ? "Agent Flow" : "Flow Queue"} icon={Activity}>
            <div className="space-y-3">
              {state.flows.slice(0, 20).map((flow) => {
                const active = selectedFlow?.id === flow.id;
                return (
                  <button
                    key={flow.id}
                    type="button"
                    onClick={() => setSelectedFlowId(flow.id)}
                    className={`w-full rounded-2xl border p-3 text-left transition ${
                      active
                        ? "border-siem-accent/45 bg-siem-accent/12"
                        : "border-siem-border bg-siem-panel/55 hover:border-siem-accent/25 hover:bg-siem-panel-strong"
                    }`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate font-semibold">{flow.id}</div>
                        <div className="mt-1 text-xs text-siem-muted">{flow.latest_preview || "No decoded preview available"}</div>
                      </div>
                      <span className="rounded-full border border-siem-border px-2 py-1 text-[11px] uppercase tracking-[0.18em] text-siem-muted">
                        {flow.latest_status || "active"}
                      </span>
                    </div>
                    <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                      <Tag>{flow.message_count} msgs</Tag>
                      <Tag>{flow.topic_count} topics</Tag>
                      <Tag>{flow.sender_count} senders</Tag>
                      <Tag>{formatTime(flow.last_seen)}</Tag>
                    </div>
                  </button>
                );
              })}
            </div>
          </Panel>

          <Panel title={mode === "HYBRID" ? "Fusion Summary" : "Trace Graph"} icon={GitBranch}>
            <div className="space-y-4">
              {selectedFlow ? (
                <>
                  <div className="rounded-2xl border border-siem-border bg-siem-panel/60 p-4">
                    <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Selected flow</div>
                    <div className="mt-2 text-lg font-semibold">{selectedFlow.id}</div>
                    <div className="mt-2 grid gap-2 text-sm text-siem-muted">
                      <div>First seen: {formatTime(selectedFlow.first_seen)}</div>
                      <div>Last seen: {formatTime(selectedFlow.last_seen)}</div>
                      <div>Topics: {selectedFlow.topics.join(", ") || "none"}</div>
                      <div>Senders: {selectedFlow.senders.join(", ") || "none"}</div>
                    </div>
                  </div>
                  <div className="space-y-3">
                    {relatedTraces.length === 0 && mode === "HYBRID" ? (
                      <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4 text-sm text-siem-muted">
                        External Intel context is policy-driven. This selected flow has no matched OSINT fusion card yet.
                      </div>
                    ) : null}
                    {relatedTraces.map((trace) => (
                      <div key={trace.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <div className="font-semibold">{trace.id}</div>
                            <div className="mt-1 text-xs text-siem-muted">{trace.latest_title || "trace span chain"}</div>
                          </div>
                          <Tag>{trace.span_count} spans</Tag>
                        </div>
                        <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                          {trace.span_types.map((type) => (
                            <Tag key={type}>{type}</Tag>
                          ))}
                        </div>
                      </div>
                    ))}
                    {relatedTasks.map((task) => (
                      <div key={task.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
                        <div className="font-semibold">{task.id}</div>
                        <div className="mt-1 text-xs text-siem-muted">{task.description || task.last_summary || "task record"}</div>
                        <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                          <Tag>{task.status || "unknown"}</Tag>
                          {task.requester_id ? <Tag>from {task.requester_id}</Tag> : null}
                          {task.responder_id ? <Tag>to {task.responder_id}</Tag> : null}
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              ) : (
                <EmptyState text="No AgentOps flow data yet. Once Kafka tracking is enabled and messages are consumed, flows and traces appear here." />
              )}
            </div>
          </Panel>

          <div className="grid gap-4">
            <Panel title={mode === "HYBRID" ? "External Intel Context" : "Agent Context"} icon={ShieldAlert}>
              <div className="grid gap-3">
                <StatusRow label="Tracking group" value={state.health.group_id || "unconfigured"} />
                <StatusRow label="Connected" value={state.health.connected ? "yes" : "no"} />
                <StatusRow label="Accepted" value={String(state.health.accepted_count)} />
                <StatusRow label="Rejected" value={String(state.health.rejected_count)} />
                <StatusRow label="Mirrored" value={String(state.health.mirrored_count)} />
                <StatusRow label="Last poll" value={formatTime(state.health.last_poll_at || "")} />
              </div>
            </Panel>

            <Panel title="Topic Health" icon={RadioTower}>
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

            <Panel title="Replay Panel" icon={TimerReset}>
              <div className="space-y-2">
                {(state.replay_sessions.length === 0 ? [{ id: "none", status: "idle", group_id: "", started_at: "", message_count: 0 }] : state.replay_sessions).map((session) => (
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

        <section className="grid gap-4 xl:grid-cols-[1.1fr_1fr]">
          <Panel title="Message Detail" icon={RadioTower}>
            <div className="space-y-3">
              {relatedMessages.map((message) => (
                <MessageCard key={message.id} message={message} />
              ))}
              {relatedMessages.length === 0 ? <EmptyState text="No decoded messages for the selected flow yet." /> : null}
            </div>
          </Panel>
          <Panel title={mode === "HYBRID" ? "Internal / External Split" : "Replay Notes"} icon={TimerReset}>
            <div className="space-y-3 text-sm text-siem-muted">
              {mode === "HYBRID" ? (
                <>
                  <p>Hybrid mode keeps AgentOps telemetry distinct from OSINT context until an explicit fusion rule matches.</p>
                  <p>The current implementation surfaces the operating split and reserved fusion lane without collapsing both domains into one queue.</p>
                </>
              ) : (
                <>
                  <p>Replay starts with a dedicated consumer group and reads from `earliest` without mutating the live tracking group.</p>
                  <p>Normal Kafka messages are decoded directly. LFS-backed entries stay pointer-only and are shown as S3 paths.</p>
                </>
              )}
            </div>
          </Panel>
        </section>
      </div>
    </main>
  );
}
