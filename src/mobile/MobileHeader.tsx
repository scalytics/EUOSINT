import { useState } from "react";
import { Shield, ChevronDown, Check } from "lucide-react";

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
  clock: string;
}

export function MobileHeader({ regionFilter, onRegionChange, clock }: Props) {
  const [pickerOpen, setPickerOpen] = useState(false);

  function selectRegion(value: string) {
    onRegionChange(value);
    setPickerOpen(false);
  }

  const currentLabel = REGIONS.find((r) => r.value === regionFilter)?.label ?? "Global";

  return (
    <>
      <div className="mobile-header">
        <Shield size={18} className="text-sky-400 flex-shrink-0" />
        <span className="text-xs font-bold tracking-wide text-slate-300">EUOSINT</span>

        <button
          className="mobile-region-pill ml-auto"
          onClick={() => setPickerOpen(true)}
        >
          {currentLabel}
          <ChevronDown size={12} />
        </button>

        <span className="text-[10px] font-mono text-slate-500 tabular-nums">{clock}</span>
      </div>

      {pickerOpen && (
        <>
          <div
            className="mobile-sheet-backdrop"
            onClick={() => setPickerOpen(false)}
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
    </>
  );
}
