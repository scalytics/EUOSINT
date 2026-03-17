/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Globe2,
  Radar,
  ShieldAlert,
  TrendingUp,
} from "lucide-react";
import type { Alert, AlertCategory } from "@/types/alert";
import type { SourceHealthDocument } from "@/types/source-health";
import { categoryLabels, categoryBadge, categoryOrder } from "@/lib/severity";
import { alertMatchesRegionFilter } from "@/lib/regions";

type View = "overview" | "feeds" | "authorities" | "health";
type SeverityFilter = "critical" | "high" | null;

interface Props {
  view: View;
  alerts: Alert[];
  sourceHealth: SourceHealthDocument | null;
  isLoading: boolean;
  selectedSourceIds: string[];
  onSelectSourceIdsChange: (sourceIds: string[]) => void;
  categoryFilter: AlertCategory | "all";
  onSelectCategory: (category: AlertCategory | "all") => void;
  regionFilter: string;
  onSelectCountry: (countryCode: string) => void;
  severityFilter: SeverityFilter;
  onSeverityFilterChange: (filter: SeverityFilter) => void;
}

export function FeedDirectory({
  alerts,
  sourceHealth,
  isLoading,
  selectedSourceIds,
  onSelectSourceIdsChange,
  categoryFilter,
  onSelectCategory,
  regionFilter,
  onSelectCountry,
  severityFilter,
  onSeverityFilterChange,
}: Props) {
  const sources = useMemo(() => sourceHealth?.sources ?? [], [sourceHealth]);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(timer);
  }, []);

  /* ── Region-scoped alerts ──────────────────────────────────────── */

  const regionAlerts = useMemo(
    () => regionFilter === "all" ? alerts : alerts.filter((a) => alertMatchesRegionFilter(a, regionFilter)),
    [alerts, regionFilter],
  );

  /* ── Derived stats (all from region-scoped alerts) ─────────────── */

  const severityCounts = useMemo(() => {
    const counts = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
    for (const a of regionAlerts) {
      counts[a.severity] = (counts[a.severity] ?? 0) + 1;
    }
    return counts;
  }, [regionAlerts]);

  const countryCounts = useMemo(() => {
    const map = new Map<string, { name: string; code: string; count: number }>();
    for (const a of regionAlerts) {
      const key = a.source.country_code;
      const existing = map.get(key);
      if (existing) existing.count++;
      else map.set(key, { name: a.source.country, code: key, count: 1 });
    }
    return [...map.values()].sort((a, b) => b.count - a.count).slice(0, 12);
  }, [regionAlerts]);

  const categoryCounts = useMemo(() => {
    const counts: Partial<Record<AlertCategory, number>> = {};
    for (const a of regionAlerts) {
      counts[a.category] = (counts[a.category] ?? 0) + 1;
    }
    return categoryOrder
      .filter((cat) => cat !== "informational" && (counts[cat] ?? 0) > 0)
      .map((cat) => ({ category: cat, count: counts[cat]! }));
  }, [regionAlerts]);

  const topAuthorities = useMemo(() => {
    const map = new Map<string, { name: string; sourceId: string; count: number; maxItems: number }>();
    for (const a of regionAlerts) {
      const key = a.source_id;
      const existing = map.get(key);
      if (existing) existing.count++;
      else map.set(key, { name: a.source.authority_name, sourceId: key, count: 1, maxItems: 0 });
    }
    return [...map.values()].sort((a, b) => b.count - a.count).slice(0, 12);
  }, [regionAlerts]);

  /* ── Zone summary stats ─────────────────────────────────────────── */
  const zoneSummary = useMemo(() => {
    const uniqueCountries = new Set(regionAlerts.map((a) => a.source.country_code));
    const uniqueFeeds = new Set(regionAlerts.map((a) => a.source_id));
    // For global view, use the health document's total which includes sources
    // that returned 0 alerts (errors, empty feeds, etc.).
    const feedCount =
      regionFilter === "all" && sources.length > 0
        ? sources.length
        : uniqueFeeds.size;
    return {
      alerts: regionAlerts.length,
      countries: uniqueCountries.size,
      feeds: feedCount,
    };
  }, [regionAlerts, regionFilter, sources]);

  const toggleSource = (sourceId: string) => {
    if (selectedSourceIds.includes(sourceId)) {
      onSelectSourceIdsChange(selectedSourceIds.filter((id) => id !== sourceId));
      return;
    }
    onSelectSourceIdsChange([...selectedSourceIds, sourceId]);
  };

  const snapshotStatus = useMemo(() => {
    const generatedAt = sourceHealth?.generated_at?.trim();
    if (!generatedAt) {
      return null;
    }
    const ts = Date.parse(generatedAt);
    if (Number.isNaN(ts)) {
      return null;
    }
    const ageMinutes = Math.max(0, Math.floor((now - ts) / 60_000));
    const tone =
      ageMinutes > 60 ? "text-rose-300" : ageMinutes >= 20 ? "text-amber-300" : "text-siem-muted";
    const relative =
      ageMinutes < 1 ? "just now" : ageMinutes === 1 ? "1 min old" : `${ageMinutes} min old`;
    const exact = new Intl.DateTimeFormat(undefined, {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      timeZone: "UTC",
      timeZoneName: "short",
      hour12: false,
    }).format(new Date(ts));
    return { tone, relative, exact };
  }, [now, sourceHealth?.generated_at]);

  if (isLoading && !sourceHealth && regionAlerts.length === 0) {
    return (
      <section className="flex h-full items-center justify-center rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_24px_80px_rgba(0,0,0,0.28)]">
        <div className="text-2xs text-siem-muted">Loading intelligence data...</div>
      </section>
    );
  }

  return (
    <section className="flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_24px_80px_rgba(0,0,0,0.28)]">
      {/* Header */}
      <div className="border-b border-siem-border/80 px-4 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-3xs uppercase tracking-[0.2em] text-siem-muted">
            <Radar size={10} />
            {regionFilter === "all" ? "Global Overview" : regionFilter.startsWith("country:") ? regionFilter.slice(8) : regionFilter}
          </div>
          {severityFilter && (
            <span className="rounded border border-siem-accent/40 bg-siem-accent/10 px-1.5 py-0.5 text-3xs uppercase tracking-[0.14em] text-siem-accent">
              {severityFilter}
            </span>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-2 space-y-1.5">
        {/* ── Stat strip: severity + zone ──────────────────────────── */}
        <div className="grid grid-cols-5 gap-px rounded-md border border-siem-border bg-siem-border overflow-hidden">
          {([
            { key: "critical" as SeverityFilter, label: "Crit", value: severityCounts.critical, icon: AlertTriangle, tone: "text-rose-300" },
            { key: "high" as SeverityFilter, label: "High", value: severityCounts.high, icon: ShieldAlert, tone: "text-amber-300" },
            { key: null as SeverityFilter, label: "Confl", value: regionAlerts.filter((a) => a.category === "conflict_monitoring").length, icon: Radar, tone: "text-orange-300" },
          ] as const).map((card) => (
            <button
              key={card.label}
              type="button"
              onClick={() => {
                if (card.key !== null) {
                  onSeverityFilterChange(severityFilter === card.key ? null : card.key);
                } else {
                  onSelectCategory(categoryFilter === "conflict_monitoring" ? "all" : "conflict_monitoring");
                }
              }}
              className={`flex flex-col items-center py-1.5 transition-colors ${
                (severityFilter === card.key && card.key !== null) || (card.key === null && categoryFilter === "conflict_monitoring")
                  ? "bg-siem-accent/14"
                  : "bg-siem-panel-strong hover:bg-siem-accent/8"
              }`}
            >
              <div className={`text-xs font-bold tabular-nums ${card.tone}`}>{card.value}</div>
              <div className="text-3xs uppercase tracking-[0.1em] text-siem-muted">{card.label}</div>
            </button>
          ))}
          <div className="flex flex-col items-center py-1.5 bg-siem-panel-strong">
            <div className="text-xs font-bold tabular-nums text-siem-accent">{zoneSummary.countries}</div>
            <div className="text-3xs uppercase tracking-[0.1em] text-siem-muted">Ctry</div>
          </div>
          <div className="flex flex-col items-center py-1.5 bg-siem-panel-strong">
            <div className="text-xs font-bold tabular-nums text-emerald-300">{zoneSummary.feeds}</div>
            <div className="text-3xs uppercase tracking-[0.1em] text-siem-muted">Feeds</div>
          </div>
        </div>

        {/* ── Category breakdown ──────────────────────────────────── */}
        <div>
          <div className="mb-1 flex items-center justify-between">
            <span className="text-3xs uppercase tracking-[0.16em] text-siem-muted">Categories</span>
            {categoryFilter !== "all" && (
              <button
                type="button"
                onClick={() => onSelectCategory("all")}
                className="text-3xs uppercase tracking-[0.12em] text-siem-accent hover:text-siem-text transition-colors"
              >
                Clear
              </button>
            )}
          </div>
          <div className="space-y-px">
            {categoryCounts.map(({ category, count }) => (
              <button
                key={category}
                type="button"
                onClick={() => onSelectCategory(categoryFilter === category ? "all" : category)}
                className={`flex w-full items-center justify-between gap-1.5 rounded px-1.5 py-[3px] text-left transition-colors ${
                  categoryFilter === category
                    ? "bg-siem-accent/14 text-siem-text"
                    : "text-siem-text hover:bg-siem-accent/8"
                }`}
              >
                <span
                  className={`inline-flex items-center px-1 py-px text-3xs font-semibold uppercase tracking-wider rounded border leading-tight ${categoryBadge[category]}`}
                >
                  {categoryLabels[category]}
                </span>
                <span className="text-3xs tabular-nums text-siem-muted">{count}</span>
              </button>
            ))}
          </div>
        </div>

        {/* ── Top authorities ──────────────────────────────────────── */}
        <div>
          <div className="mb-1 flex items-center gap-1 text-3xs uppercase tracking-[0.16em] text-siem-muted">
            <TrendingUp size={8} />
            Authorities
            {selectedSourceIds.length > 0 && (
              <button
                type="button"
                onClick={() => onSelectSourceIdsChange([])}
                className="ml-auto text-3xs uppercase tracking-[0.12em] text-siem-accent hover:text-siem-text transition-colors"
              >
                Clear
              </button>
            )}
          </div>
          <div className="space-y-px">
            {topAuthorities.map((auth) => {
              const commonCaps = [15, 20, 40, 60, 80, 100];
              const likelyCapped = commonCaps.includes(auth.count);
              return (
                <button
                  key={auth.sourceId}
                  type="button"
                  onClick={() => toggleSource(auth.sourceId)}
                  className={`w-full flex items-center justify-between gap-1.5 rounded px-1.5 py-[3px] text-left text-3xs transition-colors ${
                    selectedSourceIds.includes(auth.sourceId)
                      ? "bg-siem-accent/14 text-siem-text"
                      : "text-siem-text hover:bg-siem-accent/8"
                  }`}
                >
                  <span className="truncate">{auth.name}</span>
                  <span className="shrink-0 tabular-nums text-siem-muted" title={likelyCapped ? "May have more (per-source limit)" : undefined}>
                    {likelyCapped ? `>${auth.count}` : auth.count}
                  </span>
                </button>
              );
            })}
          </div>
        </div>

        {/* ── Top countries ───────────────────────────────────────── */}
        <div>
          <div className="mb-1 text-3xs uppercase tracking-[0.16em] text-siem-muted">Countries</div>
          <div className="space-y-px">
            {countryCounts.map((c) => (
              <button
                key={c.code}
                type="button"
                onClick={() => onSelectCountry(c.code)}
                className={`flex w-full items-center justify-between gap-1.5 rounded px-1.5 py-[3px] text-left text-3xs transition-colors ${
                  regionFilter === `country:${c.code}`
                    ? "bg-siem-accent/14 text-siem-text"
                    : "text-siem-text hover:bg-siem-accent/8"
                }`}
              >
                <span className="truncate">
                  {c.name} <span className="text-siem-muted">{c.code}</span>
                </span>
                <span className="shrink-0 tabular-nums text-siem-muted">{c.count}</span>
              </button>
            ))}
          </div>
        </div>

      </div>

      {snapshotStatus && (
        <div className="border-t border-siem-border/80 px-3 py-2">
          <div className={`text-3xs uppercase tracking-[0.16em] ${snapshotStatus.tone}`}>
            Data snapshot: {snapshotStatus.relative}
          </div>
          <div className="mt-0.5 text-3xs text-siem-muted">
            Last collector update: {snapshotStatus.exact}
          </div>
        </div>
      )}
    </section>
  );
}
