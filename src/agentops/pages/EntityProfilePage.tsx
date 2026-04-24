import { useMemo, useState } from "react";
import { Activity, ArrowLeft, GitBranch, NotebookText, Pin, RadioTower, ShieldAlert, Workflow } from "lucide-react";
import { EmptyState, Panel, StatusRow, Tag } from "@/agentops/components/Chrome";
import { EntityChip } from "@/agentops/components/EntityChip";
import { EntityProfileCard } from "@/agentops/components/EntityProfileCard";
import { GraphCanvas } from "@/agentops/components/GraphCanvas";
import { MessageCard } from "@/agentops/components/MessageCard";
import { ProvenanceDrawer } from "@/agentops/components/ProvenanceDrawer";
import { edgeColorMap } from "@/agentops/lib/graph";
import { entityCanonicalID, entityKey, entityRefFromFlow, type EntityRef } from "@/agentops/lib/entities";
import { readEntityRoute } from "@/agentops/lib/routes";
import { formatTime } from "@/agentops/lib/view";
import { useSavedInvestigation } from "@/hooks/useSavedInvestigation";
import { useEntityNeighborhood, useEntityProfile, useEntityProvenance, useEntityTimeline, useFlows, useOntologyPacks } from "@/hooks/useAgentOpsApi";
import type { AgentOpsMode } from "@/agentops/types";

export function EntityProfilePage({ mode }: { mode: AgentOpsMode }) {
  const [subject] = useState<EntityRef | null>(() => readEntityRoute());
  const [drawerSubject, setDrawerSubject] = useState<EntityRef | null>(null);
  const canonicalID = subject ? entityCanonicalID(subject) : null;
  const { profile, error: profileError } = useEntityProfile(subject?.type ?? null, canonicalID);
  const { neighborhood } = useEntityNeighborhood(subject?.type ?? null, canonicalID, { depth: 2 });
  const { messages, next } = useEntityTimeline(subject?.type ?? null, canonicalID, { limit: 50 });
  const { provenance } = useEntityProvenance(subject?.type ?? null, canonicalID);
  const { flows } = useFlows({ limit: 50, q: canonicalID ?? undefined });
  const { packs } = useOntologyPacks();
  const colors = useMemo(() => edgeColorMap(packs), [packs]);
  const investigationID = profile?.entity.id || (subject ? entityKey(subject) : "entity");
  const saved = useSavedInvestigation(investigationID);

  if (!subject) {
    return (
      <main className="min-h-dvh bg-siem-bg p-6 text-siem-text">
        <EmptyState text="Entity route is missing type or id." />
      </main>
    );
  }

  const selectedEntityID = profile?.entity.id || entityKey(subject);
  const pinnedSubject = saved.isPinned({ type: subject.type, id: canonicalID || subject.id });

  return (
    <main className="h-dvh overflow-hidden bg-siem-bg text-siem-text">
      <div className="mx-auto flex h-full max-w-[1800px] flex-col gap-5 overflow-hidden px-4 py-5 md:px-6">
        <section className="rounded-[28px] border border-siem-border bg-[linear-gradient(135deg,rgba(24,40,53,0.94),rgba(5,10,17,0.96))] p-5 shadow-[0_22px_70px_rgba(0,0,0,0.34)]">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="space-y-3">
              <a href="/" className="inline-flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-siem-muted hover:text-siem-text">
                <ArrowLeft size={14} />
                AgentOps
              </a>
              <div className="inline-flex items-center gap-2 rounded-full border border-siem-accent/30 bg-siem-accent/10 px-3 py-1 text-[11px] uppercase tracking-[0.24em] text-siem-accent">
                <Workflow size={12} />
                {mode === "HYBRID" ? "Fusion Entity" : "Entity"}
              </div>
              <div>
                <h1 className="text-3xl font-semibold tracking-[0.04em]">{profile?.entity.display_name || canonicalID || subject.id}</h1>
                <p className="mt-2 max-w-3xl font-mono text-sm text-siem-muted">{selectedEntityID}</p>
              </div>
            </div>
            <button
              type="button"
              onClick={() => saved.togglePinned({ type: subject.type, id: canonicalID || subject.id })}
              className={`inline-flex items-center gap-2 rounded-2xl border px-4 py-3 text-sm ${pinnedSubject ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border text-siem-muted"}`}
            >
              <Pin size={15} />
              {pinnedSubject ? "Pinned" : "Pin"}
            </button>
          </div>
        </section>

        <section className="grid min-h-0 flex-1 gap-4 overflow-hidden xl:grid-cols-[0.9fr_1.2fr_0.9fr]">
          <Panel title="Identity" icon={ShieldAlert} bodyClassName="overflow-y-auto pr-1">
            <EntityProfileCard profile={profile} packs={packs} onTogglePin={saved.togglePinned} isPinned={saved.isPinned} />
            {profileError ? <div className="mt-3"><EmptyState text={profileError} /></div> : null}
          </Panel>

          <Panel title="Neighborhood Graph" icon={GitBranch} bodyClassName="overflow-y-auto pr-1">
            <GraphCanvas
              entities={neighborhood.entities}
              edges={neighborhood.edges}
              edgeColors={colors}
              selectedEntityId={selectedEntityID}
              onEntityClick={(entity) => { window.location.href = `?view=entity&type=${entity.type}&id=${entityCanonicalID(entity)}`; }}
            />
          </Panel>

          <div className="grid min-h-0 gap-4 overflow-hidden">
            <Panel title="Related Flows" icon={Activity} bodyClassName="overflow-y-auto pr-1">
              <div className="space-y-3">
                {flows.map((flow) => (
                  <div key={flow.id} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate font-semibold">{flow.latest_preview || flow.id}</div>
                        <div className="mt-1 font-mono text-[11px] text-siem-muted">{flow.id}</div>
                      </div>
                      <EntityChip entity={entityRefFromFlow(flow)} pinned={saved.isPinned(entityRefFromFlow(flow))} onTogglePin={saved.togglePinned} />
                    </div>
                    <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                      <Tag>{flow.message_count} messages</Tag>
                      <Tag>{formatTime(flow.last_seen)}</Tag>
                      {flow.latest_status ? <Tag>{flow.latest_status}</Tag> : null}
                    </div>
                  </div>
                ))}
                {flows.length === 0 ? <EmptyState text="No related flows returned for this entity." /> : null}
              </div>
            </Panel>

            <Panel title="Notes" icon={NotebookText} bodyClassName="overflow-y-auto pr-1">
              <textarea
                value={saved.investigation.notes}
                onChange={(event) => saved.setNotes(event.target.value)}
                className="min-h-[180px] w-full resize-none rounded-2xl border border-siem-border bg-siem-bg/45 p-3 text-sm text-siem-text outline-none"
              />
              <div className="mt-3 grid gap-2">
                <StatusRow label="Opened" value={formatTime(saved.investigation.openedAt)} />
                <StatusRow label="Pinned entities" value={String(saved.investigation.pinnedEntities.length)} />
              </div>
            </Panel>
          </div>
        </section>

        <section className="grid min-h-0 flex-[0.8] gap-4 overflow-hidden xl:grid-cols-[1.2fr_0.8fr]">
          <Panel title="Activity Timeline" icon={RadioTower} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-3">
              {messages.map((message) => (
                <div key={message.id} className="space-y-2">
                  <MessageCard message={message} />
                  <button type="button" onClick={() => setDrawerSubject(refForMessage(message) || subject)} className="rounded-full border border-siem-border px-3 py-1 text-[11px] uppercase tracking-[0.18em] text-siem-muted hover:text-siem-text">
                    why
                  </button>
                </div>
              ))}
              {messages.length === 0 ? <EmptyState text="No timeline messages returned for this entity." /> : null}
              {next ? <Tag>more rows available</Tag> : null}
            </div>
          </Panel>

          <Panel title="Provenance" icon={ShieldAlert} bodyClassName="overflow-y-auto pr-1">
            <div className="space-y-3">
              {provenance.map((item, index) => (
                <div key={`${item.subject_id}-${item.stage}-${index}`} className="rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="font-semibold">{item.stage}</div>
                      <div className="mt-1 text-xs text-siem-muted">{formatTime(item.produced_at)}</div>
                    </div>
                    {item.decision ? <Tag>{item.decision}</Tag> : null}
                  </div>
                </div>
              ))}
              {provenance.length === 0 ? <EmptyState text="No provenance rows returned for this entity." /> : null}
            </div>
          </Panel>
        </section>
      </div>
      <ProvenanceDrawer subject={drawerSubject} onClose={() => setDrawerSubject(null)} />
    </main>
  );
}

function refForMessage(message: { task_id?: string; trace_id?: string; sender_id?: string; correlation_id?: string }): EntityRef | null {
  if (message.task_id) return { type: "task", id: message.task_id, label: message.task_id };
  if (message.trace_id) return { type: "trace", id: message.trace_id, label: message.trace_id };
  if (message.sender_id) return { type: "agent", id: message.sender_id, label: message.sender_id };
  if (message.correlation_id) return { type: "correlation", id: message.correlation_id, label: message.correlation_id };
  return null;
}
