/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Header } from "@/components/Header";
import { useAlerts } from "@/hooks/useAlerts";
import { useAlertState } from "@/hooks/useAlertState";
import { useSearch } from "@/hooks/useSearch";
import { useSourceHealth } from "@/hooks/useSourceHealth";
import { useCurrentConflicts } from "@/hooks/useCurrentConflicts";
import type { ConflictCountryFocus } from "@/types/current-conflicts";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { alertMatchesConflictLens, getConflictLensById } from "@/lib/conflict-lenses";
import type { AlertCategory } from "@/types/alert";

type SeverityFilter = "critical" | "high" | null;

const SOURCE_SELECTION_COOKIE = "euosint_selected_sources";
const GlobeView = lazy(() => import("@/components/GlobeView").then((mod) => ({ default: mod.GlobeView })));
const AlertFeed = lazy(() => import("@/components/AlertFeed").then((mod) => ({ default: mod.AlertFeed })));
const AlertDetail = lazy(() => import("@/components/AlertDetail").then((mod) => ({ default: mod.AlertDetail })));
const FeedDirectory = lazy(() => import("@/components/FeedDirectory").then((mod) => ({ default: mod.FeedDirectory })));

function normalizeConflictRegion(raw: string): string {
  const value = raw.trim().toLowerCase();
  if (!value) return "";
  // UCDP region codes in conflict tables:
  // 1=Europe, 2=Middle East, 3=Asia, 4=Africa, 5=Americas
  if (value === "1") return "Europe";
  if (value === "2") return "Asia";
  if (value === "3") return "Asia";
  if (value === "4") return "Africa";
  if (value === "5") return "North America";
  if (value.includes(",")) {
    const first = value.split(",")[0]?.trim() ?? "";
    if (first === "1") return "Europe";
    if (first === "2") return "Asia";
    if (first === "3") return "Asia";
    if (first === "4") return "Africa";
    if (first === "5") return "North America";
  }
  if (value.includes("europe")) return "Europe";
  if (value.includes("africa")) return "Africa";
  if (value.includes("asia") || value.includes("middle east")) return "Asia";
  if (value.includes("north america") || value.includes("caribbean")) return "North America";
  if (value.includes("south america") || value.includes("latin america")) return "South America";
  if (value.includes("oceania")) return "Oceania";
  return "";
}

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
  const { conflicts: currentConflicts } = useCurrentConflicts();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedSourceIds, setSelectedSourceIds] = useState<string[]>([]);
  const [categoryFilter, setCategoryFilter] = useState<AlertCategory | "all">("all");
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>(null);
  const [regionFilter, setRegionFilter] = useState<string>("Europe");
  const [conflictLensId, setConflictLensId] = useState<string | null>(null);
  const [conflictCountryFocus, setConflictCountryFocus] = useState<ConflictCountryFocus | null>(null);
  const { query: searchQuery, setQuery: setSearchQuery, results: searchResults, isApiAvailable } = useSearch();
  const [visibleNowAlertIds, setVisibleNowAlertIds] = useState<string[]>([]);
  const [visibleHistoryAlertIds, setVisibleHistoryAlertIds] = useState<string[]>([]);
  const [mobilePane, setMobilePane] = useState<"intel" | "map" | "alerts">("map");
  const panelRef = useRef<HTMLDivElement>(null);
  const preLensRegionRef = useRef<string | null>(null);
  const preLensSourcesRef = useRef<string[] | null>(null);
  const [utcTime, setUtcTime] = useState(() => new Date().toISOString().slice(0, 19).replace("T", " ") + "Z");
  const activeDynamicConflict = useMemo(
    () => (conflictLensId ? currentConflicts.find((conflict) => conflict.lensIds.includes(conflictLensId)) ?? null : null),
    [conflictLensId, currentConflicts],
  );

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
    preLensRegionRef.current = null;
    preLensSourcesRef.current = null;
    setRegionFilter(nextRegion);
    setConflictLensId(null);
    setConflictCountryFocus(null);
    setSelectedSourceIds([]);
    setSelectedId(null);
  }, []);

  const handleConflictLensChange = useCallback((nextLensId: string | null) => {
    const lens = getConflictLensById(nextLensId);
    if (lens) {
      if (!conflictLensId) {
        preLensRegionRef.current = regionFilter;
        preLensSourcesRef.current = [...selectedSourceIds];
      }
      setConflictLensId(nextLensId);
      setConflictCountryFocus(null);
      setRegionFilter(lens.regionFilter);
      setSelectedSourceIds([]);
    } else if (nextLensId) {
      if (!conflictLensId) {
        preLensRegionRef.current = regionFilter;
        preLensSourcesRef.current = [...selectedSourceIds];
      }
      setConflictLensId(nextLensId);
      setConflictCountryFocus(null);
      const selectedConflict = currentConflicts.find((conflict) => conflict.lensIds.includes(nextLensId));
      const region = normalizeConflictRegion(selectedConflict?.region ?? "");
      if (region) {
        setRegionFilter(region);
      }
      setSelectedSourceIds([]);
    } else {
      setConflictLensId(null);
      setConflictCountryFocus(null);
      if (preLensRegionRef.current) {
        setRegionFilter(preLensRegionRef.current);
      }
      if (preLensSourcesRef.current) {
        setSelectedSourceIds(preLensSourcesRef.current);
      }
      preLensRegionRef.current = null;
      preLensSourcesRef.current = null;
    }
    setSelectedId(null);
  }, [conflictLensId, currentConflicts, regionFilter, selectedSourceIds]);

  const regionScopedAlerts = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    const activeLens = getConflictLensById(conflictLensId);
    const dynamicCountryCodes = new Set((activeDynamicConflict?.overlayCountryCodes ?? []).map((code) => code.toUpperCase()));

    // When API search returned results, use those (already ranked by BM25).
    if (query && isApiAvailable && searchResults.length > 0) {
      let filtered = searchResults;
      if (regionFilter !== "all") {
        filtered = filtered.filter((alert) => alertMatchesRegionFilter(alert, regionFilter));
      }
      if (activeLens) {
        filtered = filtered.filter((alert) => alertMatchesConflictLens(alert, activeLens));
      } else if (conflictLensId && dynamicCountryCodes.size > 0) {
        filtered = filtered.filter((alert) => {
          const code = (alert.event_country_code || alert.source.country_code || "").toUpperCase();
          return dynamicCountryCodes.has(code);
        });
      }
      return filtered;
    }

    // Fallback: client-side filter.
    let filtered = alerts;
    if (regionFilter !== "all") {
      filtered = filtered.filter((alert) => alertMatchesRegionFilter(alert, regionFilter));
    }
    if (activeLens) {
      filtered = filtered.filter((alert) => alertMatchesConflictLens(alert, activeLens));
    } else if (conflictLensId && dynamicCountryCodes.size > 0) {
      filtered = filtered.filter((alert) => {
        const code = (alert.event_country_code || alert.source.country_code || "").toUpperCase();
        return dynamicCountryCodes.has(code);
      });
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
  }, [activeDynamicConflict?.overlayCountryCodes, alerts, conflictLensId, regionFilter, searchQuery, searchResults, isApiAvailable]);

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
    const activeLens = getConflictLensById(conflictLensId);
    const dynamicCountryCodes = new Set((activeDynamicConflict?.overlayCountryCodes ?? []).map((code) => code.toUpperCase()));
    let filtered = stateAlerts;
    if (regionFilter !== "all") {
      filtered = filtered.filter((alert) => alertMatchesRegionFilter(alert, regionFilter));
    }
    if (activeLens) {
      filtered = filtered.filter((alert) => alertMatchesConflictLens(alert, activeLens));
    } else if (conflictLensId && dynamicCountryCodes.size > 0) {
      filtered = filtered.filter((alert) => {
        const code = (alert.event_country_code || alert.source.country_code || "").toUpperCase();
        return dynamicCountryCodes.has(code);
      });
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
  }, [activeDynamicConflict?.overlayCountryCodes, conflictLensId, regionFilter, searchQuery, stateAlerts]);

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

  const handleConflictCountryFocusChange = useCallback((focus: ConflictCountryFocus | null) => {
    if (!focus) {
      setConflictCountryFocus(null);
      return;
    }
    if (focus.lensId) {
      handleConflictLensChange(focus.lensId);
    } else {
      handleCountrySelect(focus.code);
    }
    setConflictCountryFocus(focus);
  }, [handleConflictLensChange, handleCountrySelect]);

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

  return (
    <div className="flex h-[100dvh] flex-col bg-siem-bg text-siem-text">
      <Header
        regionFilter={regionFilter}
        onRegionChange={handleRegionChange}
        conflictLensId={conflictLensId}
        onConflictLensChange={handleConflictLensChange}
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
          <Suspense fallback={<div className="flex h-full items-center justify-center text-sm text-siem-muted">Loading intel panel...</div>}>
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
          </Suspense>
        </div>

        {/* Center — map */}
        <div className={`${mobilePane === "map" ? "block" : "hidden"} md:block min-h-0 flex-1 min-w-0`}>
          <Suspense fallback={<div className="flex h-full items-center justify-center text-sm text-siem-muted">Loading map...</div>}>
            <GlobeView
              alerts={scopedAlerts}
              historicalAlerts={scopedStateAlerts}
              selectedId={selectedId}
              onSelect={setSelectedId}
              regionFilter={regionFilter}
              onRegionChange={handleRegionChange}
              conflictLensId={conflictLensId}
              onConflictCountryFocusChange={handleConflictCountryFocusChange}
              visibleNowAlertIds={visibleNowAlertIds}
              visibleHistoryAlertIds={visibleHistoryAlertIds}
              onSelectSourceIdsChange={handleSourceSelectionChange}
              selectedSourceIds={selectedSourceIds}
            />
          </Suspense>
        </div>

        {/* Right panel — alert queue (contained, scrollable) */}
        <div className={`${mobilePane === "alerts" ? "block" : "hidden"} md:block w-full md:w-[24rem] md:shrink-0 min-h-0`}>
          <div className="flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_24px_80px_rgba(0,0,0,0.28)]">
            {isLoading ? (
              <div className="flex flex-1 items-center justify-center text-sm text-siem-muted">
                Loading live alert queue...
              </div>
            ) : (
              <Suspense fallback={<div className="flex flex-1 items-center justify-center text-sm text-siem-muted">Loading alert queue...</div>}>
                <AlertFeed
                  alerts={scopedAlerts}
                  historicalAlerts={scopedStateAlerts}
                  selectedId={selectedId}
                  onSelect={setSelectedId}
                  categoryFilter={categoryFilter}
                  regionFilter={regionFilter}
                  conflictLensId={conflictLensId}
                  conflictCountryFocus={conflictCountryFocus}
                  onRegionChange={handleRegionChange}
                  onVisibleAlertIdsChange={({ nowIds, historyIds }) => {
                    setVisibleNowAlertIds(nowIds);
                    setVisibleHistoryAlertIds(historyIds);
                  }}
                />
              </Suspense>
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
              <Suspense fallback={<div className="flex h-full w-full items-center justify-center text-sm text-siem-muted">Loading alert details...</div>}>
                <AlertDetail alert={selectedAlert} onClose={handleClose} />
              </Suspense>
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
