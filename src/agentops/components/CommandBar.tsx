import { Search, X } from "lucide-react";
import { EmptyState, Tag } from "@/agentops/components/Chrome";
import { EntityChip } from "@/agentops/components/EntityChip";
import { entityFromSearchResult } from "@/agentops/lib/entities";
import type { SearchResult } from "@/agentops/types";

export function CommandBar({
  value,
  results,
  isLoading,
  onChange,
  onSubmit,
  onClear,
  onFlowSelect,
}: {
  value: string;
  results: SearchResult[];
  isLoading?: boolean;
  onChange: (value: string) => void;
  onSubmit: () => void;
  onClear: () => void;
  onFlowSelect: (id: string) => void;
}) {
  return (
    <div className="space-y-3 rounded-2xl border border-siem-border bg-siem-panel/55 p-3">
      <form
        className="flex items-center gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <Search size={15} className="shrink-0 text-siem-accent" />
        <input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder="agent:alice topic:requests window:1h"
          className="min-w-0 flex-1 bg-transparent text-sm text-siem-text outline-none placeholder:text-siem-muted"
        />
        {value ? (
          <button type="button" aria-label="Clear command search" onClick={onClear} className="rounded-full p-1 text-siem-muted hover:text-siem-text">
            <X size={14} />
          </button>
        ) : null}
      </form>
      {isLoading ? <div className="text-xs text-siem-muted">Searching.</div> : null}
      {results.length > 0 ? (
        <div className="space-y-2">
          {results.map((result) => <SearchHit key={`${result.kind}-${result.id}`} result={result} onFlowSelect={onFlowSelect} />)}
        </div>
      ) : value && !isLoading ? <EmptyState text="No command results." /> : null}
    </div>
  );
}

function SearchHit({ result, onFlowSelect }: { result: SearchResult; onFlowSelect: (id: string) => void }) {
  const entity = entityFromSearchResult(result);
  if (entity) {
    return (
      <div className="flex items-center justify-between gap-3 rounded-2xl border border-siem-border bg-siem-bg/35 px-3 py-2">
        <EntityChip entity={entity} />
        <Tag>{result.type}</Tag>
      </div>
    );
  }
  if (result.kind === "flow") {
    return (
      <button type="button" onClick={() => onFlowSelect(result.id)} className="block w-full rounded-2xl border border-siem-border bg-siem-bg/35 px-3 py-2 text-left hover:border-siem-accent/35">
        <div className="flex items-start justify-between gap-3">
          <span className="min-w-0 truncate font-semibold text-siem-text">{result.title || result.id}</span>
          {result.latest_status ? <Tag>{result.latest_status}</Tag> : null}
        </div>
        <div className="mt-1 font-mono text-[11px] text-siem-muted">{result.id}</div>
      </button>
    );
  }
  return (
    <div className="rounded-2xl border border-siem-border bg-siem-bg/35 px-3 py-2">
      <div className="flex items-start justify-between gap-3">
        <span className="font-semibold text-siem-text">{result.title || result.detector_id || result.id}</span>
        {result.severity ? <Tag>{result.severity}</Tag> : null}
      </div>
      {result.source ? <div className="mt-1 text-[11px] text-siem-muted">{result.source}</div> : null}
    </div>
  );
}
