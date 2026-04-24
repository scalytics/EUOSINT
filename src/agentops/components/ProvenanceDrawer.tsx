import { X } from "lucide-react";
import { EmptyState, Tag } from "@/agentops/components/Chrome";
import type { EntityRef } from "@/agentops/lib/entities";
import { entityCanonicalID, entityKey } from "@/agentops/lib/entities";
import { formatTime } from "@/agentops/lib/view";
import { useEntityProvenance } from "@/hooks/useAgentOpsApi";

export function ProvenanceDrawer({ subject, onClose }: { subject: EntityRef | null; onClose: () => void }) {
  const { provenance, isLoading, error } = useEntityProvenance(subject?.type ?? null, subject ? entityCanonicalID(subject) : null);
  if (!subject) return null;

  return (
    <aside className="fixed inset-y-0 right-0 z-[60] flex w-full max-w-[520px] flex-col border-l border-siem-border bg-siem-panel p-5 shadow-[0_0_60px_rgba(0,0,0,0.42)] animate-slide-in">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-[11px] uppercase tracking-[0.22em] text-siem-muted">Provenance</div>
          <div className="mt-2 font-mono text-sm text-siem-text">{entityKey(subject)}</div>
        </div>
        <button type="button" aria-label="Close provenance" onClick={onClose} className="rounded-full border border-siem-border p-2 text-siem-muted hover:text-siem-text">
          <X size={16} />
        </button>
      </div>
      <div className="mt-5 min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
        {isLoading ? <EmptyState text="Loading provenance." /> : null}
        {error ? <EmptyState text={error} /> : null}
        {!isLoading && provenance.length === 0 ? <EmptyState text="No provenance rows returned for this subject." /> : null}
        {provenance.map((item, index) => (
          <div key={`${item.subject_id}-${item.stage}-${item.produced_at}-${index}`} className="rounded-2xl border border-siem-border bg-siem-bg/45 p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <div className="font-semibold text-siem-text">{item.stage}</div>
                <div className="mt-1 text-xs text-siem-muted">{formatTime(item.produced_at)}</div>
              </div>
              {item.decision ? <Tag>{item.decision}</Tag> : null}
            </div>
            {item.reasons?.length ? (
              <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
                {item.reasons.map((reason) => <Tag key={reason}>{reason}</Tag>)}
              </div>
            ) : null}
            {item.inputs ? <pre className="mt-3 max-h-48 overflow-auto rounded-2xl bg-siem-panel/55 p-3 text-xs text-siem-muted">{JSON.stringify(item.inputs, null, 2)}</pre> : null}
          </div>
        ))}
      </div>
    </aside>
  );
}
