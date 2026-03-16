/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useState } from "react";
import {
  Activity,
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
}: Props) {
  const sources = sourceHealth?.sources ?? [];
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
    return [...map.values()].sort((a, b) => b.count - a.count).slice(0, 10);
  }, [regionAlerts]);

  const categoryCounts = useMemo(() => {
    const counts: Partial<Record<AlertCategory, number>> = {};
    for (const a of regionAlerts) {
      counts[a.category] = (counts[a.category] ?? 0) + 1;
    }
    return categoryOrder
      .filter((cat) => (counts[cat] ?? 0) > 0)
      .map((cat) => ({ category: cat, count: counts[cat]! }));
  }, [regionAlerts]);

  const topAuthorities = useMemo(() => {
    const map = new Map<string, { name: string; sourceId: string; count: number }>();
    for (const a of regionAlerts) {
      const key = a.source_id;
      const existing = map.get(key);
      if (existing) existing.count++;
      else map.set(key, { name: a.source.authority_name, sourceId: key, count: 1 });
    }
    return [...map.values()].sort((a, b) => b.count - a.count).slice(0, 10);
  }, [regionAlerts]);

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
        <div className="text-sm text-siem-muted">Loading intelligence data...</div>
      </section>
    );
  }

  return (
    <section className="flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_24px_80px_rgba(0,0,0,0.28)]">
      {/* Header */}
      <div className="border-b border-siem-border/80 px-4 py-3">
        <div className="flex items-center gap-2 text-[11px] uppercase tracking-[0.2em] text-siem-muted">
          <Radar size={12} />
          Intelligence Overview
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-3 space-y-3">
        {/* ── Severity breakdown ──────────────────────────────────── */}
        <div className="grid grid-cols-3 gap-2">
          {[
            { label: "Critical", value: severityCounts.critical, icon: AlertTriangle, tone: "text-rose-300" },
            { label: "High", value: severityCounts.high, icon: ShieldAlert, tone: "text-amber-300" },
            { label: "Active", value: regionAlerts.filter((a) => a.status === "active").length, icon: Activity, tone: "text-emerald-300" },
          ].map((card) => (
            <div
              key={card.label}
              className="rounded-xl border border-siem-border bg-siem-panel-strong px-3 py-2.5"
            >
              <card.icon size={13} className={card.tone} />
              <div className={`mt-1 text-lg font-semibold ${card.tone}`}>{card.value}</div>
              <div className="text-[10px] uppercase tracking-[0.16em] text-siem-muted">{card.label}</div>
            </div>
          ))}
        </div>

        {/* ── Feed health summary ─────────────────────────────────── */}
        <div className="grid grid-cols-2 gap-2">
          <div className="rounded-xl border border-siem-border bg-siem-panel-strong px-3 py-2.5">
            <div className="flex items-center gap-2">
              <Globe2 size={13} className="text-siem-accent" />
              <span className="text-lg font-semibold text-siem-text">
                {new Set(regionAlerts.map((a) => a.source.country_code)).size}
              </span>
            </div>
            <div className="text-[10px] uppercase tracking-[0.16em] text-siem-muted">Countries</div>
          </div>
          <div className="rounded-xl border border-siem-border bg-siem-panel-strong px-3 py-2.5">
            <div className="flex items-center gap-2">
              <Activity size={13} className="text-emerald-300" />
              <span className="text-lg font-semibold text-siem-text">
                {sourceHealth?.sources_ok ?? 0}/{sourceHealth?.total_sources ?? 0}
              </span>
            </div>
            <div className="text-[10px] uppercase tracking-[0.16em] text-siem-muted">
              Feeds OK
            </div>
          </div>
        </div>

        {/* ── Top authorities (clickable to scope) ────────────────── */}
        <div className="rounded-xl border border-siem-border bg-siem-panel px-3 py-3">
          <div className="mb-2.5 flex items-center gap-2 text-[10px] uppercase tracking-[0.16em] text-siem-muted">
            <TrendingUp size={11} />
            Top authorities
          </div>
          <div className="space-y-1">
            {selectedSourceIds.length > 0 && (
              <button
                type="button"
                onClick={() => onSelectSourceIdsChange([])}
                className="w-full flex items-center justify-between gap-2 rounded-lg border border-siem-accent px-2.5 py-1.5 text-left text-xs bg-siem-accent/14 text-siem-text transition-colors"
              >
                <span className="truncate">All feeds (clear filter)</span>
              </button>
            )}
            {topAuthorities.map((auth) => (
              <button
                key={auth.sourceId}
                type="button"
                onClick={() => toggleSource(auth.sourceId)}
                className={`w-full flex items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-left text-xs transition-colors ${
                  selectedSourceIds.includes(auth.sourceId)
                    ? "border-siem-accent bg-siem-accent/14 text-siem-text"
                    : "border-siem-border bg-siem-panel-strong text-siem-text hover:border-siem-accent/40 hover:bg-siem-accent/8"
                }`}
              >
                <span className="truncate">{auth.name}</span>
                <span className="shrink-0 text-[10px] text-siem-muted">{auth.count}</span>
              </button>
            ))}
          </div>
        </div>

        {/* ── Category breakdown ──────────────────────────────────── */}
        <div className="rounded-xl border border-siem-border bg-siem-panel px-3 py-3">
          <div className="mb-2.5 flex items-center justify-between gap-2">
            <div className="text-[10px] uppercase tracking-[0.16em] text-siem-muted">
              Categories
            </div>
            {categoryFilter !== "all" && (
              <button
                type="button"
                onClick={() => onSelectCategory("all")}
                className="rounded border border-siem-accent bg-siem-accent/14 px-1.5 py-0.5 text-[10px] uppercase tracking-[0.14em] text-siem-text"
              >
                All
              </button>
            )}
          </div>
          <div className="space-y-1.5">
            {categoryCounts.map(({ category, count }) => (
              <button
                key={category}
                type="button"
                onClick={() => onSelectCategory(categoryFilter === category ? "all" : category)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-left transition-colors ${
                  categoryFilter === category
                    ? "border-siem-accent bg-siem-accent/14 text-siem-text"
                    : "border-siem-border bg-siem-panel-strong text-siem-text hover:border-siem-accent/40 hover:bg-siem-accent/8"
                }`}
              >
                <span
                  className={`inline-flex items-center px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider rounded border ${categoryBadge[category]}`}
                >
                  {categoryLabels[category]}
                </span>
                <span className="text-xs text-siem-muted">{count}</span>
              </button>
            ))}
          </div>
        </div>

        {/* ── Top countries ───────────────────────────────────────── */}
        <div className="rounded-xl border border-siem-border bg-siem-panel px-3 py-3">
          <div className="mb-2.5 text-[10px] uppercase tracking-[0.16em] text-siem-muted">
            Top countries
          </div>
          <div className="space-y-1">
            {countryCounts.map((c) => (
              <button
                key={c.code}
                type="button"
                onClick={() => onSelectCountry(c.code)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-left text-xs transition-colors ${
                  regionFilter === `country:${c.code}`
                    ? "border-siem-accent bg-siem-accent/14 text-siem-text"
                    : "border-siem-border bg-siem-panel-strong text-siem-text hover:border-siem-accent/40 hover:bg-siem-accent/8"
                }`}
              >
                <span className="truncate text-siem-text">
                  {c.name}{" "}
                  <span className="text-siem-muted">({c.code})</span>
                </span>
                <span className="shrink-0 text-siem-muted">{c.count}</span>
              </button>
            ))}
          </div>
        </div>

      </div>

      {snapshotStatus && (
        <div className="border-t border-siem-border/80 px-4 py-2.5">
          <div className={`text-[10px] uppercase tracking-[0.16em] ${snapshotStatus.tone}`}>
            Data snapshot: {snapshotStatus.relative}
          </div>
          <div className="mt-1 text-[11px] text-siem-muted">
            Last collector update: {snapshotStatus.exact}
          </div>
        </div>
      )}
    </section>
  );
}
