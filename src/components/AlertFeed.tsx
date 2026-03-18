/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import type { Alert, AlertCategory, Severity } from "@/types/alert";
import {
  severityColor,
  severityBg,
  severityLabel,
  categoryLabels,
  categoryBadge,
  freshnessLabel,
} from "@/lib/severity";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { Clock, Building2, ChevronDown, Globe } from "lucide-react";

const LIVE_WINDOW_MS = 48 * 60 * 60 * 1000; // 48 hours

interface Props {
  alerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  categoryFilter: AlertCategory | "all";
  onCategoryChange: (category: AlertCategory | "all") => void;
  regionFilter: string;
  onRegionChange: (region: string) => void;
  onVisibleAlertIdsChange: (ids: string[]) => void;
}

export function AlertFeed({
  alerts,
  selectedId,
  onSelect,
  categoryFilter,
  onCategoryChange,
  regionFilter,
  onRegionChange,
  onVisibleAlertIdsChange,
}: Props) {
  const [viewMode, setViewMode] = useState<"live" | "history">("live");
  const [severityFilter, setSeverityFilter] = useState<Severity | "all">("all");
  const [isRefreshingList, setIsRefreshingList] = useState(false);
  const [newAlertIds, setNewAlertIds] = useState<Set<string>>(new Set());
  const knownAlertIdsRef = useRef<Set<string>>(new Set());
  const lastVisibleSigRef = useRef("");
  const refreshTimeoutRef = useRef<number | null>(null);
  const glowTimeoutsRef = useRef<number[]>([]);

  const regions = useMemo(() => {
    const set = new Map<string, number>();
    alerts.forEach((a) => {
      const r = a.source.region;
      set.set(r, (set.get(r) ?? 0) + 1);
    });
    return [...set.entries()].sort((a, b) => b[1] - a[1]);
  }, [alerts]);

  const countries = useMemo(() => {
    const set = new Map<string, { name: string; count: number }>();
    alerts.forEach((a) => {
      const key = a.source.country_code;
      const existing = set.get(key);
      if (existing) {
        existing.count++;
      } else {
        set.set(key, { name: a.source.country, count: 1 });
      }
    });
    return [...set.entries()]
      .map(([code, { name, count }]) => ({ code, name, count }))
      .sort((a, b) => b.count - a.count);
  }, [alerts]);

  const regionFiltered =
    regionFilter === "all"
      ? alerts
      : alerts.filter((a) => alertMatchesRegionFilter(a, regionFilter));

  const facetFiltered = regionFiltered.filter((a) => {
    const categoryMatch = categoryFilter === "all" || a.category === categoryFilter;
    const severityMatch = severityFilter === "all" || a.severity === severityFilter;
    return categoryMatch && severityMatch;
  });

  // Split alerts into live (last 48h) and history (older).
  const now = useMemo(() => Date.now(), [facetFiltered]);
  const liveCutoff = now - LIVE_WINDOW_MS;

  const liveAlerts = useMemo(
    () =>
      facetFiltered
        .filter((a) => {
          const t = new Date(a.last_seen).getTime();
          return t >= liveCutoff;
        })
        .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
    [facetFiltered, liveCutoff],
  );

  const historyAlerts = useMemo(
    () =>
      facetFiltered
        .filter((a) => {
          const t = new Date(a.last_seen).getTime();
          return t < liveCutoff;
        })
        .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
    [facetFiltered, liveCutoff],
  );

  // History grouped by day.
  const historyGroups = useMemo(() => {
    const todayStart = new Date();
    todayStart.setHours(0, 0, 0, 0);
    const dayMs = 86_400_000;
    const buckets: { label: string; alerts: Alert[] }[] = [
      { label: "2-3 days ago", alerts: [] },
      { label: "This week", alerts: [] },
      { label: "Last week", alerts: [] },
      { label: "Older", alerts: [] },
    ];

    for (const alert of historyAlerts) {
      const t = new Date(alert.last_seen).getTime();
      const age = todayStart.getTime() - t;
      if (age < 3 * dayMs) buckets[0].alerts.push(alert);
      else if (age < 7 * dayMs) buckets[1].alerts.push(alert);
      else if (age < 14 * dayMs) buckets[2].alerts.push(alert);
      else buckets[3].alerts.push(alert);
    }

    return buckets.filter((b) => b.alerts.length > 0);
  }, [historyAlerts]);

  // Keep globe visibility aligned — only live alerts show pins.
  const visibleAlertIds = useMemo(
    () => liveAlerts.filter((a) => a.severity !== "info").map((a) => a.alert_id),
    [liveAlerts],
  );

  useEffect(() => {
    const glowTimeouts = glowTimeoutsRef.current;
    return () => {
      if (refreshTimeoutRef.current) {
        window.clearTimeout(refreshTimeoutRef.current);
      }
      glowTimeouts.forEach((id) => window.clearTimeout(id));
    };
  }, []);

  useEffect(() => {
    const currentIds = new Set(alerts.map((a) => a.alert_id));
    const previousIds = knownAlertIdsRef.current;
    const hasPreviousSnapshot = previousIds.size > 0;

    if (hasPreviousSnapshot) {
      if (refreshTimeoutRef.current) {
        window.clearTimeout(refreshTimeoutRef.current);
      }
      setIsRefreshingList(true);
      refreshTimeoutRef.current = window.setTimeout(() => {
        setIsRefreshingList(false);
      }, 160);
    }

    const incoming = alerts
      .filter((a) => !previousIds.has(a.alert_id))
      .map((a) => a.alert_id);

    if (hasPreviousSnapshot && incoming.length > 0) {
      setNewAlertIds((prev) => {
        const next = new Set(prev);
        incoming.forEach((id) => next.add(id));
        return next;
      });

      const clearId = window.setTimeout(() => {
        setNewAlertIds((prev) => {
          const next = new Set(prev);
          incoming.forEach((id) => next.delete(id));
          return next;
        });
      }, 2200);
      glowTimeoutsRef.current.push(clearId);
    }

    knownAlertIdsRef.current = currentIds;
  }, [alerts]);

  const severityRail = (s: Severity) => severityColor(s);

  useEffect(() => {
    const sig = visibleAlertIds.join("|");
    if (sig === lastVisibleSigRef.current) return;
    lastVisibleSigRef.current = sig;
    onVisibleAlertIdsChange(visibleAlertIds);
  }, [visibleAlertIds, onVisibleAlertIdsChange]);

  const renderAlertCard = (alert: Alert, position: number) => {
    const isSelected = selectedId === alert.alert_id;
    const isNew = newAlertIds.has(alert.alert_id);

    // Age-based opacity: < 6h full, 6-24h slightly faded, 24-48h faded.
    const ageMs = now - new Date(alert.last_seen).getTime();
    const ageOpacity =
      ageMs < 6 * 3600_000 ? "" : ageMs < 24 * 3600_000 ? "opacity-85" : "opacity-65";

    return (
      <button
        key={alert.alert_id}
        onClick={() => onSelect(alert.alert_id)}
        className={`relative w-full text-left rounded-lg border border-siem-border px-3 py-2.5 pl-4 bg-siem-bg/45 transition-colors hover:bg-siem-accent/8 ${ageOpacity} ${
          isSelected ? "bg-siem-accent/10 border-siem-accent/45" : ""
        } ${isNew ? "animate-alert-new-glow" : ""}`}
      >
        <span
          className="absolute left-0 top-0 h-full w-1 rounded-l-lg opacity-90"
          style={{ backgroundColor: severityRail(alert.severity) }}
          aria-hidden
        />
        <div className="flex items-center justify-between gap-2 mb-1.5">
          <div className="flex items-center gap-1.5">
            <span
              className={`inline-flex items-center px-1.5 py-0.5 text-2xs font-bold uppercase tracking-wider rounded border ${
                severityBg[alert.severity]
              } ${alert.severity === "critical" ? "animate-critical-badge" : ""}`}
            >
              {severityLabel[alert.severity]}
            </span>
            {isNew && (
              <span className="inline-flex items-center px-1.5 py-0.5 text-2xs uppercase tracking-wider rounded border border-siem-accent/35 text-siem-accent bg-siem-accent/12">
                New
              </span>
            )}
            {typeof alert.triage?.relevance_score === "number" && (
              <span className="inline-flex items-center px-1.5 py-0.5 text-2xs uppercase tracking-wider rounded border border-siem-border text-siem-muted bg-white/5">
                Rel {Math.round(alert.triage.relevance_score * 100)}
              </span>
            )}
          </div>
          <span className="text-2xs text-siem-muted font-mono uppercase tracking-wider">
            #{position + 1}
          </span>
        </div>
        <p className="text-sm text-siem-text leading-snug line-clamp-2 mb-2">{alert.title}</p>
        <div className="flex items-center justify-between gap-2 text-2xs text-siem-muted font-mono uppercase tracking-wide">
          <span className="flex items-center gap-1 min-w-0">
            <Building2 size={10} />
            <span className="truncate">{alert.source.authority_name}</span>
          </span>
          <span className="flex items-center gap-1 shrink-0">
            <Clock size={10} />
            {freshnessLabel(alert.freshness_hours)}
          </span>
        </div>
        <div className="mt-1.5 flex items-center gap-1.5 text-2xs">
          <span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 bg-siem-accent/10 text-siem-accent border border-siem-accent/20">
            <Globe size={9} />
            {alert.source.region}
          </span>
          <span
            className={`inline-flex items-center px-1.5 py-0.5 rounded border ${categoryBadge[alert.category]}`}
            style={{ fontSize: "0.6rem" }}
          >
            {categoryLabels[alert.category]}
          </span>
        </div>
      </button>
    );
  };

  const currentAlerts = viewMode === "live" ? liveAlerts : historyAlerts;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="px-3 py-3 border-b border-siem-border bg-siem-panel/95 space-y-2.5">
        <div className="flex items-center justify-between">
          <h2 className="text-xxs font-bold uppercase tracking-[0.18em] text-siem-muted">
            Intelligence Feed
          </h2>
          <div className="text-2xs uppercase tracking-[0.18em] text-siem-muted">
            {regionFilter === "all"
              ? "Global"
              : regionFilter.startsWith("country:")
                ? countries.find((c) => c.code === regionFilter.slice(8))?.name ?? regionFilter.slice(8)
                : regionFilter}
          </div>
        </div>

        {/* Live / History tabs */}
        <div className="grid grid-cols-2 gap-2">
          <button
            type="button"
            onClick={() => setViewMode("live")}
            className={`rounded border px-2 py-1.5 text-2xs font-mono uppercase tracking-wider transition-colors ${
              viewMode === "live"
                ? "bg-siem-accent/18 text-siem-accent border-siem-accent/35"
                : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
            }`}
          >
            Live ({liveAlerts.length})
          </button>
          <button
            type="button"
            onClick={() => setViewMode("history")}
            className={`rounded border px-2 py-1.5 text-2xs font-mono uppercase tracking-wider transition-colors ${
              viewMode === "history"
                ? "bg-siem-accent/18 text-siem-accent border-siem-accent/35"
                : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
            }`}
          >
            History ({historyAlerts.length})
          </button>
        </div>

        {/* Compact stat strip */}
        <div className="grid grid-cols-3 gap-2 text-2xs font-mono uppercase tracking-wide">
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
            <span className="text-siem-muted">Total</span>{" "}
            <span className="text-siem-text">{currentAlerts.length}</span>
          </div>
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
            <span className="text-siem-muted">Crit</span>{" "}
            <span className="text-rose-300">
              {currentAlerts.filter((a) => a.severity === "critical").length}
            </span>
          </div>
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
            <span className="text-siem-muted">High</span>{" "}
            <span className="text-amber-300">
              {currentAlerts.filter((a) => a.severity === "high").length}
            </span>
          </div>
        </div>

        {/* Filters */}
        <div className="grid grid-cols-2 gap-2">
          <div className="relative">
            <select
              value={severityFilter}
              onChange={(e) => setSeverityFilter(e.target.value as Severity | "all")}
              className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-2.5 pr-8 py-1.5 text-xs text-siem-text cursor-pointer hover:bg-siem-accent/10 transition-colors focus:outline-none focus:ring-1 focus:ring-siem-accent"
            >
              <option value="all">All Severity</option>
              <option value="critical">Critical</option>
              <option value="high">High</option>
              <option value="medium">Medium</option>
              <option value="low">Low</option>
            </select>
            <ChevronDown size={12} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-siem-muted pointer-events-none" />
          </div>
          <div className="relative">
            <Globe size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-siem-muted pointer-events-none" />
            <select
              value={regionFilter}
              onChange={(e) => onRegionChange(e.target.value)}
              className="w-full appearance-none bg-white/5 border border-siem-border rounded-md pl-7 pr-8 py-1.5 text-xs text-siem-text cursor-pointer hover:bg-siem-accent/10 transition-colors focus:outline-none focus:ring-1 focus:ring-siem-accent"
            >
              <option value="all">Global ({alerts.length})</option>
              {regions.map(([region, count]) => (
                <option key={region} value={region}>
                  {region} ({count})
                </option>
              ))}
              {countries.length > 0 && (
                <option disabled>── Countries ──</option>
              )}
              {countries.map((c) => (
                <option key={c.code} value={`country:${c.code}`}>
                  {c.name} ({c.count})
                </option>
              ))}
            </select>
            <ChevronDown size={12} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-siem-muted pointer-events-none" />
          </div>
        </div>
      </div>

      {/* Alert list */}
      <div
        className={`min-h-0 flex-1 overflow-y-auto px-3 py-3 space-y-2 ${
          isRefreshingList ? "animate-alert-list-refresh" : ""
        }`}
      >
        {viewMode === "live" ? (
          liveAlerts.length > 0 ? (
            liveAlerts.map((alert, idx) => renderAlertCard(alert, idx))
          ) : (
            <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
              <p className="text-xs text-siem-muted uppercase tracking-wider">
                No alerts in the last 48 hours matching current filters
              </p>
            </div>
          )
        ) : historyGroups.length > 0 ? (
          historyGroups.map((group) => (
            <section key={group.label}>
              <div className="sticky top-0 z-10 px-1 pb-1.5 pt-1 text-2xs font-mono uppercase tracking-wider text-siem-muted bg-siem-panel/95 backdrop-blur-sm">
                {group.label} ({group.alerts.length})
              </div>
              <div className="space-y-2">
                {group.alerts.map((alert, idx) => renderAlertCard(alert, idx))}
              </div>
            </section>
          ))
        ) : (
          <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
            <p className="text-xs text-siem-muted uppercase tracking-wider">
              No historical alerts older than 48 hours
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
