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
import { buildConflictBrief, mergeZoneBriefing } from "@/lib/conflict-briefs";
import { getConflictLensById } from "@/lib/conflict-lenses";
import { useZoneBriefings } from "@/hooks/useZoneBriefings";
import { Clock, Building2, ChevronDown, Globe } from "lucide-react";

const LIVE_WINDOW_MS = 48 * 60 * 60 * 1000; // 48 hours
const PREFERRED_REGION_ORDER = ["Europe", "Africa", "North America", "Asia"] as const;

function isUCDPAlert(alert: Alert): boolean {
  const sourceID = (alert.source_id || "").trim().toLowerCase();
  if (sourceID === "ucdp-ged") return true;
  return (alert.source.authority_name || "").toLowerCase().includes("ucdp");
}

interface Props {
  alerts: Alert[];
  historicalAlerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  categoryFilter: AlertCategory | "all";
  regionFilter: string;
  conflictLensId: string | null;
  onRegionChange: (region: string) => void;
  onVisibleAlertIdsChange: (payload: {
    nowIds: string[];
    historyIds: string[];
    mode: "now" | "history" | "now_history" | "briefing";
  }) => void;
}

export function AlertFeed({
  alerts,
  historicalAlerts,
  selectedId,
  onSelect,
  categoryFilter,
  regionFilter,
  conflictLensId,
  onRegionChange,
  onVisibleAlertIdsChange,
}: Props) {
  const [viewMode, setViewMode] = useState<"now" | "history" | "now_history" | "briefing">("now");
  const [severityFilter, setSeverityFilter] = useState<Severity | "all">("all");
  const [isRefreshingList, setIsRefreshingList] = useState(false);
  const [newAlertIds, setNewAlertIds] = useState<Set<string>>(new Set());
  const knownAlertIdsRef = useRef<Set<string>>(new Set());
  const lastVisibleSigRef = useRef("");
  const refreshTimeoutRef = useRef<number | null>(null);
  const glowTimeoutsRef = useRef<number[]>([]);
  const activeConflictLens = useMemo(() => getConflictLensById(conflictLensId), [conflictLensId]);
  const { briefings: zoneBriefings } = useZoneBriefings();
  const activeConflictBrief = useMemo(() => {
    const derived = buildConflictBrief([...alerts, ...historicalAlerts], activeConflictLens);
    const override = activeConflictLens
      ? zoneBriefings.find((briefing) => briefing.lensId === activeConflictLens.id)
      : null;
    return mergeZoneBriefing(derived, override);
  }, [activeConflictLens, alerts, historicalAlerts, zoneBriefings]);
  const isConflictContextMode = activeConflictLens !== null;

  const regions = useMemo(() => {
    const set = new Map<string, number>();
    alerts.forEach((a) => {
      const r = a.source.region;
      set.set(r, (set.get(r) ?? 0) + 1);
    });
    const preferred = PREFERRED_REGION_ORDER.map((region) => [region, set.get(region) ?? 0] as [string, number]);
    return preferred;
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

  const historicalRegionFiltered =
    regionFilter === "all"
      ? historicalAlerts
      : historicalAlerts.filter((a) => alertMatchesRegionFilter(a, regionFilter));

  const facetFiltered = regionFiltered.filter((a) => {
    const categoryMatch = categoryFilter === "all" || a.category === categoryFilter;
    const severityMatch = severityFilter === "all" || a.severity === severityFilter;
    return categoryMatch && severityMatch;
  });

  const historicalFacetFiltered = historicalRegionFiltered.filter((a) => {
    const categoryMatch = categoryFilter === "all" || a.category === categoryFilter;
    const severityMatch = severityFilter === "all" || a.severity === severityFilter;
    return categoryMatch && severityMatch;
  });

  // Separate briefing lane from actionable lanes.
  const actionableAlerts = useMemo(
    () => facetFiltered.filter((a) => (a.signal_lane ?? (a.severity === "info" ? "info" : "intel")) !== "info"),
    [facetFiltered],
  );

  const briefingAlerts = useMemo(
    () =>
      facetFiltered
        .filter((a) => (a.signal_lane ?? (a.severity === "info" ? "info" : "intel")) === "info")
        .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
    [facetFiltered],
  );

  // Split actionable alerts into live (last 48h) and history (older).
  const now = Date.now();
  const liveCutoff = now - LIVE_WINDOW_MS;

  const nowAlerts = useMemo(
    () =>
      actionableAlerts
        .filter((a) => {
          const t = new Date(a.last_seen).getTime();
          return t >= liveCutoff;
        })
        .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
    [actionableAlerts, liveCutoff],
  );

  const historyAlerts = useMemo(
    () =>
      historicalFacetFiltered
        .filter((a) => a.status !== "filtered")
        .filter((a) => (a.signal_lane ?? (a.severity === "info" ? "info" : "intel")) !== "info")
        .filter((a) => {
          const t = new Date(a.last_seen).getTime();
          return t < liveCutoff;
        })
        .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
    [historicalFacetFiltered, liveCutoff],
  );

  const nowAndHistoryAlerts = useMemo(() => {
    const byId = new Map<string, Alert>();
    for (const alert of nowAlerts) {
      byId.set(alert.alert_id, alert);
    }
    for (const alert of historyAlerts) {
      if (!byId.has(alert.alert_id)) {
        byId.set(alert.alert_id, alert);
      }
    }
    return [...byId.values()].sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime());
  }, [nowAlerts, historyAlerts]);

  const contextAlerts = useMemo(() => {
    const combined = [...alerts, ...historicalAlerts]
      .filter((alert) => isUCDPAlert(alert))
      .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime());
    const deduped = new Map<string, Alert>();
    for (const alert of combined) {
      if (!deduped.has(alert.alert_id)) {
        deduped.set(alert.alert_id, alert);
      }
    }
    return [...deduped.values()];
  }, [alerts, historicalAlerts]);
  const lensContextAlerts = useMemo(() => {
    if (!activeConflictLens) return [] as Alert[];
    return contextAlerts.filter((alert) => {
      if (alert.lat !== 0 || alert.lng !== 0) {
        return (
          alert.lat >= activeConflictLens.bounds.south &&
          alert.lat <= activeConflictLens.bounds.north &&
          alert.lng >= activeConflictLens.bounds.west &&
          alert.lng <= activeConflictLens.bounds.east
        );
      }
      const code = (alert.event_country_code || alert.source.country_code || "").toUpperCase();
      return activeConflictLens.primaryCountryCodes.includes(code);
    });
  }, [activeConflictLens, contextAlerts]);

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

  const visibleNowIds = useMemo(
    () => nowAlerts.map((a) => a.alert_id),
    [nowAlerts],
  );
  const visibleHistoryIds = useMemo(
    () => historyAlerts.map((a) => a.alert_id),
    [historyAlerts],
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
    const sig = `${viewMode}|N:${visibleNowIds.join("|")}|H:${visibleHistoryIds.join("|")}`;
    if (sig === lastVisibleSigRef.current) return;
    lastVisibleSigRef.current = sig;
    if (viewMode === "now") {
      onVisibleAlertIdsChange({ nowIds: visibleNowIds, historyIds: [], mode: "now" });
      return;
    }
    if (viewMode === "history") {
      onVisibleAlertIdsChange({ nowIds: [], historyIds: visibleHistoryIds, mode: "history" });
      return;
    }
    if (viewMode === "now_history") {
      onVisibleAlertIdsChange({ nowIds: visibleNowIds, historyIds: visibleHistoryIds, mode: "now_history" });
      return;
    }
    onVisibleAlertIdsChange({ nowIds: visibleNowIds, historyIds: visibleHistoryIds, mode: "briefing" });
  }, [viewMode, visibleNowIds, visibleHistoryIds, onVisibleAlertIdsChange]);

  useEffect(() => {
    if (!activeConflictLens) return;
    if (viewMode !== "briefing") {
      setViewMode("briefing");
    }
  }, [activeConflictLens, viewMode]);

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
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-2xs">
          <span className="inline-flex max-w-[11rem] items-center gap-1 rounded px-1.5 py-0.5 border border-siem-border bg-white/5 text-siem-text">
            <Globe size={9} className="text-siem-muted" />
            <span className="truncate">{alert.event_country || alert.source.country}</span>
          </span>
          {(alert.event_country || alert.source.country) !== alert.source.country && (
            <span className="inline-flex items-center px-1.5 py-0.5 rounded border border-siem-border text-3xs text-siem-muted bg-white/5">
              Source {alert.source.country}
            </span>
          )}
          {(alert.event_geo_confidence ?? 0) < 0.6 && (
            <span
              className="inline-flex items-center rounded border border-amber-500/30 bg-amber-500/10 px-1 py-0.5"
              title="Low geo confidence"
              aria-label="Low geo confidence"
            >
              <Globe size={9} className="text-amber-300" />
            </span>
          )}
          <span className={`inline-flex items-center px-1.5 py-0.5 rounded border text-2xs font-medium ${categoryBadge[alert.category]}`}>
            {categoryLabels[alert.category]}
          </span>
        </div>
      </button>
    );
  };

  const currentAlerts =
    isConflictContextMode
      ? lensContextAlerts
      : viewMode === "now"
      ? nowAlerts
      : viewMode === "history"
        ? historyAlerts
        : viewMode === "now_history"
          ? nowAndHistoryAlerts
          : briefingAlerts;

  const recentWindowStart = now - 24 * 60 * 60 * 1000;
  const baselineWindowStart = now - 7 * 24 * 60 * 60 * 1000;
  const recentCount = useMemo(
    () => nowAndHistoryAlerts.filter((a) => new Date(a.last_seen).getTime() >= recentWindowStart).length,
    [nowAndHistoryAlerts, recentWindowStart],
  );
  const baselineCount = useMemo(
    () =>
      nowAndHistoryAlerts.filter((a) => {
        const t = new Date(a.last_seen).getTime();
        return t >= baselineWindowStart && t < recentWindowStart;
      }).length,
    [nowAndHistoryAlerts, baselineWindowStart, recentWindowStart],
  );
  const baselineDaily = Math.max(1, Math.round(baselineCount / 6));
  const deltaRatio = recentCount / baselineDaily;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="px-3 py-3 border-b border-siem-border bg-siem-panel/95 space-y-2.5">
        <div className="flex items-center justify-between">
          <h2 className="text-xxs font-bold uppercase tracking-[0.18em] text-siem-muted">
            {isConflictContextMode ? "Context" : "Intelligence Feed"}
          </h2>
          <div className="text-2xs uppercase tracking-[0.18em] text-siem-muted">
            {regionFilter === "all"
              ? "Global"
              : regionFilter.startsWith("country:")
                ? countries.find((c) => c.code === regionFilter.slice(8))?.name ?? regionFilter.slice(8)
                : regionFilter}
          </div>
        </div>

        {isConflictContextMode ? (
          <div className="rounded border border-sky-500/25 bg-sky-500/10 px-2 py-2 text-2xs">
            <div className="font-mono uppercase tracking-[0.16em] text-sky-200">
              Context for {activeConflictLens.label}
            </div>
            <div className="mt-1 text-siem-muted">{activeConflictLens.description}</div>
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              onClick={() => setViewMode("now")}
              className={`rounded border px-2 py-1.5 text-2xs font-mono uppercase tracking-wider transition-colors ${
                viewMode === "now"
                  ? "bg-siem-accent/18 text-siem-accent border-siem-accent/35"
                  : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
              }`}
            >
              Now ({nowAlerts.length})
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
            <button
              type="button"
              onClick={() => setViewMode((current) => (current === "now_history" ? "now" : "now_history"))}
              className={`rounded border px-2 py-1.5 text-2xs font-mono uppercase tracking-wider transition-colors ${
                viewMode === "now_history"
                  ? "bg-cyan-500/18 text-cyan-300 border-cyan-500/35"
                  : "bg-white/5 text-siem-muted border-siem-border hover:bg-cyan-500/10 hover:text-cyan-300"
              }`}
            >
              Now + History ({nowAndHistoryAlerts.length})
            </button>
            <button
              type="button"
              onClick={() => setViewMode("briefing")}
              className={`rounded border px-2 py-1.5 text-2xs font-mono uppercase tracking-wider transition-colors ${
                viewMode === "briefing"
                  ? "bg-sky-500/18 text-sky-300 border-sky-500/35"
                  : "bg-white/5 text-siem-muted border-siem-border hover:bg-sky-500/10 hover:text-sky-300"
              }`}
            >
              Context ({briefingAlerts.length})
            </button>
          </div>
        )}

        {isConflictContextMode && activeConflictBrief && (
          <div className="rounded-lg border border-siem-border bg-siem-panel-strong px-3 py-3">
            <div className="flex items-start justify-between gap-3">
              <div>
                <div className="text-2xs uppercase tracking-[0.16em] text-siem-muted">Zone brief</div>
                <div className="mt-1 text-sm text-siem-text">{activeConflictBrief.lens.label}</div>
                <div className="mt-1 text-[10px] uppercase tracking-[0.14em] text-siem-muted">
                  {activeConflictBrief.sourceLabel}
                </div>
                {activeConflictBrief.sourceURL && (
                  <a
                    href={activeConflictBrief.sourceURL}
                    target="_blank"
                    rel="noreferrer"
                    className="mt-1 inline-flex text-[10px] uppercase tracking-[0.14em] text-siem-accent hover:text-siem-text"
                  >
                    More about this zone
                  </a>
                )}
              </div>
              <div className="text-2xs uppercase tracking-[0.14em] text-siem-muted">
                {activeConflictBrief.asOf ? `As of ${new Date(activeConflictBrief.asOf).toISOString().slice(0, 10)}` : "No dated context"}
              </div>
            </div>

            <div className="mt-3 grid grid-cols-3 gap-2">
              {activeConflictBrief.metrics.map((metric) => (
                <div key={metric.label} className="rounded border border-siem-border bg-white/5 px-2 py-1.5">
                  <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">{metric.label}</div>
                  <div className="mt-1 text-xs font-mono text-siem-text">{metric.value}</div>
                </div>
              ))}
            </div>

            <div className="mt-3 space-y-2">
              <div>
                <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Dominant categories</div>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {activeConflictBrief.topCategories.map((item) => (
                    <span key={item.key} className="inline-flex items-center gap-1 rounded-full border border-siem-border bg-white/5 px-2 py-1 text-2xs text-siem-text">
                      <span>{item.label}</span>
                      <span className="text-siem-muted">{item.count}</span>
                    </span>
                  ))}
                </div>
              </div>

              <div>
                <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Hot countries</div>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {activeConflictBrief.topCountries.map((item) => (
                    <span key={item.code} className="inline-flex items-center gap-1 rounded-full border border-siem-border bg-white/5 px-2 py-1 text-2xs text-siem-text">
                      <span>{item.label}</span>
                      <span className="text-siem-muted">{item.count}</span>
                    </span>
                  ))}
                </div>
              </div>

              <div>
                <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Actors / entities</div>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {activeConflictBrief.actors.map((item) => (
                    <span key={item} className="inline-flex items-center gap-1 rounded-full border border-siem-border bg-white/5 px-2 py-1 text-2xs text-siem-text">
                      {item}
                    </span>
                  ))}
                </div>
              </div>

              <div>
                <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Violence / focus</div>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {activeConflictBrief.violenceTypes.map((item) => (
                    <span key={item} className="inline-flex items-center gap-1 rounded-full border border-siem-border bg-white/5 px-2 py-1 text-2xs text-siem-text">
                      {item}
                    </span>
                  ))}
                </div>
              </div>

              <div>
                <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Top sources</div>
                <div className="mt-1 space-y-1">
                  {activeConflictBrief.topSources.map((item) => (
                    <div key={item.id} className="flex items-center justify-between rounded border border-siem-border bg-white/5 px-2 py-1.5 text-2xs">
                      <span className="truncate text-siem-text">{item.label}</span>
                      <span className="shrink-0 font-mono text-siem-muted">{item.count}</span>
                    </div>
                  ))}
                </div>
              </div>

              {activeConflictBrief.recentEvents.length > 0 && (
                <div>
                  <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Latest 5 UCDP events</div>
                  <div className="mt-1 space-y-1">
                    {activeConflictBrief.recentEvents.map((event, idx) => (
                      <div key={`${event.title}-${event.published ?? idx}`} className="rounded border border-siem-border bg-white/5 px-2 py-1.5 text-2xs">
                        <div className="text-siem-text line-clamp-2">{event.title}</div>
                        <div className="mt-1 text-siem-muted">
                          {event.published ? new Date(event.published).toISOString().slice(0, 10) : "n/a"}
                          {event.country ? ` · ${event.country}` : ""}
                          {typeof event.fatalities === "number" ? ` · fat ${event.fatalities}` : ""}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {activeConflictBrief.summaryBullets && activeConflictBrief.summaryBullets.length > 0 && (
                <div className="rounded border border-siem-border bg-white/5 px-2 py-2 text-2xs text-siem-muted">
                  {activeConflictBrief.summaryBullets.slice(0, 3).map((item) => (
                    <div key={item}>- {item}</div>
                  ))}
                </div>
              )}

              {activeConflictBrief.coverageNote && (
                <div className="rounded border border-siem-border bg-white/5 px-2 py-2 text-2xs text-siem-muted">
                  {activeConflictBrief.coverageNote}
                </div>
              )}

              {activeConflictBrief.latestAlert && (
                <button
                  type="button"
                  onClick={() => onSelect(activeConflictBrief.latestAlert!.alert_id)}
                  className="w-full rounded-lg border border-siem-accent/35 bg-siem-accent/10 px-3 py-2 text-left transition-colors hover:bg-siem-accent/14"
                >
                  <div className="text-[10px] uppercase tracking-[0.14em] text-siem-muted">Latest alert in lens</div>
                  <div className="mt-1 text-xs text-siem-text line-clamp-2">{activeConflictBrief.latestAlert.title}</div>
                  <div className="mt-1 text-2xs text-siem-muted">
                    {activeConflictBrief.latestAlert.source.authority_name} · {freshnessLabel(activeConflictBrief.latestAlert.freshness_hours)}
                  </div>
                </button>
              )}
            </div>
          </div>
        )}

        {!isConflictContextMode && viewMode === "now_history" && (
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1.5 text-2xs font-mono uppercase tracking-wide">
            <span className="text-siem-muted">Delta 24h vs baseline:</span>{" "}
            <span
              className={
                deltaRatio >= 1.5
                  ? "text-rose-300"
                  : deltaRatio <= 0.7
                    ? "text-emerald-300"
                    : "text-amber-300"
              }
            >
              {deltaRatio.toFixed(2)}x ({recentCount}/{baselineDaily})
            </span>
          </div>
        )}

        {/* Compact stat strip */}
        <div className="grid grid-cols-3 gap-2 text-2xs font-mono uppercase tracking-wide">
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
            <span className="text-siem-muted">Total</span>{" "}
            <span className="text-siem-text">{currentAlerts.length}</span>
          </div>
          {isConflictContextMode ? (
            <>
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">7d</span>{" "}
                <span className="text-sky-300">{activeConflictBrief?.recent7d ?? 0}</span>
              </div>
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">Trend</span>{" "}
                <span className="text-sky-300">{activeConflictBrief?.trendLabel ?? "flat"}</span>
              </div>
            </>
          ) : viewMode === "briefing" ? (
            <>
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">Sources</span>{" "}
                <span className="text-sky-300">
                  {new Set(currentAlerts.map((a) => a.source_id)).size}
                </span>
              </div>
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">Mapped</span>{" "}
                <span className="text-sky-300">
                  {currentAlerts.filter((a) => a.lat !== 0 || a.lng !== 0).length}
                </span>
              </div>
            </>
          ) : (
            <>
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
            </>
          )}
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
              {regions.map(([region, count]) => (
                <option key={region} value={region}>
                  {region} ({count})
                </option>
              ))}
              <option value="all">Global ({alerts.length})</option>
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
        {isConflictContextMode ? (
          currentAlerts.length > 0 ? (
            currentAlerts.map((alert, idx) => renderAlertCard(alert, idx))
          ) : (
            <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
              <p className="text-xs text-siem-muted uppercase tracking-wider">
                No context items in this conflict lens
              </p>
            </div>
          )
        ) : viewMode === "now" ? (
          nowAlerts.length > 0 ? (
            nowAlerts.map((alert, idx) => renderAlertCard(alert, idx))
          ) : (
            <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
              <p className="text-xs text-siem-muted uppercase tracking-wider">
                No alerts in the last 48 hours matching current filters
              </p>
            </div>
          )
        ) : viewMode === "history" ? (
          historyGroups.length > 0 ? (
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
          )
        ) : viewMode === "now_history" ? (
          nowAndHistoryAlerts.length > 0 ? (
            nowAndHistoryAlerts.map((alert, idx) => renderAlertCard(alert, idx))
          ) : (
            <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
              <p className="text-xs text-siem-muted uppercase tracking-wider">
                No now/history alerts matching current filters
              </p>
            </div>
          )
        ) : briefingAlerts.length > 0 ? (
          briefingAlerts.map((alert, idx) => renderAlertCard(alert, idx))
        ) : (
          <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
            <p className="text-xs text-siem-muted uppercase tracking-wider">
              No context items matching current filters
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
