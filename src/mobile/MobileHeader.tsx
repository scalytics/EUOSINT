import { useState } from "react";
import { Shield, ChevronDown, Check, Filter } from "lucide-react";
import type { AlertCategory } from "@/types/alert";
import { categoryLabels } from "@/lib/severity";

const REGIONS = [
  { value: "all", label: "Global" },
  { value: "Europe", label: "Europe" },
  { value: "Middle East", label: "Middle East" },
  { value: "Africa", label: "Africa" },
  { value: "North America", label: "North America" },
  { value: "Asia-Pacific", label: "Asia-Pacific" },
  { value: "Caribbean", label: "Caribbean" },
];

interface Props {
  regionFilter: string;
  onRegionChange: (region: string) => void;
  categoryFilter: Set<AlertCategory>;
  onCategoryChange: (c: Set<AlertCategory>) => void;
  categoriesWithCounts: Array<{ value: AlertCategory; label: string; count: number }>;
  clock: string;
}

export function MobileHeader({
  regionFilter,
  onRegionChange,
  categoryFilter,
  onCategoryChange,
  categoriesWithCounts,
  clock,
}: Props) {
  const [regionPickerOpen, setRegionPickerOpen] = useState(false);
  const [categoryPickerOpen, setCategoryPickerOpen] = useState(false);

  function selectRegion(value: string) {
    onRegionChange(value);
    setRegionPickerOpen(false);
  }

  function toggleCategory(cat: AlertCategory) {
    const next = new Set(categoryFilter);
    if (next.has(cat)) next.delete(cat);
    else next.add(cat);
    onCategoryChange(next);
  }

  function clearCategories() {
    onCategoryChange(new Set());
  }

  const currentRegionLabel = REGIONS.find((r) => r.value === regionFilter)?.label ?? "Global";
  const hasFilter = categoryFilter.size > 0;

  // Short label for category pill
  let categoryPillLabel: string;
  if (!hasFilter) {
    categoryPillLabel = "Category";
  } else if (categoryFilter.size === 1) {
    const cat = [...categoryFilter][0];
    categoryPillLabel = categoryLabels[cat] ?? cat;
  } else {
    categoryPillLabel = `${categoryFilter.size} categories`;
  }

  return (
    <>
      <div className="mobile-header">
        <Shield size={18} className="text-sky-400 flex-shrink-0" />
        <span className="text-xs font-bold tracking-wide text-slate-300">kafSIEM</span>

        <div className="flex items-center gap-1 ml-auto">
          {/* Category pill */}
          <button
            className={`mobile-region-pill ${hasFilter ? "mobile-region-pill--active" : ""}`}
            onClick={() => setCategoryPickerOpen(true)}
          >
            <Filter size={11} />
            {categoryPillLabel}
            <ChevronDown size={12} />
          </button>

          {/* Region pill */}
          <button
            className="mobile-region-pill"
            onClick={() => setRegionPickerOpen(true)}
          >
            {currentRegionLabel}
            <ChevronDown size={12} />
          </button>
        </div>

        <span className="text-[10px] font-mono text-slate-500 tabular-nums">{clock}</span>
      </div>

      {/* Region picker sheet */}
      {regionPickerOpen && (
        <>
          <div
            className="mobile-sheet-backdrop"
            onClick={() => setRegionPickerOpen(false)}
          />
          <div className="mobile-picker-sheet">
            <div className="mobile-sheet-handle" />
            <div className="mobile-picker-title">Select Region</div>
            {REGIONS.map((r) => (
              <button
                key={r.value}
                className={`mobile-picker-item ${regionFilter === r.value ? "active" : ""}`}
                onClick={() => selectRegion(r.value)}
              >
                <span>{r.label}</span>
                {regionFilter === r.value && (
                  <Check size={16} className="text-sky-400" />
                )}
              </button>
            ))}
          </div>
        </>
      )}

      {/* Category picker sheet */}
      {categoryPickerOpen && (
        <>
          <div
            className="mobile-sheet-backdrop"
            onClick={() => setCategoryPickerOpen(false)}
          />
          <div className="mobile-picker-sheet">
            <div className="mobile-sheet-handle" />
            <div
              className="mobile-picker-title"
              style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}
            >
              <span>Filter by Category</span>
              {hasFilter && (
                <button
                  className="mobile-clear-btn"
                  onClick={clearCategories}
                >
                  Clear all
                </button>
              )}
            </div>
            {categoriesWithCounts.map(({ value, label, count }) => (
              <button
                key={value}
                className={`mobile-picker-item ${categoryFilter.has(value) ? "active" : ""}`}
                onClick={() => toggleCategory(value)}
              >
                <span>{label}</span>
                <span className="flex items-center gap-2">
                  <span className="text-xs opacity-50">{count}</span>
                  {categoryFilter.has(value) && <Check size={16} className="text-sky-400" />}
                </span>
              </button>
            ))}
          </div>
        </>
      )}
    </>
  );
}
