import { Pin, PinOff } from "lucide-react";
import type { EntityRef } from "@/agentops/lib/entities";
import { entityHref, entityKey, entityLabel } from "@/agentops/lib/entities";

export function EntityChip({
  entity,
  pinned = false,
  onTogglePin,
}: {
  entity: EntityRef;
  pinned?: boolean;
  onTogglePin?: (entity: EntityRef) => void;
}) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-siem-border bg-siem-bg/35 px-2 py-1">
      <a href={entityHref(entity)} className="max-w-[220px] truncate font-mono text-[11px] text-siem-text hover:text-siem-accent" title={entityKey(entity)}>
        {entityLabel(entity)}
      </a>
      {onTogglePin ? (
        <button
          type="button"
          aria-label={`${pinned ? "Unpin" : "Pin"} ${entityLabel(entity)}`}
          onClick={() => onTogglePin(entity)}
          className="rounded-full p-0.5 text-siem-muted transition hover:text-siem-accent"
        >
          {pinned ? <PinOff size={11} /> : <Pin size={11} />}
        </button>
      ) : null}
    </span>
  );
}
