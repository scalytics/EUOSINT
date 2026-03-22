import { useSearch } from "@/hooks/useSearch";
import { MobileAlertCard } from "./MobileAlertCard";
import type { AlertCategory } from "@/types/alert";
import { categoryLabels, categoryOrder } from "@/lib/severity";
import { useState, useMemo } from "react";
import { SearchX, WifiOff } from "lucide-react";

interface Props {
  onSelectAlert: (alertId: string) => void;
}

// Show a useful subset of categories as quick-filter chips
const CHIP_CATEGORIES: AlertCategory[] = categoryOrder.slice(0, 10);

export function MobileSearch({ onSelectAlert }: Props) {
  const { query, setQuery, results, isSearching, isApiAvailable } = useSearch();
  const [categoryFilter, setCategoryFilter] = useState<AlertCategory | null>(null);

  const filtered = useMemo(
    () =>
      categoryFilter
        ? results.filter((a) => a.category === categoryFilter)
        : results,
    [results, categoryFilter],
  );

  if (isApiAvailable === false) {
    return (
      <div className="mobile-empty">
        <WifiOff size={32} />
        <span>Search API unavailable</span>
        <span className="text-xs">The collector API is not reachable</span>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      <div className="px-3 pt-3">
        <input
          className="mobile-search-input"
          type="search"
          inputMode="search"
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck={false}
          enterKeyHint="search"
          placeholder="Search alerts..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          autoFocus
        />

        <div className="mobile-search-chips">
          {CHIP_CATEGORIES.map((cat) => (
            <button
              key={cat}
              className={`mobile-search-chip ${categoryFilter === cat ? "active" : ""}`}
              onClick={() =>
                setCategoryFilter(categoryFilter === cat ? null : cat)
              }
            >
              {categoryLabels[cat]}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {isSearching && (
          <div className="flex justify-center py-8">
            <div className="mobile-ptr-spinner" />
          </div>
        )}

        {!isSearching && query.trim() && filtered.length === 0 && (
          <div className="mobile-empty">
            <SearchX size={32} />
            <span>No results for "{query}"</span>
          </div>
        )}

        {!isSearching &&
          filtered.map((alert) => (
            <MobileAlertCard
              key={alert.alert_id}
              alert={alert}
              onSelect={onSelectAlert}
            />
          ))}
      </div>
    </div>
  );
}
