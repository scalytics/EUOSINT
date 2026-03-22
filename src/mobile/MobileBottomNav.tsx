import { Bell, Map, Search } from "lucide-react";

export type MobileTab = "alerts" | "map" | "search";

interface Props {
  activeTab: MobileTab;
  onTabChange: (tab: MobileTab) => void;
  alertCount: number;
}

export function MobileBottomNav({ activeTab, onTabChange, alertCount }: Props) {
  return (
    <nav className="mobile-nav">
      <button
        className={`mobile-nav-tab ${activeTab === "alerts" ? "active" : ""}`}
        onClick={() => onTabChange("alerts")}
      >
        <div className="relative">
          <Bell size={22} />
          {alertCount > 0 && (
            <span className="absolute -top-1.5 -right-2.5 min-w-[18px] h-[18px] flex items-center justify-center px-1 text-[10px] font-bold bg-red-500 text-white rounded-full">
              {alertCount > 99 ? "99+" : alertCount}
            </span>
          )}
        </div>
        <span>Alerts</span>
      </button>

      <button
        className={`mobile-nav-tab ${activeTab === "map" ? "active" : ""}`}
        onClick={() => onTabChange("map")}
      >
        <Map size={22} />
        <span>Map</span>
      </button>

      <button
        className={`mobile-nav-tab ${activeTab === "search" ? "active" : ""}`}
        onClick={() => onTabChange("search")}
      >
        <Search size={22} />
        <span>Search</span>
      </button>
    </nav>
  );
}
