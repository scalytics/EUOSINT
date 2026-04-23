import { EmptyState, StatusRow, Tag } from "@/agentops/components/Chrome";
import { formatTime } from "@/agentops/lib/view";
import type { Pack, Profile, View } from "@/agentops/lib/api-client/types";

function findView(packs: Pack[], entityType: string): { pack: Pack; view: View } | null {
  for (const pack of packs) {
    const view = pack.views?.find((item) => item.entity_type === entityType);
    if (view) return { pack, view };
  }
  return null;
}

function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === "") return "-";
  if (Array.isArray(value)) return value.map((item) => displayValue(item)).join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

export function EntityProfileCard({ profile, packs }: { profile: Profile | null; packs: Pack[] }) {
  if (!profile) {
    return <EmptyState text="Select an entity route to inspect a pack-defined profile." />;
  }

  const attrs = profile.entity.attrs ?? {};
  const match = findView(packs, profile.entity.type);
  const fieldRows = match?.view.fields?.length
    ? match.view.fields.map((field) => ({
        key: field.id,
        label: field.label || field.id,
        value: displayValue(attrs[field.id]),
      }))
    : Object.entries(attrs).map(([key, value]) => ({
        key,
        label: key,
        value: displayValue(value),
      }));

  return (
    <div className="space-y-4">
      <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">
              {match ? `${match.pack.name} pack` : "Core entity"}
            </div>
            <div className="mt-2 text-lg font-semibold">{profile.entity.display_name || profile.entity.canonical_id}</div>
            <div className="mt-1 font-mono text-xs text-siem-muted">{profile.entity.id}</div>
          </div>
          <Tag>{match?.view.title || profile.entity.type}</Tag>
        </div>
        <div className="mt-3 grid gap-2 text-sm text-siem-muted">
          <div>First seen: {formatTime(profile.first_seen)}</div>
          <div>Last seen: {formatTime(profile.last_seen)}</div>
        </div>
      </div>

      <div className="grid gap-3">
        {fieldRows.length > 0 ? fieldRows.map((field) => <StatusRow key={field.key} label={field.label} value={field.value} />) : <EmptyState text="No pack fields declared for this entity." />}
      </div>

      <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
        <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Edge Counts</div>
        <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-siem-muted">
          {Object.entries(profile.edge_counts).map(([edgeType, count]) => <Tag key={edgeType}>{edgeType}:{count}</Tag>)}
          {Object.keys(profile.edge_counts).length === 0 ? <Tag>none</Tag> : null}
        </div>
      </div>

      <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
        <div className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">Top Neighbors</div>
        <div className="mt-3 space-y-2">
          {profile.top_neighbors.length > 0 ? profile.top_neighbors.map((neighbor) => (
            <div key={neighbor.entity_id} className="flex items-center justify-between gap-3 rounded-2xl border border-siem-border bg-siem-panel/55 px-3 py-2 text-sm">
              <span>{neighbor.entity_type}</span>
              <span className="font-mono text-xs text-siem-muted">{neighbor.entity_id}</span>
            </div>
          )) : <EmptyState text="No graph neighbors ranked for this entity yet." />}
        </div>
      </div>
    </div>
  );
}
