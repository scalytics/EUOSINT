/*
 * EUOSINT
 * Open-source OSINT pipeline distribution.
 * See LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { Globe2, Radar, Search, Shield, X } from "lucide-react";
import type { Alert } from "@/types/alert";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { useCurrentConflicts } from "@/hooks/useCurrentConflicts";

type MenuView = "overview" | "feeds" | "sources" | "health";

interface Props {
  regionFilter: string;
  onRegionChange: (region: string) => void;
  conflictLensId: string | null;
  onConflictLensChange: (lensId: string | null) => void;
  sourceCount: number;
  selectedSourceIds: string[];
  onSelectedSourceIdsChange: (sourceIds: string[]) => void;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  activeMenu: MenuView;
  onMenuChange: (view: MenuView) => void;
  alerts: Alert[];
}

const REGIONS = [
  "Europe",
  "Africa",
  "North America",
  "Asia",
  "all",
];

const SEARCH_HISTORY_COOKIE = "euosint_search_history";
const REPO_URL = "https://github.com/scalytics/EUOSINT";

function readSearchHistory(): string[] {
  if (typeof document === "undefined") return [];
  const cookie = document.cookie
    .split("; ")
    .find((entry) => entry.startsWith(`${SEARCH_HISTORY_COOKIE}=`));
  if (!cookie) return [];
  try {
    const value = decodeURIComponent(cookie.split("=").slice(1).join("="));
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === "string") : [];
  } catch {
    return [];
  }
}

function writeSearchHistory(history: string[]) {
  if (typeof document === "undefined") return;
  const expires = new Date();
  expires.setMonth(expires.getMonth() + 6);
  document.cookie = `${SEARCH_HISTORY_COOKIE}=${encodeURIComponent(JSON.stringify(history))}; expires=${expires.toUTCString()}; path=/; SameSite=Lax`;
}

function FeedFocus({
  alerts,
  sourceCount,
  selectedSourceIds,
  onSelectedSourceIdsChange,
  onClear,
}: {
  alerts: Alert[];
  sourceCount: number;
  selectedSourceIds: string[];
  onSelectedSourceIdsChange: (sourceIds: string[]) => void;
  onClear: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const sources = useMemo(() => {
    const map = new Map<string, { id: string; name: string; country: string; count: number }>();
    for (const alert of alerts) {
      const existing = map.get(alert.source_id);
      if (existing) {
        existing.count++;
      } else {
        map.set(alert.source_id, {
          id: alert.source_id,
          name: alert.source.authority_name,
          country: alert.source.country,
          count: 1,
        });
      }
    }
    return [...map.values()].sort((a, b) => {
      if (b.count !== a.count) return b.count - a.count;
      return a.name.localeCompare(b.name);
    });
  }, [alerts]);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const filteredSources = useMemo(() => {
    const trimmed = query.trim().toLowerCase();
    if (!trimmed) return sources;
    return sources.filter((source) =>
      `${source.name} ${source.country} ${source.id}`.toLowerCase().includes(trimmed),
    );
  }, [query, sources]);

  const selectedLabels = useMemo(() => {
    if (selectedSourceIds.length === 0) return `All ${sourceCount} sources`;
    const labels = selectedSourceIds
      .map((sourceId) => sources.find((source) => source.id === sourceId)?.name ?? sourceId)
      .slice(0, 2);
    if (selectedSourceIds.length <= 2) return labels.join(", ");
    return `${labels.join(", ")} +${selectedSourceIds.length - 2}`;
  }, [selectedSourceIds, sourceCount, sources]);

  const toggleSource = (sourceId: string) => {
    if (selectedSourceIds.includes(sourceId)) {
      onSelectedSourceIdsChange(selectedSourceIds.filter((id) => id !== sourceId));
      return;
    }
    onSelectedSourceIdsChange([...selectedSourceIds, sourceId]);
  };

  return (
    <div ref={containerRef} className="relative">
      <div className="rounded-2xl border border-siem-border bg-siem-panel-strong px-3 py-2">
        <div className="mb-1 flex items-center justify-between gap-2 text-2xs uppercase tracking-[0.18em] text-siem-muted">
          <span className="inline-flex items-center gap-2">
            <Radar size={11} />
            Feed focus
          </span>
          {selectedSourceIds.length > 0 && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onClear();
                setOpen(false);
                setQuery("");
              }}
              className="rounded-full border border-siem-border/70 p-1 text-siem-muted transition-colors hover:border-siem-accent/35 hover:text-siem-text"
              aria-label="Clear feed focus"
              title="Clear feed focus"
            >
              <X size={11} />
            </button>
          )}
        </div>
        <button
          type="button"
          onClick={() => {
            setOpen((current) => !current);
            setQuery("");
            setTimeout(() => inputRef.current?.focus(), 50);
          }}
          className="flex items-center gap-2 bg-transparent text-left text-sm text-siem-text outline-none cursor-pointer"
        >
          <span className="max-w-[14rem] truncate">{selectedLabels}</span>
          <Search size={12} className="shrink-0 text-siem-muted" />
        </button>
      </div>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-80 rounded-xl border border-siem-border bg-siem-panel-strong shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div className="relative border-b border-siem-border p-2">
            <Search size={12} className="absolute left-4 top-1/2 -translate-y-1/2 text-siem-muted" />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search feeds, agencies, countries..."
              className="w-full bg-transparent pl-7 pr-7 py-1.5 text-xs text-siem-text placeholder:text-siem-muted/60 outline-none"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery("")}
                className="absolute right-4 top-1/2 -translate-y-1/2 text-siem-muted hover:text-siem-text"
              >
                <X size={12} />
              </button>
            )}
          </div>
          <div className="border-b border-siem-border p-2">
            <button
              type="button"
              onClick={() => onSelectedSourceIdsChange([])}
              className="w-full rounded-lg border border-siem-accent/35 bg-siem-accent/12 px-3 py-2 text-left text-xxs uppercase tracking-[0.16em] text-siem-text"
            >
              All feeds
            </button>
          </div>
          <div className="max-h-72 overflow-y-auto p-1">
            {filteredSources.length === 0 && (
              <div className="px-3 py-4 text-center text-xxs text-siem-muted">No matching feeds</div>
            )}
            {filteredSources.map((source) => {
              const selected = selectedSourceIds.includes(source.id);
              return (
                <button
                  key={source.id}
                  type="button"
                  onClick={() => toggleSource(source.id)}
                  className={`flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-xs transition-colors ${
                    selected
                      ? "bg-siem-accent/14 text-siem-text"
                      : "text-siem-text hover:bg-siem-accent/8"
                  }`}
                >
                  <span className="min-w-0">
                    <span className="block truncate">{source.name}</span>
                    <span className="block truncate text-2xs text-siem-muted">
                      {source.country} · {source.id}
                    </span>
                  </span>
                  <span className="shrink-0 text-2xs text-siem-muted">{source.count}</span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

function SearchBar({
  query,
  onQueryChange,
}: {
  query: string;
  onQueryChange: (query: string) => void;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setHistory(readSearchHistory());
  }, []);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const filteredHistory = useMemo(() => {
    const trimmed = query.trim().toLowerCase();
    if (!trimmed) return history;
    return history.filter((item) => item.toLowerCase().includes(trimmed));
  }, [history, query]);

  const commitQuery = (value: string) => {
    const trimmed = value.trim();
    onQueryChange(trimmed);
    if (!trimmed) return;
    const nextHistory = [trimmed, ...history.filter((item) => item !== trimmed)].slice(0, 8);
    setHistory(nextHistory);
    writeSearchHistory(nextHistory);
  };

  return (
    <div ref={containerRef} className="relative w-full max-w-2xl">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          commitQuery(query);
          setIsOpen(false);
        }}
        className="relative"
      >
        <Search size={14} className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-siem-muted" />
        <input
          ref={inputRef}
          type="search"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onFocus={() => setIsOpen(true)}
          placeholder="Search alerts, agencies, countries, categories..."
          className="w-full rounded-full border border-siem-border bg-siem-panel-strong pl-10 pr-36 py-3 text-sm text-siem-text outline-none transition-colors placeholder:text-siem-muted/60 focus:border-siem-accent/45"
        />
        <div className="absolute right-2 top-1/2 flex -translate-y-1/2 items-center gap-1.5">
          <button
            type="submit"
            className="rounded-full border border-siem-accent/35 bg-siem-accent/14 px-3 py-1.5 text-2xs uppercase tracking-[0.16em] text-siem-text"
          >
            Search
          </button>
        </div>
      </form>

      {isOpen && filteredHistory.length > 0 && (
        <div className="absolute left-0 right-0 top-full z-50 mt-2 rounded-2xl border border-siem-border bg-siem-panel-strong shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div className="border-b border-siem-border px-4 py-2 text-2xs uppercase tracking-[0.18em] text-siem-muted">
            Recent queries
          </div>
          <div className="p-2">
            {filteredHistory.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => {
                  onQueryChange(item);
                  commitQuery(item);
                  setIsOpen(false);
                }}
                className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-xs text-siem-text transition-colors hover:bg-siem-accent/8"
              >
                <Search size={12} className="shrink-0 text-siem-muted" />
                <span className="truncate">{item}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function RegionSearch({
  regionFilter,
  onRegionChange,
  alerts,
  onClear,
}: {
  regionFilter: string;
  onRegionChange: (region: string) => void;
  alerts: Alert[];
  onClear: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Build searchable items: regions + unique countries.
  const items = useMemo(() => {
    const regionItems = REGIONS.map((r) => ({
      value: r,
      label: r === "all" ? "Global" : r,
      type: "region" as const,
      count: r === "all" ? alerts.length : alerts.filter((a) => alertMatchesRegionFilter(a, r)).length,
    }));

    const countryMap = new Map<string, { country: string; code: string; count: number }>();
    for (const a of alerts) {
      const key = a.source.country_code;
      const existing = countryMap.get(key);
      if (existing) {
        existing.count++;
      } else {
        countryMap.set(key, { country: a.source.country, code: a.source.country_code, count: 1 });
      }
    }
    const countryItems = [...countryMap.values()]
      .sort((a, b) => b.count - a.count)
      .map((c) => ({
        value: `country:${c.code}`,
        label: `${c.country} (${c.code})`,
        type: "country" as const,
        count: c.count,
      }));

    return [...regionItems, ...countryItems];
  }, [alerts]);

  const filtered = useMemo(() => {
    const q = query.toLowerCase().trim();
    if (!q) return items.filter((i) => i.count > 0 || i.type === "region");
    return items.filter((i) => i.label.toLowerCase().includes(q));
  }, [items, query]);

  // Close dropdown on outside click.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const currentLabel = useMemo(() => {
    if (regionFilter.startsWith("country:")) {
      const code = regionFilter.slice(8);
      const match = items.find((i) => i.value === regionFilter);
      return match ? match.label : code;
    }
    return regionFilter === "all" ? "Global" : regionFilter;
  }, [regionFilter, items]);

  return (
    <div ref={containerRef} className="relative">
      <div className="rounded-2xl border border-siem-border bg-siem-panel-strong px-3 py-2">
        <div className="mb-1 flex items-center justify-between gap-2 text-2xs uppercase tracking-[0.18em] text-siem-muted">
          <span className="inline-flex items-center gap-2">
            <Globe2 size={11} />
            Region scope
          </span>
          {regionFilter !== "all" && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onClear();
                setOpen(false);
                setQuery("");
              }}
              className="rounded-full border border-siem-border/70 p-1 text-siem-muted transition-colors hover:border-siem-accent/35 hover:text-siem-text"
              aria-label="Clear region scope"
              title="Clear region scope"
            >
              <X size={11} />
            </button>
          )}
        </div>
        <button
          type="button"
          onClick={() => {
            setOpen(!open);
            setQuery("");
            setTimeout(() => inputRef.current?.focus(), 50);
          }}
          className="flex items-center gap-2 bg-transparent text-sm text-siem-text outline-none cursor-pointer"
        >
          <span className="truncate max-w-[14rem]">{currentLabel}</span>
          <Search size={12} className="text-siem-muted shrink-0" />
        </button>
      </div>

      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 w-72 rounded-xl border border-siem-border bg-siem-panel-strong shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div className="relative border-b border-siem-border p-2">
            <Search size={12} className="absolute left-4 top-1/2 -translate-y-1/2 text-siem-muted" />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search regions, countries..."
              className="w-full bg-transparent pl-7 pr-7 py-1.5 text-xs text-siem-text placeholder:text-siem-muted/60 outline-none"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery("")}
                className="absolute right-4 top-1/2 -translate-y-1/2 text-siem-muted hover:text-siem-text"
              >
                <X size={12} />
              </button>
            )}
          </div>
          <div className="max-h-64 overflow-y-auto p-1">
            {filtered.length === 0 && (
              <div className="px-3 py-4 text-center text-xxs text-siem-muted">No matches</div>
            )}
            {filtered.map((item) => (
              <button
                key={item.value}
                type="button"
                onClick={() => {
                  onRegionChange(item.value);
                  setOpen(false);
                  setQuery("");
                }}
                className={`w-full flex items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-xs transition-colors ${
                  regionFilter === item.value
                    ? "bg-siem-accent/14 text-siem-text"
                    : "text-siem-text hover:bg-siem-accent/8"
                }`}
              >
                <span className="flex items-center gap-2 min-w-0">
                  <span
                    className={`inline-block w-1.5 h-1.5 rounded-full shrink-0 ${
                      item.type === "region" ? "bg-siem-accent" : "bg-siem-info"
                    }`}
                  />
                  <span className="truncate">{item.label}</span>
                </span>
                <span className="text-2xs text-siem-muted shrink-0">{item.count}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ConflictLensSearch({
  conflictLensId,
  onConflictLensChange,
  onClear,
}: {
  conflictLensId: string | null;
  onConflictLensChange: (lensId: string | null) => void;
  onClear: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const { conflicts: currentConflicts } = useCurrentConflicts();

  const clusterLabel = (conflict: {
    title: string;
    sideA?: string;
    sideB?: string;
    overlayCountryCodes?: string[];
    region?: string;
  }): string => {
    const normalizeRegion = (raw?: string): string => {
      const value = (raw ?? "").trim().toLowerCase();
      if (!value) return "Other";
      if (value === "1") return "Europe";
      if (value === "2" || value === "3") return "Asia";
      if (value === "4") return "Africa";
      if (value === "5") return "North America";
      if (value.includes(",")) {
        const first = value.split(",")[0]?.trim() ?? "";
        if (first === "1") return "Europe";
        if (first === "2" || first === "3") return "Asia";
        if (first === "4") return "Africa";
        if (first === "5") return "North America";
      }
      if (value.includes("europe")) return "Europe";
      if (value.includes("africa")) return "Africa";
      if (value.includes("asia") || value.includes("middle east")) return "Asia";
      if (value.includes("north america") || value.includes("caribbean")) return "North America";
      if (value.includes("south america") || value.includes("latin america")) return "South America";
      if (value.includes("oceania")) return "Oceania";
      return "Other";
    };

    const codes = new Set((conflict.overlayCountryCodes ?? []).map((code) => code.toUpperCase()));
    const text = `${conflict.title} ${conflict.sideA ?? ""} ${conflict.sideB ?? ""}`.toLowerCase();
    if (codes.has("IL") || codes.has("PS") || codes.has("LB") || text.includes("gaza") || text.includes("israel")) {
      return "Israel / Gaza";
    }
    if (codes.has("UA") || codes.has("RU") || text.includes("ukraine") || text.includes("russia")) {
      return "Ukraine";
    }
    if (codes.has("CD") || text.includes("congo")) {
      return "DRC";
    }
    if (codes.has("SD") || codes.has("SS") || text.includes("sudan")) {
      return "Sudan";
    }
    return normalizeRegion(conflict.region);
  };

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const titleCounts = new Map<string, number>();
    for (const conflict of currentConflicts) {
      const key = (conflict.title || "").trim().toLowerCase();
      if (!key) continue;
      titleCounts.set(key, (titleCounts.get(key) ?? 0) + 1);
    }
    const dynamicRows = currentConflicts
      .filter((conflict) => {
        if (!q) return true;
        return `${conflict.title} ${conflict.sideA ?? ""} ${conflict.sideB ?? ""} ${conflict.typeOfConflict ?? ""}`.toLowerCase().includes(q);
      })
      .slice(0, q ? 50 : 20)
      .map((conflict) => {
        const parties = [conflict.sideA, conflict.sideB].filter((value) => (value ?? "").trim().length > 0).join(" vs ");
        const titleKey = (conflict.title || "").trim().toLowerCase();
        const duplicateTitle = (titleCounts.get(titleKey) ?? 0) > 1;
        return {
          key: `conflict:${conflict.conflictId}`,
          label: duplicateTitle && parties ? `${conflict.title} (${parties})` : conflict.title,
          description: `${conflict.typeOfConflict ?? "Conflict"} · ${conflict.year}`,
          parties,
          lensId: conflict.lensIds[0] ?? "",
          sourceUrl: conflict.sourceUrl ?? "",
          hasMapProfile: (conflict.overlayCountryCodes ?? []).length > 0,
          cluster: clusterLabel(conflict),
          kind: "conflict" as const,
        };
      });

    return dynamicRows;
  }, [currentConflicts, query]);

  const grouped = useMemo(() => {
    const groups = new Map<string, typeof filtered>();
    for (const row of filtered) {
      const key = row.cluster || "Other";
      const bucket = groups.get(key);
      if (bucket) {
        bucket.push(row);
      } else {
        groups.set(key, [row]);
      }
    }
    return [...groups.entries()];
  }, [filtered]);

  const currentLabel = useMemo(() => {
    if (!conflictLensId) return "None";
    const current = currentConflicts.find((conflict) => conflict.lensIds.includes(conflictLensId));
    if (current) return current.title;
    return "Selected conflict";
  }, [conflictLensId, currentConflicts]);

  return (
    <div ref={containerRef} className="relative">
      <div className="rounded-2xl border border-siem-border bg-siem-panel-strong px-3 py-2">
        <div className="mb-1 flex items-center justify-between gap-2 text-2xs uppercase tracking-[0.18em] text-siem-muted">
          <span className="inline-flex items-center gap-2">
            <Radar size={11} />
            Conflict lens
          </span>
          {conflictLensId && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onClear();
                setOpen(false);
                setQuery("");
              }}
              className="rounded-full border border-siem-border/70 p-1 text-siem-muted transition-colors hover:border-siem-accent/35 hover:text-siem-text"
              aria-label="Clear conflict lens"
              title="Clear conflict lens"
            >
              <X size={11} />
            </button>
          )}
        </div>
        <button
          type="button"
          onClick={() => {
            setOpen((current) => !current);
            setQuery("");
            setTimeout(() => inputRef.current?.focus(), 50);
          }}
          className="flex items-center gap-2 bg-transparent text-left text-sm text-siem-text outline-none cursor-pointer"
        >
          <span className="max-w-[14rem] truncate">{currentLabel}</span>
          <Search size={12} className="shrink-0 text-siem-muted" />
        </button>
      </div>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-80 rounded-xl border border-siem-border bg-siem-panel-strong shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div className="relative border-b border-siem-border p-2">
            <Search size={12} className="absolute left-4 top-1/2 -translate-y-1/2 text-siem-muted" />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search conflict zones..."
              className="w-full bg-transparent pl-7 pr-7 py-1.5 text-xs text-siem-text placeholder:text-siem-muted/60 outline-none"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery("")}
                className="absolute right-4 top-1/2 -translate-y-1/2 text-siem-muted hover:text-siem-text"
              >
                <X size={12} />
              </button>
            )}
          </div>
          <div className="border-b border-siem-border p-2">
            <button
              type="button"
              onClick={() => {
                onConflictLensChange(null);
                setOpen(false);
              }}
              className="w-full rounded-lg border border-siem-accent/35 bg-siem-accent/12 px-3 py-2 text-left text-xxs uppercase tracking-[0.16em] text-siem-text"
            >
              No lens
            </button>
          </div>
          <div className="max-h-72 overflow-y-auto p-1">
            {filtered.length === 0 && (
              <div className="px-3 py-4 text-center text-xxs text-siem-muted">No matching zones</div>
            )}
            {grouped.map(([group, rows]) => (
              <div key={group} className="mb-2">
                <div className="px-2 py-1 text-2xs uppercase tracking-[0.12em] text-siem-muted">{group}</div>
                {rows.map((row) => (
                  <button
                    key={row.key}
                    type="button"
                    onClick={() => {
                      if (!row.lensId) return;
                      onConflictLensChange(row.lensId);
                      setOpen(false);
                      setQuery("");
                    }}
                    className={`w-full rounded-lg px-3 py-2 text-left text-xs transition-colors ${
                      conflictLensId === row.lensId
                        ? "bg-siem-accent/14 text-siem-text"
                        : row.lensId
                          ? "text-siem-text hover:bg-siem-accent/8"
                          : "text-siem-muted/80"
                    }`}
                  >
                    <div className="truncate">{row.label}</div>
                    <div className="mt-0.5 text-2xs text-siem-muted">{row.description}</div>
                    {row.parties && (
                      <div className="mt-1 text-2xs text-siem-muted line-clamp-2">{row.parties}</div>
                    )}
                    {row.kind === "conflict" && row.sourceUrl && (
                      <div className="mt-1 text-2xs uppercase tracking-[0.12em] text-siem-accent">
                        UCDP current conflict
                      </div>
                    )}
                    {row.kind === "conflict" && !row.hasMapProfile && (
                      <div className="mt-1 text-2xs uppercase tracking-[0.12em] text-siem-muted">
                        No map profile yet
                      </div>
                    )}
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export function Header({
  regionFilter,
  onRegionChange,
  conflictLensId,
  onConflictLensChange,
  sourceCount,
  selectedSourceIds,
  onSelectedSourceIdsChange,
  searchQuery,
  onSearchChange,
  activeMenu,
  onMenuChange,
  alerts,
}: Props) {
  void activeMenu;
  void onMenuChange;
  return (
    <header className="border-b border-siem-border bg-siem-panel/96 px-4 py-3 md:px-5">
      <div className="flex flex-col gap-3">
        <div className="grid gap-3 md:grid-cols-[auto_minmax(18rem,1fr)_auto] md:items-center">
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-orange-500/40 bg-orange-500/12 text-orange-300 shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]">
              <Shield size={20} strokeWidth={2.1} />
            </div>
            <div>
              <div className="text-xxs uppercase tracking-[0.22em] text-siem-muted">
                Scalytics OSINT{" "}
                <a
                  href={REPO_URL}
                  target="_blank"
                  rel="noreferrer"
                  className="text-siem-accent hover:text-siem-text transition-colors"
                  title="Open repository"
                >
                  {`${__APP_VERSION__}`}
                </a>
              </div>
              <div className="mt-1 text-xl font-semibold tracking-[0.02em] text-siem-text">
                Open Source Intelligence Console
              </div>
            </div>
          </div>

          <div className="md:px-3">
            <SearchBar query={searchQuery} onQueryChange={onSearchChange} />
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <RegionSearch
              regionFilter={regionFilter}
              onRegionChange={onRegionChange}
              alerts={alerts}
              onClear={() => onRegionChange("all")}
            />

            <ConflictLensSearch
              conflictLensId={conflictLensId}
              onConflictLensChange={onConflictLensChange}
              onClear={() => onConflictLensChange(null)}
            />

            <FeedFocus
              alerts={alerts}
              sourceCount={sourceCount}
              selectedSourceIds={selectedSourceIds}
              onSelectedSourceIdsChange={onSelectedSourceIdsChange}
              onClear={() => onSelectedSourceIdsChange([])}
            />
          </div>
        </div>

      </div>
    </header>
  );
}
