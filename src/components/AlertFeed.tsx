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
  categoryOrder,
  categoryBadge,
  freshnessLabel,
} from "@/lib/severity";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { Clock, Building2, ChevronDown, ChevronRight, Globe } from "lucide-react";

interface Props {
  alerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  categoryFilter: AlertCategory | "all";
  onCategoryChange: (category: AlertCategory | "all") => void;
  regionFilter: string;
  onRegionChange: (region: string) => void;
  onNavigatorSelect?: (region: string, category: AlertCategory) => void;
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
  onNavigatorSelect,
  onVisibleAlertIdsChange,
}: Props) {
  const [viewMode, setViewMode] = useState<"navigator" | "timeline">("navigator");
  const [actionableOnly, setActionableOnly] = useState(true);
  const [severityFilter, setSeverityFilter] = useState<Severity | "all">("all");
  const [activeNavigatorGroupKey, setActiveNavigatorGroupKey] = useState<string | null>(null);
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(new Set());
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

  const actionable = actionableOnly
    ? regionFiltered.filter((a) => a.reporting?.url || a.reporting?.phone)
    : regionFiltered;

  const facetFiltered = actionable.filter((a) => {
    const categoryMatch = categoryFilter === "all" || a.category === categoryFilter;
    const severityMatch = severityFilter === "all" || a.severity === severityFilter;
    return categoryMatch && severityMatch;
  });

  const infoAlerts = facetFiltered.filter((a) => a.severity === "info");
  const primaryAlerts = facetFiltered.filter((a) => a.severity !== "info");

  const sorted = [...primaryAlerts].sort((a, b) => {
    const sev = ["critical", "high", "medium", "low", "info"];
    const diff = sev.indexOf(a.severity) - sev.indexOf(b.severity);
    if (diff !== 0) return diff;
    return new Date(b.first_seen).getTime() - new Date(a.first_seen).getTime();
  });

  const grouped = categoryOrder
    .map((category) => ({
      category,
      alerts: sorted.filter((alert) => alert.category === category),
    }))
    .filter((group) => group.alerts.length > 0);

  const navigatorGroups = useMemo(() => {
    const buckets = new Map<
      string,
      {
        key: string;
        region: string;
        category: AlertCategory;
        alerts: Alert[];
        total: number;
        critical: number;
      }
    >();
    facetFiltered.forEach((alert) => {
      const key = `${alert.source.region}::${alert.category}`;
      const existing = buckets.get(key);
      if (existing) {
        existing.alerts.push(alert);
        existing.total += 1;
        if (alert.severity === "critical") existing.critical += 1;
        return;
      }
      buckets.set(key, {
        key,
        region: alert.source.region,
        category: alert.category,
        alerts: [alert],
        total: 1,
        critical: alert.severity === "critical" ? 1 : 0,
      });
    });
    return [...buckets.values()]
      .map((group) => ({
        ...group,
        alerts: [...group.alerts].sort(
          (a, b) => new Date(b.first_seen).getTime() - new Date(a.first_seen).getTime()
        ),
      }))
      .sort((a, b) => {
        const criticalDelta = b.critical - a.critical;
        if (criticalDelta !== 0) return criticalDelta;
        return b.total - a.total;
      });
  }, [facetFiltered]);

  // Reset navigator selection when region or category filter changes.
  useEffect(() => {
    setActiveNavigatorGroupKey(null);
  }, [regionFilter, categoryFilter]);

  useEffect(() => {
    if (navigatorGroups.length === 0) {
      setActiveNavigatorGroupKey(null);
      return;
    }
    if (
      activeNavigatorGroupKey &&
      navigatorGroups.some((group) => group.key === activeNavigatorGroupKey)
    ) {
      return;
    }
    setActiveNavigatorGroupKey(navigatorGroups[0].key);
  }, [activeNavigatorGroupKey, navigatorGroups]);

  const activeNavigatorGroup =
    navigatorGroups.find((group) => group.key === activeNavigatorGroupKey) ?? null;

  const handleNavigatorGroupSelect = (groupKey: string) => {
    setActiveNavigatorGroupKey(groupKey);
    const group = navigatorGroups.find((entry) => entry.key === groupKey);
    if (!group) {
      return;
    }
    onNavigatorSelect?.(group.region, group.category);
  };

  // Keep globe visibility aligned with current filters, not only the active navigator bucket.
  const visibleAlertIds = useMemo(
    () => facetFiltered.map((a) => a.alert_id),
    [facetFiltered]
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

  const renderAlertCard = (alert: Alert, queueLabel: string, position: number) => {
    const isSelected = selectedId === alert.alert_id;
    const isNew = newAlertIds.has(alert.alert_id);

    return (
      <button
        key={alert.alert_id}
        onClick={() => onSelect(alert.alert_id)}
        className={`relative w-full text-left rounded-lg border border-siem-border px-3 py-2.5 pl-4 bg-siem-bg/45 transition-colors hover:bg-siem-accent/8 ${
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
              className={`inline-flex items-center px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider rounded border ${
                severityBg[alert.severity]
              } ${alert.severity === "critical" ? "animate-critical-badge" : ""}`}
            >
              {severityLabel[alert.severity]}
            </span>
            {isNew && (
              <span className="inline-flex items-center px-1.5 py-0.5 text-[10px] uppercase tracking-wider rounded border border-siem-accent/35 text-siem-accent bg-siem-accent/12">
                New
              </span>
            )}
            {typeof alert.triage?.relevance_score === "number" && (
              <span className="inline-flex items-center px-1.5 py-0.5 text-[10px] uppercase tracking-wider rounded border border-siem-border text-siem-muted bg-white/5">
                Rel {Math.round(alert.triage.relevance_score * 100)}
              </span>
            )}
          </div>
          <span className="text-[10px] text-siem-muted font-mono uppercase tracking-wider">
            {queueLabel} #{position + 1}
          </span>
        </div>
        <p className="text-sm text-siem-text leading-snug line-clamp-2 mb-2">{alert.title}</p>
        <div className="flex items-center justify-between gap-2 text-[10px] text-siem-muted font-mono uppercase tracking-wide">
          <span className="flex items-center gap-1 min-w-0">
            <Building2 size={10} />
            <span className="truncate">{alert.source.authority_name}</span>
          </span>
          <span className="flex items-center gap-1 shrink-0">
            <Clock size={10} />
            {freshnessLabel(alert.freshness_hours)}
          </span>
        </div>
        <div className="mt-1.5 flex items-center gap-1.5 text-[10px]">
          <span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 bg-siem-accent/10 text-siem-accent border border-siem-accent/20">
            <Globe size={9} />
            {alert.source.region}
          </span>
          <span className="inline-flex items-center rounded px-1.5 py-0.5 bg-white/5 text-siem-muted border border-siem-border">
            {alert.status}
          </span>
        </div>
      </button>
    );
  };

  const toggleSection = (key: string) => {
    setCollapsedSections((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="px-3 py-3 border-b border-siem-border bg-siem-panel/95 space-y-2.5">
        <div className="flex items-center justify-between">
          <h2 className="text-[11px] font-bold uppercase tracking-[0.18em] text-siem-muted">
            Intelligence Queue
          </h2>
          <div className="text-[10px] uppercase tracking-[0.18em] text-siem-muted">
            {regionFilter === "all"
              ? "Global scope"
              : regionFilter.startsWith("country:")
                ? `${countries.find((c) => c.code === regionFilter.slice(8))?.name ?? regionFilter.slice(8)} scope`
                : `${regionFilter} scope`}
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2 text-[10px] font-mono uppercase tracking-wide">
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
            <span className="text-siem-muted">Active</span>{" "}
            <span className="text-siem-text">{alerts.filter((a) => a.status === "active").length}</span>
          </div>
          <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
            <span className="text-siem-muted">Actionable</span>{" "}
            <span className="text-siem-text">
              {alerts.filter((a) => a.reporting?.url || a.reporting?.phone).length}
            </span>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-2">
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
          <div className="relative">
            <select
              value={categoryFilter}
              onChange={(e) => onCategoryChange(e.target.value as AlertCategory | "all")}
              className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-2.5 pr-8 py-1.5 text-xs text-siem-text cursor-pointer hover:bg-siem-accent/10 transition-colors focus:outline-none focus:ring-1 focus:ring-siem-accent"
            >
              <option value="all">All Categories</option>
              {categoryOrder.map((category) => (
                <option key={category} value={category}>
                  {categoryLabels[category]}
                </option>
              ))}
            </select>
            <ChevronDown size={12} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-siem-muted pointer-events-none" />
          </div>
          <div className="flex items-center gap-2">
            <div className="relative flex-1">
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
                <option value="info">Informational</option>
              </select>
              <ChevronDown size={12} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-siem-muted pointer-events-none" />
            </div>
            <button
              type="button"
              onClick={() => setActionableOnly((prev) => !prev)}
              className={`shrink-0 rounded-md border px-2 py-1.5 text-[10px] font-bold uppercase tracking-wider transition-colors ${
                actionableOnly
                  ? "bg-siem-accent/18 text-siem-accent border-siem-accent/35"
                  : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
              }`}
            >
              Reporting
            </button>
          </div>
        </div>
      </div>
      <div className="border-b border-siem-border bg-siem-panel/95 px-3 py-3 space-y-3">
        <div className="grid grid-cols-2 gap-2">
          <button
            type="button"
            onClick={() => setViewMode("navigator")}
            className={`rounded border px-2 py-1.5 text-[10px] font-mono uppercase tracking-wider transition-colors ${
              viewMode === "navigator"
                ? "bg-siem-accent/18 text-siem-accent border-siem-accent/35"
                : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
            }`}
          >
            Navigator
          </button>
          <button
            type="button"
            onClick={() => setViewMode("timeline")}
            className={`rounded border px-2 py-1.5 text-[10px] font-mono uppercase tracking-wider transition-colors ${
              viewMode === "timeline"
                ? "bg-siem-accent/18 text-siem-accent border-siem-accent/35"
                : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
            }`}
          >
            Queue
          </button>
        </div>
        {viewMode === "navigator" && (
          <>
            <div className="grid grid-cols-3 gap-2 text-[10px] font-mono uppercase tracking-wide">
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">Cases</span>{" "}
                <span className="text-siem-text">{facetFiltered.length}</span>
              </div>
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">Critical</span>{" "}
                <span className="text-siem-text">
                  {facetFiltered.filter((a) => a.severity === "critical").length}
                </span>
              </div>
              <div className="rounded border border-siem-border bg-white/5 px-2 py-1">
                <span className="text-siem-muted">Countries</span>{" "}
                <span className="text-siem-text">
                  {new Set(facetFiltered.map((a) => a.source.country_code)).size}
                </span>
              </div>
            </div>
            {navigatorGroups.length > 0 && (
              <section className="rounded-lg border border-siem-border bg-siem-panel/35 overflow-hidden">
                <div className="px-3 py-2 border-b border-siem-border bg-siem-panel/70 text-[10px] font-mono uppercase tracking-wider text-siem-muted">
                  Region + category navigation
                </div>
                <div className="max-h-44 overflow-y-auto p-2 space-y-1.5">
                  {navigatorGroups.map((group) => (
                    <button
                      key={group.key}
                      type="button"
                      onClick={() => handleNavigatorGroupSelect(group.key)}
                      className={`w-full text-left rounded border px-2 py-1.5 transition-colors ${
                        activeNavigatorGroupKey === group.key
                          ? "bg-siem-accent/12 border-siem-accent/35"
                          : "bg-white/5 border-siem-border hover:bg-siem-accent/8"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-[10px] text-siem-text uppercase tracking-wide truncate">
                          {group.region}
                        </span>
                        <span className="text-[10px] text-siem-muted font-mono">{group.total}</span>
                      </div>
                      <div className="mt-1 flex items-center justify-between gap-2">
                        <span
                          className={`inline-flex items-center px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider rounded border ${categoryBadge[group.category]}`}
                        >
                          {categoryLabels[group.category]}
                        </span>
                        {group.critical > 0 && (
                          <span className="text-[10px] text-red-300 font-mono">
                            {group.critical} critical
                          </span>
                        )}
                      </div>
                    </button>
                  ))}
                </div>
              </section>
            )}
          </>
        )}
      </div>
      <div
        className={`min-h-0 flex-1 px-3 py-3 ${
          isRefreshingList ? "animate-alert-list-refresh" : ""
        }`}
      >
        {viewMode === "navigator" ? (
          <div className="flex h-full min-h-0 flex-col">
            {activeNavigatorGroup && (
              <section className="flex min-h-0 flex-1 flex-col rounded-lg border border-siem-border bg-siem-panel/35 overflow-hidden">
                <div className="px-3 py-2 border-b border-siem-border bg-siem-panel/70 flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="text-[10px] uppercase tracking-wider text-siem-muted">Case Queue</p>
                    <p className="text-xs text-siem-text truncate">
                      {activeNavigatorGroup.region} •{" "}
                      {categoryLabels[activeNavigatorGroup.category]}
                    </p>
                  </div>
                  <span className="text-[10px] text-siem-muted font-mono">
                    {activeNavigatorGroup.total}
                  </span>
                </div>
                <div className="min-h-0 flex-1 overflow-y-auto p-2 space-y-2">
                  {activeNavigatorGroup.alerts.map((alert, idx) =>
                    renderAlertCard(alert, "Case", idx)
                  )}
                </div>
              </section>
            )}
            {!activeNavigatorGroup && (
              <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
                <p className="text-xs text-siem-muted uppercase tracking-wider">
                  Select a region/category bucket to inspect its case queue
                </p>
              </div>
            )}
          </div>
        ) : (
          <div className="h-full overflow-y-auto space-y-3">
            {grouped.map((group) => (
              <section
                key={group.category}
                className="rounded-lg border border-siem-border bg-siem-panel/35 overflow-hidden"
              >
                <button
                  type="button"
                  onClick={() => toggleSection(group.category)}
                  className="w-full flex items-center justify-between px-3 py-2 border-b border-siem-border bg-siem-panel/70 hover:bg-siem-accent/10 transition-colors"
                >
                  <div className="flex items-center gap-2">
                    {collapsedSections.has(group.category) ? (
                      <ChevronRight size={12} className="text-siem-muted" />
                    ) : (
                      <ChevronDown size={12} className="text-siem-muted" />
                    )}
                    <span
                      className={`inline-flex items-center px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider rounded border ${categoryBadge[group.category]}`}
                    >
                      {categoryLabels[group.category]}
                    </span>
                  </div>
                  <span className="text-[10px] text-siem-muted font-mono uppercase tracking-wide">
                    {group.alerts.length}
                  </span>
                </button>
                {!collapsedSections.has(group.category) && (
                  <div className="p-2 space-y-2">
                    {group.alerts.map((alert, idx) => renderAlertCard(alert, "Stack", idx))}
                  </div>
                )}
              </section>
            ))}
            {infoAlerts.length > 0 && (
              <section className="rounded-lg border border-siem-border bg-siem-panel/35 overflow-hidden">
                <button
                  type="button"
                  onClick={() => toggleSection("informational")}
                  className="w-full flex items-center justify-between px-3 py-2 border-b border-siem-border bg-siem-panel/70 hover:bg-siem-accent/10 transition-colors"
                >
                  <div className="flex items-center gap-2">
                    {collapsedSections.has("informational") ? (
                      <ChevronRight size={12} className="text-siem-muted" />
                    ) : (
                      <ChevronDown size={12} className="text-siem-muted" />
                    )}
                    <span className="inline-flex items-center px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider rounded border bg-cyan-500/15 text-cyan-300 border-cyan-500/30">
                      Informational / Traffic
                    </span>
                  </div>
                  <span className="text-[10px] text-siem-muted font-mono uppercase tracking-wide">
                    {infoAlerts.length}
                  </span>
                </button>
                {!collapsedSections.has("informational") && (
                  <div className="p-2 space-y-2">
                    {infoAlerts.map((alert, idx) => renderAlertCard(alert, "Stack", idx))}
                  </div>
                )}
              </section>
            )}
          </div>
        )}
        {facetFiltered.length === 0 && (
          <div className="rounded-lg border border-siem-border bg-siem-panel/35 p-4 text-center">
            <button
              type="button"
              onClick={() => {
                setActionableOnly(false);
                onCategoryChange("all");
                setSeverityFilter("all");
                onRegionChange("all");
              }}
              className="rounded border border-siem-border bg-white/5 px-2 py-1 text-[10px] text-siem-muted hover:bg-siem-accent/10 hover:text-siem-accent mb-2"
            >
              Reset filters
            </button>
            <p className="text-xs text-siem-muted uppercase tracking-wider">No cases match current filters</p>
          </div>
        )}
      </div>
    </div>
  );
}
