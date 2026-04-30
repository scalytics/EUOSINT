import { useEffect, useMemo, useState, useTransition, type ReactNode } from "react";
import { AlertTriangle, CheckCircle2, PlayCircle, Search, TimerReset, X } from "lucide-react";
import { EmptyState, StatusRow, Tag } from "@/agentops/components/Chrome";
import { EntityChip } from "@/agentops/components/EntityChip";
import { GraphCanvas } from "@/agentops/components/GraphCanvas";
import { MessageCard } from "@/agentops/components/MessageCard";
import { ProvenanceDrawer } from "@/agentops/components/ProvenanceDrawer";
import { RuntimeMap } from "@/agentops/components/RuntimeMap";
import { buildFailureBuckets } from "@/agentops/lib/failures";
import { edgeColorMap } from "@/agentops/lib/graph";
import { buildFusionMatches } from "@/agentops/lib/hybrid";
import { buildConversationTimeline, buildRunSummary } from "@/agentops/lib/investigation";
import { loadAnomaliesOnly, loadQueueFilter, loadSelectedRunId, persistAnomaliesOnly, persistQueueFilter, persistSelectedRunId, type RunQueueFilter } from "@/agentops/lib/preferences";
import { displayModeName } from "@/agentops/lib/state";
import { agentOpsDataSource } from "@/agentops/lib/api-source";
import { demoLabel, isAgentOpsDemo } from "@/agentops/lib/demo";
import { formatTime } from "@/agentops/lib/view";
import {
  entityCanonicalID,
  entityFromSearchResult,
  entityKey,
  entityRefFromFlow,
  parseCommandFilters,
  refsForFlow,
  refsForMessage,
  type EntityRef,
} from "@/agentops/lib/entities";
import { useAgentOpsOperator } from "@/hooks/useAgentOpsOperator";
import { useSavedInvestigation } from "@/hooks/useSavedInvestigation";
import {
  useEntityNeighborhood,
  useEntityProfile,
  useEntityProvenance,
  useFlow,
  useFlowMessages,
  useFlowTasks,
  useFlowTraces,
  useFlows,
  useHealth,
  useMapFeatures,
  useMapLayers,
  useOntologyPacks,
  useReplaySessions,
  useSearchEntities,
  useTopicHealth,
} from "@/hooks/useAgentOpsApi";
import { useAlerts } from "@/hooks/useAlerts";
import type { AgentOpsFlow, AgentOpsHealth, AgentOpsMessage, AgentOpsMode, Pack, Profile, ReplaySession, SearchResult } from "@/agentops/types";

const CORE_ENTITY_TYPES = ["agent", "task", "trace", "topic", "correlation", "location", "area"];

type WorkspaceTab = "topology" | "map" | "replay" | "failures" | "raw" | "operator";

const WORKSPACE_TABS: Array<{ id: WorkspaceTab; label: string; aria: string }> = [
  { id: "topology", label: "topology", aria: "Topology" },
  { id: "map", label: "map", aria: "Map" },
  { id: "replay", label: "replay", aria: "Replay Studio" },
  { id: "failures", label: "failures", aria: "Failure Workbench" },
  { id: "raw", label: "raw", aria: "Raw Messages" },
  { id: "operator", label: "operator", aria: "Operator" },
];

const EMPTY_HEALTH: AgentOpsHealth = {
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
};

interface Props {
  mode: AgentOpsMode;
}

function anomalyHint(status?: string, messageCount = 0): boolean {
  if (messageCount === 0) return true;
  return /(attention|blocked|degraded|failed|error|rejected|timeout|stalled|triage)/i.test(status || "");
}

export function AgentOpsRuntimeDesk({ mode }: Props) {
  const modeName = displayModeName(mode);
  const demoActive = isAgentOpsDemo();
  const activeDemoLabel = demoLabel();
  const operator = useAgentOpsOperator(mode !== "OSINT");
  const { alerts } = useAlerts();
  const [selectedFlowId, setSelectedFlowId] = useState<string | null>(() => (demoActive ? null : loadSelectedRunId()));
  const [queueFilter, setQueueFilter] = useState<RunQueueFilter>(() => (demoActive ? "attention" : loadQueueFilter()));
  const [anomaliesOnly, setAnomaliesOnly] = useState<boolean>(() => (demoActive ? false : loadAnomaliesOnly()));
  const [workspaceTab, setWorkspaceTab] = useState<WorkspaceTab>("topology");
  const [selectedMessageId, setSelectedMessageId] = useState<string | null>(null);
  const [replayScope, setReplayScope] = useState<"run" | "trace" | "task" | "topics">("run");
  const [replayNotice, setReplayNotice] = useState("");
  const [optimisticReplaySessions, setOptimisticReplaySessions] = useState<ReplaySession[]>([]);
  const [mapBBox, setMapBBox] = useState("14.40,35.80,14.60,36.00");
  const [mapTypes, setMapTypes] = useState<string[]>([]);
  const [commandValue, setCommandValue] = useState("");
  const [activeCommand, setActiveCommand] = useState("");
  const [drawerSubject, setDrawerSubject] = useState<EntityRef | null>(null);
  const [isPending, startTransition] = useTransition();

  const commandFilters = useMemo(() => parseCommandFilters(activeCommand), [activeCommand]);
  const { flows } = useFlows({
    limit: 50,
    status: commandFilters.status ?? (queueFilter === "all" || queueFilter === "attention" ? undefined : queueFilter),
    topic: commandFilters.topic,
    sender: commandFilters.sender,
    q: commandFilters.text,
  });
  const { health } = useHealth();
  const effectiveHealth = health ?? EMPTY_HEALTH;
  const { topicHealth } = useTopicHealth();
  const { replaySessions } = useReplaySessions();
  const { mapLayers } = useMapLayers();
  const { featureCollection } = useMapFeatures({ bbox: mapBBox, types: mapTypes.length > 0 ? mapTypes.join(",") : undefined });
  const { packs } = useOntologyPacks();
  const { results: commandResults, isLoading: commandLoading } = useSearchEntities(activeCommand);

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
      if (!demoActive) persistSelectedRunId(next);
    }
  }, [demoActive, flows, selectedFlowId]);

  const { flow: selectedFlow } = useFlow(selectedFlowId);
  const { messages: relatedMessages } = useFlowMessages(selectedFlowId, { limit: 100 });
  const { tasks: relatedTasks } = useFlowTasks(selectedFlowId);
  const { traces: relatedTraces } = useFlowTraces(selectedFlowId);
  const selectedCorrelation = selectedFlow ? entityRefFromFlow(selectedFlow) : null;
  const selectedCanonicalID = selectedCorrelation ? entityCanonicalID(selectedCorrelation) : null;
  const { neighborhood } = useEntityNeighborhood(selectedCorrelation?.type ?? null, selectedCanonicalID, { depth: 2 });
  const { profile: selectedProfile } = useEntityProfile(selectedCorrelation?.type ?? null, selectedCanonicalID);
  const { provenance: selectedProvenance } = useEntityProvenance(selectedCorrelation?.type ?? null, selectedCanonicalID);
  const edgeColors = useMemo(() => edgeColorMap(packs), [packs]);
  const ontologyTypes = useMemo(() => activeOntologyTypes(packs), [packs]);
  const activePackLabel = useMemo(() => packLabel(packs), [packs]);
  const saved = useSavedInvestigation(selectedFlowId);

  useEffect(() => {
    setSelectedMessageId((current) => current ?? relatedMessages[0]?.id ?? null);
  }, [relatedMessages]);

  const runSummary = useMemo(
    () => buildRunSummary(selectedFlow, relatedMessages, relatedTasks, relatedTraces, effectiveHealth),
    [effectiveHealth, relatedMessages, relatedTasks, relatedTraces, selectedFlow],
  );
  const timeline = useMemo(() => buildConversationTimeline(selectedFlow, relatedMessages, relatedTasks, relatedTraces), [relatedMessages, relatedTasks, relatedTraces, selectedFlow]);
  const failureBuckets = useMemo(() => buildFailureBuckets(selectedFlow, relatedMessages, relatedTasks, effectiveHealth), [effectiveHealth, relatedMessages, relatedTasks, selectedFlow]);
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
    () =>
      [
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
    if (!demoActive) persistSelectedRunId(id);
    setWorkspaceTab("topology");
  }

  function updateQueueFilter(filter: RunQueueFilter) {
    setQueueFilter(filter);
    if (!demoActive) persistQueueFilter(filter);
  }

  function toggleAnomaliesOnly() {
    setAnomaliesOnly((current) => {
      const next = !current;
      if (!demoActive) persistAnomaliesOnly(next);
      return next;
    });
  }

  function triggerReplay() {
    startTransition(() => {
      const optimistic: ReplaySession = {
        id: `replay-${Date.now()}`,
        group_id: `${effectiveHealth.group_id || "agentops"}-replay`,
        status: "starting",
        started_at: new Date().toISOString(),
        message_count: relatedMessages.length,
        topics: selectedFlow?.topics ?? effectiveHealth.effective_topics ?? [],
      };
      setOptimisticReplaySessions((current) => [optimistic, ...current]);
      setReplayNotice("Replay request queued.");
      void agentOpsDataSource()
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

  const topicCount = effectiveHealth.effective_topics.length;
  const detectorCount = packs.reduce((total, pack) => total + (pack.detectors?.length ?? 0), 0);
  const provenanceTrail = selectedProvenance.slice(0, 4);

  return (
    <main className="h-dvh overflow-hidden bg-[#07131c] text-[#e8eef3]">
      <div className="mx-auto flex h-full max-w-[1900px] flex-col overflow-hidden p-3 md:p-4">
        <header className="grid min-h-[58px] gap-3 rounded-t border border-[#18313f] bg-[#050d14] px-4 py-3 xl:grid-cols-[180px_minmax(360px,1fr)_auto] xl:items-center">
          <div className="flex items-center gap-3">
            <div className="font-mono text-sm font-semibold tracking-[0.16em] text-[#1D9E75]">kafSIEM</div>
            <div className="rounded border border-[#1D9E75]/45 bg-[#1D9E75]/10 px-2 py-1 font-mono text-[10px] uppercase tracking-[0.18em] text-[#1D9E75]">
              {modeName}
            </div>
          </div>

          <form
            className="flex min-h-9 min-w-0 items-center gap-2 border border-[#18313f] bg-[#0b1b26] px-3"
            onSubmit={(event) => {
              event.preventDefault();
              setActiveCommand(commandValue.trim());
            }}
          >
            <Search size={14} className="shrink-0 text-[#1D9E75]" />
            <input
              value={commandValue}
              onChange={(event) => setCommandValue(event.target.value)}
              placeholder={commandPlaceholder(ontologyTypes)}
              className="min-w-0 flex-1 bg-transparent font-mono text-xs text-[#e8eef3] outline-none placeholder:text-[#6b8090]"
            />
            {commandValue ? (
              <button
                type="button"
                aria-label="Clear command search"
                onClick={() => {
                  setCommandValue("");
                  setActiveCommand("");
                }}
                className="text-[#6b8090] hover:text-[#e8eef3]"
              >
                <X size={13} />
              </button>
            ) : null}
          </form>

          <div className="flex flex-wrap items-center gap-2 font-mono text-[10px] uppercase tracking-[0.14em] text-[#a8b8c4]">
            <StatusPill label="ingest" value={effectiveHealth.connected ? `+${effectiveHealth.accepted_count}` : "offline"} ok={effectiveHealth.connected} />
            <StatusPill label="topics" value={String(topicCount)} ok={topicCount > 0} />
            <StatusPill label="det" value={String(detectorCount)} ok={detectorCount > 0} />
            <StatusPill label="pack" value={activePackLabel} ok={packs.length > 0} />
            <StatusPill label="stream" value={demoActive ? activeDemoLabel : "live"} ok />
            <button
              type="button"
              onClick={() => void triggerReplay()}
              className="inline-flex min-h-8 items-center gap-2 border border-[#1D9E75]/45 bg-[#1D9E75]/10 px-3 text-[#1D9E75] hover:bg-[#1D9E75]/16"
            >
              {isPending ? <TimerReset size={13} className="animate-spin" /> : <PlayCircle size={13} />}
              <span>Start from earliest</span>
            </button>
          </div>
        </header>

        {replayNotice ? (
          <div className="border-x border-[#18313f] bg-[#0b1b26] px-4 py-2 font-mono text-xs text-[#e8eef3]">
            <span className="inline-flex items-center gap-2">
              <CheckCircle2 size={14} className="text-[#1D9E75]" />
              {replayNotice}
            </span>
          </div>
        ) : null}

        <section className="grid min-h-0 flex-1 overflow-hidden border-x border-[#18313f] bg-[#07131c] xl:grid-cols-[280px_minmax(0,1fr)_340px]">
          <aside className="flex min-h-0 flex-col border-b border-[#18313f] xl:border-b-0 xl:border-r">
            <RailHeader title={mode === "HYBRID" ? "Fusion Queue" : "Operations Queue"} count={`${queueFlows.length} / ${flows.length}`} />
            <div className="flex flex-wrap gap-2 border-b border-[#18313f] px-3 py-2">
              {(["attention", "active", "completed", "all"] as RunQueueFilter[]).map((filter) => (
                <FilterButton key={filter} active={queueFilter === filter} onClick={() => updateQueueFilter(filter)}>
                  {filter}
                </FilterButton>
              ))}
              <FilterButton active={anomaliesOnly} onClick={toggleAnomaliesOnly}>
                anomalies only
              </FilterButton>
            </div>
            <div className="border-b border-[#18313f] px-3 py-2">
              <div className="mb-2 font-mono text-[10px] uppercase tracking-[0.16em] text-[#6b8090]">Ontology</div>
              <div className="flex flex-wrap gap-1.5">
                {ontologyTypes.slice(0, 12).map((type) => (
                  <span key={type} className="border border-[#24425a] px-2 py-1 font-mono text-[10px] text-[#a8b8c4]">
                    {type}
                  </span>
                ))}
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3">
              <CommandResults results={commandResults} isLoading={commandLoading} activeCommand={activeCommand} onFlowSelect={selectFlow} />
              {queueSections.length === 0 ? <EmptyState text="No runs match the current queue filter." /> : null}
              {queueSections.map((section) => (
                <div key={section.key} className="mb-5">
                  <div className="mb-2 px-1 font-mono text-[10px] uppercase tracking-[0.18em] text-[#6b8090]">{section.title}</div>
                  <div className="space-y-2">
                    {section.items.map((flow) => (
                      <RunQueueRow
                        key={flow.id}
                        flow={flow}
                        active={selectedFlow?.id === flow.id}
                        saved={saved}
                        onSelect={selectFlow}
                      />
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </aside>

          <section className="flex min-h-0 flex-col">
            <div className="flex min-h-10 flex-wrap items-center border-b border-[#18313f] bg-[#050d14] px-3">
              {WORKSPACE_TABS.map((tab, index) => (
                <button
                  key={tab.id}
                  type="button"
                  aria-label={tab.aria}
                  onClick={() => setWorkspaceTab(tab.id)}
                  className={`min-h-10 border-b-2 px-4 font-mono text-[11px] tracking-[0.08em] ${
                    workspaceTab === tab.id ? "border-[#1D9E75] text-[#1D9E75]" : "border-transparent text-[#6b8090] hover:text-[#e8eef3]"
                  }`}
                >
                  {tab.label}
                  <span className="ml-2 border border-[#24425a] px-1 text-[9px] text-[#6b8090]">{index + 1}</span>
                </button>
              ))}
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto bg-[linear-gradient(rgba(36,66,90,0.16)_1px,transparent_1px),linear-gradient(90deg,rgba(36,66,90,0.16)_1px,transparent_1px)] bg-[length:20px_20px] p-3">
              {workspaceTab === "topology" ? (
                selectedCorrelation ? (
                  <GraphCanvas
                    entities={neighborhood.entities}
                    edges={neighborhood.edges}
                    edgeColors={edgeColors}
                    selectedEntityId={entityKey(selectedCorrelation)}
                    onEntityClick={(entity) => {
                      window.location.href = `?view=entity&type=${entity.type}&id=${entityCanonicalID(entity)}`;
                    }}
                  />
                ) : (
                  <EmptyState text="Select a run to render the 2-hop correlation neighborhood." />
                )
              ) : null}

              {workspaceTab === "map" ? (
                <RuntimeMap bbox={mapBBox} layers={mapLayers} features={featureCollection} selectedTypes={mapTypes} onBBoxChange={setMapBBox} onTypesChange={setMapTypes} />
              ) : null}

              {workspaceTab === "replay" ? (
                <div className="grid gap-3 xl:grid-cols-[1fr_1fr]">
                  <ConsoleBlock title="Replay Scope">
                    <div className="flex flex-wrap gap-2">
                      {([
                        ["run", "Full run"],
                        ["trace", "Selected trace"],
                        ["task", "Task chain"],
                        ["topics", "Topic families"],
                      ] as const).map(([scope, label]) => (
                        <FilterButton key={scope} active={replayScope === scope} onClick={() => setReplayScope(scope)}>
                          {label}
                        </FilterButton>
                      ))}
                    </div>
                    <div className="mt-4 grid gap-2">
                      <StatusRow label="Replay group" value={`${effectiveHealth.group_id || "agentops"}-replay`} />
                      <StatusRow label="Expected records" value={String(relatedMessages.length)} />
                      <StatusRow label="Scope" value={replayScope} />
                    </div>
                  </ConsoleBlock>
                  <ConsoleBlock title="Replay Sessions">
                    <div className="space-y-2">
                      {(optimisticReplaySessions.length === 0 ? [{ id: "none", status: "idle", group_id: "", started_at: "", message_count: 0 }] : optimisticReplaySessions).map((session) => (
                        <div key={session.id} className="border border-[#18313f] bg-[#07131c]/80 p-3">
                          <div className="flex items-center justify-between gap-3">
                            <div className="text-xs font-semibold">{session.id === "none" ? "No replay sessions yet" : session.group_id}</div>
                            <Tag>{session.status}</Tag>
                          </div>
                          {session.id !== "none" ? <div className="mt-2 text-[11px] text-[#6b8090]">Started {formatTime(session.started_at)} · {session.message_count} messages</div> : null}
                        </div>
                      ))}
                    </div>
                  </ConsoleBlock>
                </div>
              ) : null}

              {workspaceTab === "failures" ? (
                <div className="space-y-3">
                  {failureBuckets.length === 0 ? (
                    <EmptyState text="No failure buckets for the selected run." />
                  ) : (
                    failureBuckets.map((bucket) => (
                      <div key={bucket.id} className="border border-[#18313f] bg-[#07131c]/85 p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <div className="font-semibold text-[#e8eef3]">{bucket.title}</div>
                            <div className="mt-1 text-xs text-[#a8b8c4]">{bucket.detail}</div>
                          </div>
                          <Tag>{bucket.count}</Tag>
                        </div>
                        {bucket.messageId ? (
                          <button
                            type="button"
                            onClick={() => {
                              setSelectedMessageId(bucket.messageId ?? null);
                              setWorkspaceTab("raw");
                            }}
                            className="mt-3 border border-[#24425a] px-3 py-1 font-mono text-[11px] uppercase tracking-[0.14em] text-[#1D9E75]"
                          >
                            inspect raw
                          </button>
                        ) : null}
                      </div>
                    ))
                  )}
                </div>
              ) : null}

              {workspaceTab === "raw" ? (
                selectedMessage ? (
                  <div className="space-y-3">
                  <ConsoleBlock title="Selected Message">
                    <div className="font-mono text-xs text-[#e8eef3]">{selectedMessage.id}</div>
                    <div className="mt-2 text-xs text-[#a8b8c4]">{selectedMessage.topic} · p{selectedMessage.partition} · o{selectedMessage.offset}</div>
                    <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-[#a8b8c4]">
                      {refsForMessage(selectedMessage, packs).map((entity) => (
                        <EntityChip key={entityKey(entity)} entity={entity} pinned={saved.isPinned(entity)} onTogglePin={saved.togglePinned} />
                      ))}
                      <button
                        type="button"
                        onClick={() => setDrawerSubject(subjectForMessage(selectedMessage) || selectedCorrelation)}
                        className="border border-[#24425a] px-2 py-1 font-mono text-[10px] uppercase tracking-[0.14em] text-[#1D9E75]"
                      >
                        why
                      </button>
                    </div>
                  </ConsoleBlock>
                    <MessageCard message={selectedMessage} />
                  </div>
                ) : (
                  <EmptyState text="Select a timeline or message entry to inspect the raw transport artifact." />
                )
              ) : null}

              {workspaceTab === "operator" ? (
                <div className="grid gap-3 xl:grid-cols-[320px_1fr]">
                  <ConsoleBlock title="Operator State">
                    <div className="grid gap-2">
                      <StatusRow label="Admin surface" value={operator.supported ? "supported" : "limited"} />
                      <StatusRow label="Live group" value={operator.live_group_id || effectiveHealth.group_id || "-"} />
                      <StatusRow label="Replay groups" value={String(operator.replay_group_ids.length)} />
                    </div>
                    {operator.last_error ? <div className="mt-3 border border-[#24425a] bg-[#07131c] p-3 text-xs text-[#a8b8c4]">{operator.last_error}</div> : null}
                  </ConsoleBlock>
                  <ConsoleBlock title="Consumer Groups">
                    <div className="space-y-2">
                      {operator.groups.length === 0 ? (
                        <EmptyState text="No consumer groups returned yet." />
                      ) : (
                        operator.groups.slice(0, 8).map((group) => (
                          <div key={group.group_id} className="border border-[#18313f] bg-[#07131c]/80 p-3">
                            <div className="flex items-center justify-between gap-3">
                              <div className="font-mono text-xs text-[#e8eef3]">{group.group_id}</div>
                              <Tag>{group.state || "unknown"}</Tag>
                            </div>
                            <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-[#6b8090]">
                              <Tag>{group.protocol_type || "consumer"}</Tag>
                              {group.protocol ? <Tag>{group.protocol}</Tag> : null}
                              <Tag>{group.members.length} members</Tag>
                            </div>
                          </div>
                        ))
                      )}
                    </div>
                  </ConsoleBlock>
                </div>
              ) : null}
            </div>

            <div className="border-t border-[#18313f] bg-[#050d14] px-4 py-2 font-mono text-[10px] uppercase tracking-[0.12em] text-[#6b8090]">
              <div className="mb-1 flex justify-between gap-3">
                <span>-24h</span>
                <span>{selectedFlow ? formatTime(selectedFlow.last_seen) : "no run selected"}</span>
              </div>
              <div className="relative h-3 border border-[#18313f] bg-[#0b1b26]">
                <div className="absolute inset-y-[-1px] left-[40%] w-[22%] border border-[#1D9E75] bg-[#1D9E75]/18" />
                <div className="absolute inset-y-0 left-[18%] w-px bg-[#d6a82e]" />
                <div className="absolute inset-y-0 left-[62%] w-px bg-[#d6a82e]" />
              </div>
            </div>
          </section>

          <aside className="flex min-h-0 flex-col border-t border-[#18313f] xl:border-l xl:border-t-0">
            <RailHeader title="Entity Detail" count={selectedProfile?.entity.type ?? selectedCorrelation?.type ?? "none"} />
            <div className="min-h-0 flex-1 overflow-y-auto p-3">
              <EntityDetailPanel profile={selectedProfile} packs={packs} fallbackFlow={selectedFlow} saved={saved} />

              <ConsoleBlock title="Run Summary" className="mt-3">
                <div className="grid gap-2">
                  <StatusRow label="Replayable" value={runSummary.replayable ? "yes" : "no"} />
                  <StatusRow label="Duration" value={runSummary.durationLabel} />
                  <StatusRow label="Participants" value={String(runSummary.participantCount)} />
                  <StatusRow label="Requests / Responses" value={`${runSummary.requestCount} / ${runSummary.responseCount}`} />
                  <StatusRow label="Confidence" value={String(runSummary.confidence)} />
                </div>
              </ConsoleBlock>

              <ConsoleBlock title={mode === "HYBRID" ? "Fusion Timeline" : "Activity Timeline"} className="mt-3">
                <div className="space-y-2">
                  {timeline.length === 0 ? (
                    <EmptyState text="No timeline events for the selected run." />
                  ) : (
                    timeline.slice(0, 5).map((item) =>
                      item.kind === "gap" ? (
                        <div key={item.id} className="border border-dashed border-[#24425a] bg-[#07131c]/70 p-3 text-xs text-[#a8b8c4]">
                          <div className="flex items-center gap-2">
                            <AlertTriangle size={13} className="text-[#d6a82e]" />
                            {item.detail}
                          </div>
                        </div>
                      ) : (
                        <div key={item.id} className="border border-[#18313f] bg-[#07131c]/80 p-3">
                          <button
                            type="button"
                            onClick={() => {
                              if (item.sourceMessageId) {
                                setSelectedMessageId(item.sourceMessageId);
                                setWorkspaceTab("raw");
                              }
                            }}
                            className="block w-full text-left"
                          >
                            <div className="flex items-start justify-between gap-3">
                              <div>
                                <div className="font-semibold text-[#e8eef3]">{item.title}</div>
                                <div className="mt-1 text-xs text-[#a8b8c4]">{item.detail}</div>
                              </div>
                              <Tag>{item.family || "event"}</Tag>
                            </div>
                          </button>
                          <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-[#a8b8c4]">
                            <Tag>{formatTime(item.at)}</Tag>
                            {item.status ? <Tag>{item.status}</Tag> : null}
                            <button
                              type="button"
                              onClick={() => setDrawerSubject(item.sender ? { type: "agent", id: item.sender, label: item.sender } : selectedCorrelation)}
                              className="border border-[#24425a] px-2 py-1 font-mono text-[10px] uppercase tracking-[0.14em] text-[#1D9E75]"
                            >
                              why
                            </button>
                          </div>
                        </div>
                      ),
                    )
                  )}
                </div>
              </ConsoleBlock>

              {mode === "HYBRID" ? (
                <ConsoleBlock title="Fusion Context" className="mt-3">
                  {fusionMatches.length > 0 ? (
                    <div className="space-y-2">
                      {fusionMatches.slice(0, 4).map((match) => (
                        <div key={match.alert_id} className="border border-[#18313f] bg-[#07131c]/80 p-3">
                          <div className="font-semibold text-[#e8eef3]">{match.title}</div>
                          <div className="mt-1 text-xs text-[#6b8090]">{match.source}</div>
                          <div className="mt-2 flex flex-wrap gap-1.5 text-[11px] text-[#a8b8c4]">
                            <Tag>{match.severity}</Tag>
                            {match.match_reasons.map((reason) => <Tag key={reason}>{reason}</Tag>)}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <EmptyState text="No OSINT fusion match for the selected flow." />
                  )}
                </ConsoleBlock>
              ) : null}

              <ConsoleBlock title="Topic Health" className="mt-3">
                <div className="space-y-2">
                  {topicHealth.length === 0 ? (
                    <EmptyState text="No topic health yet." />
                  ) : (
                    topicHealth.slice(0, 5).map((topic) => (
                      <div key={topic.topic} className="border border-[#18313f] bg-[#07131c]/80 p-3">
                        <div className="font-mono text-xs text-[#e8eef3]">{topic.topic}</div>
                        <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-[#6b8090]">
                          <Tag>{topic.active_agents} agents</Tag>
                          <Tag>{topic.messages_per_hour.toFixed(0)} msg/h</Tag>
                          <Tag>{topic.is_stale ? "stale" : "active"}</Tag>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </ConsoleBlock>

              <ConsoleBlock title="Notes" className="mt-3">
                <textarea
                  value={saved.investigation.notes}
                  onChange={(event) => saved.setNotes(event.target.value)}
                  className="min-h-[120px] w-full resize-none border border-[#18313f] bg-[#07131c] p-3 text-sm text-[#e8eef3] outline-none"
                />
                <div className="mt-3 grid gap-2">
                  <StatusRow label="Opened" value={formatTime(saved.investigation.openedAt)} />
                  <StatusRow label="Pinned entities" value={String(saved.investigation.pinnedEntities.length)} />
                </div>
              </ConsoleBlock>
            </div>
          </aside>
        </section>

        <footer className="flex min-h-[44px] flex-wrap items-center gap-3 rounded-b border border-[#18313f] bg-[#050d14] px-4 py-2 font-mono text-[11px]">
          <button
            type="button"
            onClick={() => setDrawerSubject(selectedCorrelation)}
            disabled={!selectedCorrelation}
            className="font-semibold uppercase tracking-[0.14em] text-[#d6a82e] disabled:text-[#6b8090]"
          >
            provenance · {selectedCorrelation ? entityKey(selectedCorrelation) : "no subject"}
          </button>
          {provenanceTrail.length > 0 ? (
            provenanceTrail.map((item, index) => (
              <span key={`${item.subject_id}-${item.stage}-${item.produced_at}-${index}`} className="border border-[#18313f] bg-[#07131c] px-2 py-1 text-[#a8b8c4]">
                {item.stage} · {formatTime(item.produced_at)}
              </span>
            ))
          ) : (
            <>
              <span className="border border-[#18313f] bg-[#07131c] px-2 py-1 text-[#a8b8c4]">{selectedMessage?.topic ?? "no topic"}</span>
              <span className="text-[#d6a82e]">→</span>
              <span className="border border-[#18313f] bg-[#07131c] px-2 py-1 text-[#a8b8c4]">graph</span>
              <span className="text-[#d6a82e]">→</span>
              <span className="border border-[#18313f] bg-[#07131c] px-2 py-1 text-[#a8b8c4]">{selectedFlow?.last_seen ? formatTime(selectedFlow.last_seen) : "pending"}</span>
            </>
          )}
        </footer>
      </div>
      <ProvenanceDrawer subject={drawerSubject} onClose={() => setDrawerSubject(null)} />
    </main>
  );
}

function RailHeader({ title, count }: { title: string; count: string }) {
  return (
    <div className="flex min-h-10 items-center justify-between border-b border-[#18313f] bg-[#050d14] px-3">
      <span className="font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-[#e8eef3]">{title}</span>
      <span className="font-mono text-[10px] text-[#6b8090]">{count}</span>
    </div>
  );
}

function StatusPill({ label, value, ok }: { label: string; value: string; ok: boolean }) {
  return (
    <span className="inline-flex min-h-8 items-center gap-1.5 border border-[#18313f] bg-[#07131c] px-2">
      <span>{label}</span>
      <span className={ok ? "text-[#1D9E75]" : "text-[#d6a82e]"}>●</span>
      <span className="max-w-[120px] truncate text-[#e8eef3]">{value}</span>
    </span>
  );
}

function FilterButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`border px-2.5 py-1 font-mono text-[10px] uppercase tracking-[0.14em] ${
        active ? "border-[#1D9E75] bg-[#1D9E75]/10 text-[#1D9E75]" : "border-[#24425a] text-[#a8b8c4] hover:text-[#e8eef3]"
      }`}
    >
      {children}
    </button>
  );
}

function ConsoleBlock({ title, className = "", children }: { title: string; className?: string; children: ReactNode }) {
  return (
    <section className={`border border-[#18313f] bg-[#0b1b26]/92 p-3 ${className}`}>
      <div className="mb-3 font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-[#1D9E75]">{title}</div>
      {children}
    </section>
  );
}

function RunQueueRow({
  flow,
  active,
  saved,
  onSelect,
}: {
  flow: AgentOpsFlow;
  active: boolean;
  saved: ReturnType<typeof useSavedInvestigation>;
  onSelect: (id: string) => void;
}) {
  return (
    <div className={`border p-3 ${active ? "border-[#1D9E75] bg-[#1D9E75]/8" : "border-[#18313f] bg-[#0b1b26]/75 hover:border-[#24425a]"}`}>
      <button type="button" onClick={() => onSelect(flow.id)} className="block w-full text-left">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-[#e8eef3]">{flow.latest_preview || flow.id}</div>
            <div className="mt-1 font-mono text-[11px] text-[#6b8090]">{flow.id}</div>
            <div className="mt-2 truncate text-xs text-[#a8b8c4]">{flow.topics.join(", ") || "No topics declared"}</div>
          </div>
          <span className="shrink-0 border border-[#24425a] px-2 py-1 font-mono text-[10px] uppercase tracking-[0.14em] text-[#a8b8c4]">
            {flow.latest_status || "active"}
          </span>
        </div>
      </button>
      <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-[#a8b8c4]">
        <Tag>{flow.sender_count} participants</Tag>
        <Tag>{flow.message_count} messages</Tag>
        <Tag>{formatTime(flow.last_seen)}</Tag>
        {anomalyHint(flow.latest_status, flow.message_count) ? <Tag>attention</Tag> : null}
        {active ? <Tag>selected</Tag> : null}
      </div>
      <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-[#a8b8c4]">
        {refsForFlow(flow).map((entity) => (
          <EntityChip key={entityKey(entity)} entity={entity} pinned={saved.isPinned(entity)} onTogglePin={saved.togglePinned} />
        ))}
      </div>
    </div>
  );
}

function CommandResults({
  results,
  isLoading,
  activeCommand,
  onFlowSelect,
}: {
  results: SearchResult[];
  isLoading?: boolean;
  activeCommand: string;
  onFlowSelect: (id: string) => void;
}) {
  if (!activeCommand) return null;
  return (
    <div className="mb-4 border border-[#18313f] bg-[#050d14] p-3">
      <div className="mb-2 font-mono text-[10px] uppercase tracking-[0.16em] text-[#6b8090]">Command Results</div>
      {isLoading ? <div className="text-xs text-[#a8b8c4]">Searching.</div> : null}
      {!isLoading && results.length === 0 ? <EmptyState text="No command results." /> : null}
      <div className="space-y-2">
        {results.map((result) => (
          <SearchResultRow key={`${result.kind}-${result.id}`} result={result} onFlowSelect={onFlowSelect} />
        ))}
      </div>
    </div>
  );
}

function SearchResultRow({ result, onFlowSelect }: { result: SearchResult; onFlowSelect: (id: string) => void }) {
  const entity = entityFromSearchResult(result);
  if (entity) {
    return (
      <div className="flex items-center justify-between gap-3 border border-[#18313f] bg-[#07131c] px-3 py-2">
        <EntityChip entity={entity} />
        <Tag>{result.type}</Tag>
      </div>
    );
  }
  if (result.kind === "flow") {
    return (
      <button type="button" onClick={() => onFlowSelect(result.id)} className="block w-full border border-[#18313f] bg-[#07131c] px-3 py-2 text-left hover:border-[#1D9E75]/50">
        <div className="flex items-start justify-between gap-3">
          <span className="min-w-0 truncate font-semibold text-[#e8eef3]">{result.title || result.id}</span>
          {result.latest_status ? <Tag>{result.latest_status}</Tag> : null}
        </div>
        <div className="mt-1 font-mono text-[11px] text-[#6b8090]">{result.id}</div>
      </button>
    );
  }
  return (
    <div className="border border-[#18313f] bg-[#07131c] px-3 py-2">
      <div className="flex items-start justify-between gap-3">
        <span className="font-semibold text-[#e8eef3]">{result.title || result.detector_id || result.id}</span>
        {result.severity ? <Tag>{result.severity}</Tag> : null}
      </div>
      {result.source ? <div className="mt-1 text-[11px] text-[#6b8090]">{result.source}</div> : null}
    </div>
  );
}

function EntityDetailPanel({
  profile,
  packs,
  fallbackFlow,
  saved,
}: {
  profile: Profile | null;
  packs: Pack[];
  fallbackFlow: AgentOpsFlow | null;
  saved: ReturnType<typeof useSavedInvestigation>;
}) {
  if (!profile) {
    if (!fallbackFlow) return <EmptyState text="Select a run to inspect its correlation entity." />;
    return (
      <ConsoleBlock title="Run Summary">
        <div className="font-semibold text-[#e8eef3]">{fallbackFlow.latest_preview || fallbackFlow.id}</div>
        <div className="mt-1 font-mono text-xs text-[#6b8090]">{fallbackFlow.id}</div>
        <div className="mt-3 grid gap-2 text-sm text-[#a8b8c4]">
          <div>First seen: {formatTime(fallbackFlow.first_seen)}</div>
          <div>Last seen: {formatTime(fallbackFlow.last_seen)}</div>
          <div>Participants: {fallbackFlow.senders.join(", ") || "none"}</div>
        </div>
      </ConsoleBlock>
    );
  }

  const rows = compactFieldRows(profile, packs);
  const entityRef = { type: profile.entity.type, id: entityCanonicalID({ type: profile.entity.type, id: profile.entity.id }), label: profile.entity.display_name || profile.entity.id };
  return (
    <ConsoleBlock title={profile.entity.type}>
      <div className="flex items-start justify-between gap-3 border-b border-[#18313f] pb-3">
        <div className="min-w-0">
          <div className="truncate text-lg font-semibold text-[#e8eef3]">{profile.entity.display_name || profile.entity.canonical_id || profile.entity.id}</div>
          <div className="mt-1 font-mono text-xs text-[#6b8090]">{profile.entity.id}</div>
        </div>
        <EntityChip entity={entityRef} pinned={saved.isPinned(entityRef)} onTogglePin={saved.togglePinned} />
      </div>
      <div className="mt-3 grid gap-2">
        {rows.length > 0 ? (
          rows.slice(0, 8).map((row) => (
            <div key={row.key} className="flex justify-between gap-3 border-b border-dotted border-[#18313f] py-1.5 font-mono text-[11px]">
              <span className="text-[#6b8090]">{row.label}</span>
              <span className="max-w-[160px] overflow-wrap-anywhere text-right text-[#e8eef3]">{row.value}</span>
            </div>
          ))
        ) : (
          <EmptyState text="No pack fields declared for this entity." />
        )}
      </div>
      <div className="mt-4">
        <div className="mb-2 font-mono text-[10px] uppercase tracking-[0.16em] text-[#6b8090]">Edges</div>
        <div className="flex flex-wrap gap-2 text-[11px] text-[#a8b8c4]">
          {Object.entries(profile.edge_counts).map(([edgeType, count]) => <Tag key={edgeType}>{edgeType}:{count}</Tag>)}
          {Object.keys(profile.edge_counts).length === 0 ? <Tag>none</Tag> : null}
        </div>
      </div>
    </ConsoleBlock>
  );
}

function activeOntologyTypes(packs: Pack[]): string[] {
  const seen = new Set<string>(CORE_ENTITY_TYPES);
  for (const pack of packs) {
    for (const type of pack.entity_types ?? []) seen.add(type);
  }
  return [...seen].sort();
}

function packLabel(packs: Pack[]): string {
  if (packs.length === 0) return "core";
  return packs.map((pack) => pack.name).join(" · ");
}

function commandPlaceholder(types: string[]): string {
  if (types.includes("platform")) return "platform:auv-07 window:24h pack:drones";
  if (types.includes("device")) return "device:plc-12 window:72h pack:scada";
  return "agent:alice topic:requests window:1h";
}

function compactFieldRows(profile: Profile, packs: Pack[]): Array<{ key: string; label: string; value: string }> {
  const attrs = profile.entity.attrs ?? {};
  const view = packs.flatMap((pack) => pack.views ?? []).find((item) => item.entity_type === profile.entity.type);
  if (view?.fields?.length) {
    return view.fields.map((field) => ({
      key: field.id,
      label: field.label || field.id,
      value: displayValue(attrs[field.id]),
    }));
  }
  return Object.entries(attrs).map(([key, value]) => ({
    key,
    label: key,
    value: displayValue(value),
  }));
}

function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === "") return "-";
  if (Array.isArray(value)) return value.map((item) => displayValue(item)).join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function subjectForMessage(message: Pick<AgentOpsMessage, "task_id" | "trace_id" | "sender_id" | "correlation_id">): EntityRef | null {
  if (message.task_id) return { type: "task", id: message.task_id, label: message.task_id };
  if (message.trace_id) return { type: "trace", id: message.trace_id, label: message.trace_id };
  if (message.sender_id) return { type: "agent", id: message.sender_id, label: message.sender_id };
  if (message.correlation_id) return { type: "correlation", id: message.correlation_id, label: message.correlation_id };
  return null;
}
