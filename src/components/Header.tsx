/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { Globe2, Radar, Search, Shield, X } from "lucide-react";
import type { Alert } from "@/types/alert";

type MenuView = "overview" | "feeds" | "authorities" | "health";

interface Props {
  regionFilter: string;
  onRegionChange: (region: string) => void;
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
  "all",
  "North America",
  "South America",
  "Africa",
  "Asia",
  "Oceania",
  "Caribbean",
  "International",
];

const _MENU_ITEMS: { key: MenuView; label: string }[] = [
  { key: "overview", label: "Overview" },
  { key: "feeds", label: "Feeds" },
  { key: "authorities", label: "Authorities" },
  { key: "health", label: "Source Health" },
];

const SEARCH_HISTORY_COOKIE = "euosint_search_history";

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
}: {
  alerts: Alert[];
  sourceCount: number;
  selectedSourceIds: string[];
  onSelectedSourceIdsChange: (sourceIds: string[]) => void;
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
    if (selectedSourceIds.length === 0) return `All ${sourceCount} authorities`;
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
        <div className="mb-1 flex items-center gap-2 text-[10px] uppercase tracking-[0.18em] text-siem-muted">
          <Radar size={11} />
          Feed focus
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
              className="w-full rounded-lg border border-siem-accent/35 bg-siem-accent/12 px-3 py-2 text-left text-[11px] uppercase tracking-[0.16em] text-siem-text"
            >
              All feeds
            </button>
          </div>
          <div className="max-h-72 overflow-y-auto p-1">
            {filteredSources.length === 0 && (
              <div className="px-3 py-4 text-center text-[11px] text-siem-muted">No matching feeds</div>
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
                    <span className="block truncate text-[10px] text-siem-muted">
                      {source.country} · {source.id}
                    </span>
                  </span>
                  <span className="shrink-0 text-[10px] text-siem-muted">{source.count}</span>
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
          className="w-full rounded-full border border-siem-border bg-siem-panel-strong pl-10 pr-24 py-3 text-sm text-siem-text outline-none transition-colors placeholder:text-siem-muted/60 focus:border-siem-accent/45"
        />
        {query && (
          <button
            type="button"
            onClick={() => {
              onQueryChange("");
              inputRef.current?.focus();
              setIsOpen(true);
            }}
            className="absolute right-14 top-1/2 -translate-y-1/2 text-siem-muted hover:text-siem-text"
          >
            <X size={13} />
          </button>
        )}
        <button
          type="submit"
          className="absolute right-2 top-1/2 -translate-y-1/2 rounded-full border border-siem-accent/35 bg-siem-accent/14 px-3 py-1.5 text-[10px] uppercase tracking-[0.16em] text-siem-text"
        >
          Search
        </button>
      </form>

      {isOpen && filteredHistory.length > 0 && (
        <div className="absolute left-0 right-0 top-full z-50 mt-2 rounded-2xl border border-siem-border bg-siem-panel-strong shadow-[0_16px_48px_rgba(0,0,0,0.4)]">
          <div className="border-b border-siem-border px-4 py-2 text-[10px] uppercase tracking-[0.18em] text-siem-muted">
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
}: {
  regionFilter: string;
  onRegionChange: (region: string) => void;
  alerts: Alert[];
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Build searchable items: regions + unique countries.
  const items = useMemo(() => {
    const regionItems = REGIONS.map((r) => ({
      value: r,
      label: r === "all" ? "All regions" : r,
      type: "region" as const,
      count: r === "all" ? alerts.length : alerts.filter((a) => a.source.region === r).length,
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
    return regionFilter === "all" ? "All regions" : regionFilter;
  }, [regionFilter, items]);

  return (
    <div ref={containerRef} className="relative">
      <div className="rounded-2xl border border-siem-border bg-siem-panel-strong px-3 py-2">
        <div className="mb-1 flex items-center gap-2 text-[10px] uppercase tracking-[0.18em] text-siem-muted">
          <Globe2 size={11} />
          Region scope
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
              placeholder="Search regions, countries\u2026"
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
              <div className="px-3 py-4 text-center text-[11px] text-siem-muted">No matches</div>
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
                <span className="text-[10px] text-siem-muted shrink-0">{item.count}</span>
              </button>
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
  sourceCount,
  selectedSourceIds,
  onSelectedSourceIdsChange,
  searchQuery,
  onSearchChange,
  activeMenu,
  onMenuChange,
  alerts,
}: Props) {
  return (
    <header className="border-b border-siem-border bg-siem-panel/96 px-4 py-3 md:px-5">
      <div className="flex flex-col gap-3">
        <div className="grid gap-3 md:grid-cols-[auto_minmax(18rem,1fr)_auto] md:items-center">
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-orange-500/40 bg-orange-500/12 text-orange-300 shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]">
              <Shield size={20} strokeWidth={2.1} />
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-[0.22em] text-siem-muted">
                Scalytics OSINT
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
            />

            <FeedFocus
              alerts={alerts}
              sourceCount={sourceCount}
              selectedSourceIds={selectedSourceIds}
              onSelectedSourceIdsChange={onSelectedSourceIdsChange}
            />
          </div>
        </div>

      </div>
    </header>
  );
}
