/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Header } from "@/components/Header";
import { GlobeView } from "@/components/GlobeView";
import { AlertFeed } from "@/components/AlertFeed";
import { AlertDetail } from "@/components/AlertDetail";
import { FeedDirectory } from "@/components/FeedDirectory";
import { useAlerts } from "@/hooks/useAlerts";
import { useAlertState } from "@/hooks/useAlertState";
import { useSearch } from "@/hooks/useSearch";
import { useSourceHealth } from "@/hooks/useSourceHealth";
import { alertMatchesRegionFilter } from "@/lib/regions";
import type { AlertCategory } from "@/types/alert";

type SeverityFilter = "critical" | "high" | null;

const SOURCE_SELECTION_COOKIE = "euosint_selected_sources";

function readSelectedSources(): string[] {
  if (typeof document === "undefined") return [];
  const cookie = document.cookie
    .split("; ")
    .find((entry) => entry.startsWith(`${SOURCE_SELECTION_COOKIE}=`));
  if (!cookie) return [];
  try {
    const value = decodeURIComponent(cookie.split("=").slice(1).join("="));
    const parsed = JSON.parse(value);
    return Array.isArray(parsed)
      ? parsed.filter((item): item is string => typeof item === "string" && item.trim().length > 0)
      : [];
  } catch {
    return [];
  }
}

function writeSelectedSources(sourceIds: string[]) {
  if (typeof document === "undefined") return;
  const expires = new Date();
  expires.setMonth(expires.getMonth() + 6);
  document.cookie = `${SOURCE_SELECTION_COOKIE}=${encodeURIComponent(JSON.stringify(sourceIds))}; expires=${expires.toUTCString()}; path=/; SameSite=Lax`;
}

export default function App() {
  const { alerts, isLoading, sourceCount } = useAlerts();
  const { alerts: stateAlerts } = useAlertState();
  const { sourceHealth, isLoading: isSourceHealthLoading } = useSourceHealth();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedSourceIds, setSelectedSourceIds] = useState<string[]>([]);
  const [categoryFilter, setCategoryFilter] = useState<AlertCategory | "all">("all");
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>(null);
  const [regionFilter, setRegionFilter] = useState<string>("Europe");
  const { query: searchQuery, setQuery: setSearchQuery, results: searchResults, isApiAvailable } = useSearch();
  const [visibleNowAlertIds, setVisibleNowAlertIds] = useState<string[]>([]);
  const [visibleHistoryAlertIds, setVisibleHistoryAlertIds] = useState<string[]>([]);
  const [mobilePane, setMobilePane] = useState<"intel" | "map" | "alerts">("map");
  const panelRef = useRef<HTMLDivElement>(null);
  const [utcTime, setUtcTime] = useState(() => new Date().toISOString().slice(0, 19).replace("T", " ") + "Z");

  useEffect(() => {
    const id = setInterval(() => {
      setUtcTime(new Date().toISOString().slice(0, 19).replace("T", " ") + "Z");
    }, 1000);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    setSelectedSourceIds(readSelectedSources());
  }, []);

  useEffect(() => {
    const healthSources = sourceHealth?.sources;
    if (!healthSources || healthSources.length === 0) {
      return;
    }
    const availableSourceIds = new Set(healthSources.map((entry) => entry.source_id));
    setSelectedSourceIds((current) => {
      const next = current.filter((sourceId) => availableSourceIds.has(sourceId));
      if (next.length !== current.length) {
        writeSelectedSources(next);
      }
      return next;
    });
  }, [sourceHealth]);

  useEffect(() => {
    writeSelectedSources(selectedSourceIds);
  }, [selectedSourceIds]);

  const handleRegionChange = useCallback((nextRegion: string) => {
    setRegionFilter(nextRegion);
    setSelectedSourceIds([]);
    setSelectedId(null);
  }, []);

  const regionScopedAlerts = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();

    // When API search returned results, use those (already ranked by BM25).
    if (query && isApiAvailable && searchResults.length > 0) {
      let filtered = searchResults;
      if (regionFilter !== "all") {
        filtered = filtered.filter((alert) => alertMatchesRegionFilter(alert, regionFilter));
      }
      return filtered;
    }

    // Fallback: client-side filter.
    let filtered = alerts;
    if (regionFilter !== "all") {
      filtered = filtered.filter((alert) => alertMatchesRegionFilter(alert, regionFilter));
    }
    if (query) {
      filtered = filtered.filter((alert) => {
        const haystack = [
          alert.title,
          alert.source.authority_name,
          alert.source.country,
          alert.source.country_code,
          alert.source.region,
          alert.category,
          alert.canonical_url,
        ]
          .join(" ")
          .toLowerCase();
        return haystack.includes(query);
      });
    }
    return filtered;
  }, [alerts, regionFilter, searchQuery, searchResults, isApiAvailable]);

  const scopedAlerts = useMemo(() => {
    let filtered = regionScopedAlerts;
    if (selectedSourceIds.length > 0) {
      const selectedSet = new Set(selectedSourceIds);
      filtered = filtered.filter((alert) => selectedSet.has(alert.source_id));
    }
    if (categoryFilter !== "all") {
      filtered = filtered.filter((alert) => alert.category === categoryFilter);
    }
    if (severityFilter) {
      filtered = filtered.filter((alert) => alert.severity === severityFilter);
    }
    return filtered;
  }, [categoryFilter, regionScopedAlerts, selectedSourceIds, severityFilter]);

  const stateRegionScopedAlerts = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    let filtered = stateAlerts;
    if (regionFilter !== "all") {
      filtered = filtered.filter((alert) => alertMatchesRegionFilter(alert, regionFilter));
    }
    if (query) {
      filtered = filtered.filter((alert) => {
        const haystack = [
          alert.title,
          alert.source.authority_name,
          alert.source.country,
          alert.source.country_code,
          alert.source.region,
          alert.category,
          alert.canonical_url,
        ]
          .join(" ")
          .toLowerCase();
        return haystack.includes(query);
      });
    }
    return filtered;
  }, [stateAlerts, regionFilter, searchQuery]);

  const scopedStateAlerts = useMemo(() => {
    let filtered = stateRegionScopedAlerts;
    if (selectedSourceIds.length > 0) {
      const selectedSet = new Set(selectedSourceIds);
      filtered = filtered.filter((alert) => selectedSet.has(alert.source_id));
    }
    if (categoryFilter !== "all") {
      filtered = filtered.filter((alert) => alert.category === categoryFilter);
    }
    if (severityFilter) {
      filtered = filtered.filter((alert) => alert.severity === severityFilter);
    }
    return filtered;
  }, [categoryFilter, stateRegionScopedAlerts, selectedSourceIds, severityFilter]);

  const handleCountrySelect = useCallback((countryCode: string) => {
    const nextRegion = `country:${countryCode}`;
    setRegionFilter((current) => current === nextRegion ? "all" : nextRegion);
    setSelectedSourceIds([]);
    setCategoryFilter("all");
    setSelectedId(null);
  }, []);

  const handleSourceSelectionChange = useCallback((sourceIds: string[]) => {
    setSelectedSourceIds(sourceIds);
    setSelectedId(null);
  }, []);


  const selectedAlert = selectedId
    ? scopedAlerts.find((a) => a.alert_id === selectedId) ??
      scopedStateAlerts.find((a) => a.alert_id === selectedId) ??
      alerts.find((a) => a.alert_id === selectedId) ??
      stateAlerts.find((a) => a.alert_id === selectedId) ??
      null
    : null;

  const handleClose = useCallback(() => {
    const el = panelRef.current;
    if (!el) {
      setSelectedId(null);
      return;
    }
    el.style.animation = "slide-out-right 0.24s ease-in forwards";
    el.addEventListener(
      "animationend",
      () => {
        setSelectedId(null);
      },
      { once: true }
    );
  }, []);

  useEffect(() => {
    const exists = alerts.some((a) => a.alert_id === selectedId) || stateAlerts.some((a) => a.alert_id === selectedId);
    if (selectedId && !exists) {
      setSelectedId(null);
    }
  }, [alerts, stateAlerts, selectedId]);

  useEffect(() => {
    setVisibleNowAlertIds(scopedAlerts.map((a) => a.alert_id));
    setVisibleHistoryAlertIds([]);
  }, [scopedAlerts]);

  return (
    <div className="flex h-[100dvh] flex-col bg-siem-bg text-siem-text">
      <Header
        regionFilter={regionFilter}
        onRegionChange={handleRegionChange}
        sourceCount={sourceCount}
        selectedSourceIds={selectedSourceIds}
        onSelectedSourceIdsChange={handleSourceSelectionChange}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        activeMenu="overview"
        onMenuChange={() => {}}
        alerts={alerts}
      />

      {/* Main content — fills remaining height, no overflow */}
      <div className="relative flex min-h-0 flex-1 gap-3 px-3 pb-3 pt-3 md:px-4">
        {/* Left panel — intel overview */}
        <div className={`${mobilePane === "intel" ? "block" : "hidden"} md:block w-full md:w-[20rem] md:shrink-0 min-h-0`}>
          <FeedDirectory
            view="overview"
            alerts={regionScopedAlerts}
            sourceHealth={sourceHealth}
            isLoading={isSourceHealthLoading}
            selectedSourceIds={selectedSourceIds}
            onSelectSourceIdsChange={handleSourceSelectionChange}
            categoryFilter={categoryFilter}
            onSelectCategory={setCategoryFilter}
            regionFilter={regionFilter}
            onSelectCountry={handleCountrySelect}
            severityFilter={severityFilter}
            onSeverityFilterChange={setSeverityFilter}
            onSearchTerm={setSearchQuery}
          />
        </div>

        {/* Center — map */}
        <div className={`${mobilePane === "map" ? "block" : "hidden"} md:block min-h-0 flex-1 min-w-0`}>
          <GlobeView
            alerts={scopedAlerts}
            historicalAlerts={scopedStateAlerts}
            selectedId={selectedId}
            onSelect={setSelectedId}
            regionFilter={regionFilter}
            onRegionChange={handleRegionChange}
            visibleNowAlertIds={visibleNowAlertIds}
            visibleHistoryAlertIds={visibleHistoryAlertIds}
            onSelectSourceIdsChange={handleSourceSelectionChange}
            selectedSourceIds={selectedSourceIds}
          />
        </div>

        {/* Right panel — alert queue (contained, scrollable) */}
        <div className={`${mobilePane === "alerts" ? "block" : "hidden"} md:block w-full md:w-[24rem] md:shrink-0 min-h-0`}>
          <div className="flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_24px_80px_rgba(0,0,0,0.28)]">
            {isLoading ? (
              <div className="flex flex-1 items-center justify-center text-sm text-siem-muted">
                Loading live alert queue...
              </div>
            ) : (
              <AlertFeed
                alerts={scopedAlerts}
                historicalAlerts={scopedStateAlerts}
                selectedId={selectedId}
                onSelect={setSelectedId}
                categoryFilter={categoryFilter}
                regionFilter={regionFilter}
                onRegionChange={handleRegionChange}
                onVisibleAlertIdsChange={({ nowIds, historyIds }) => {
                  setVisibleNowAlertIds(nowIds);
                  setVisibleHistoryAlertIds(historyIds);
                }}
              />
            )}
          </div>
        </div>

        {/* Mobile tab bar */}
        <div className="absolute bottom-3 left-3 right-3 grid grid-cols-3 gap-2 md:hidden">
          {[
            ["intel", "Intel"],
            ["map", "Map"],
            ["alerts", "Queue"],
          ].map(([pane, label]) => (
            <button
              key={pane}
              type="button"
              onClick={() => setMobilePane(pane as "intel" | "map" | "alerts")}
              className={`rounded-full border px-3 py-2 text-xxs uppercase tracking-[0.18em] ${
                mobilePane === pane
                  ? "border-siem-accent bg-siem-accent/14 text-siem-text"
                  : "border-siem-border bg-siem-panel text-siem-muted"
              }`}
            >
              {label}
            </button>
          ))}
        </div>

        {/* Alert detail overlay */}
        {selectedAlert && (
          <div
            ref={panelRef}
            className="absolute inset-0 z-20 flex animate-slide-in md:left-auto md:right-4 md:top-0 md:h-full md:w-[26rem]"
          >
            <div
              className="hidden md:block w-8 cursor-pointer bg-gradient-to-r from-transparent to-black/25"
              onClick={handleClose}
            />
            <div className="w-full overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel-strong shadow-[0_28px_100px_rgba(0,0,0,0.45)]">
              <AlertDetail alert={selectedAlert} onClose={handleClose} />
            </div>
          </div>
        )}
      </div>

      <div className="flex items-center justify-between border-t border-siem-border bg-siem-panel/85 px-4 py-2 text-2xs uppercase tracking-[0.18em] text-siem-muted">
        <span>
          <a href="https://www.scalytics.io/streamingintelligence?utm_source=euosint&utm_medium=footer&utm_campaign=osint_console" target="_blank" rel="noopener" className="hover:text-siem-accent transition-colors">Scalytics OSINT</a>
          {" // Live 48h + History"}          
        </span>
        <span className="font-mono tabular-nums text-siem-muted">{utcTime}</span>
        <span className="hidden md:inline">
          <a href="https://www.scalytics.io/contact?utm_source=euosint&utm_medium=footer&utm_campaign=osint_console" target="_blank" rel="noopener" className="hover:text-siem-accent transition-colors">Build your own intelligence pipeline — Contact us</a>
        </span>
      </div>
    </div>
  );
}
