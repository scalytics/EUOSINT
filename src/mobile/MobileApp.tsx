import { useState, useEffect, useMemo, lazy, Suspense, useCallback } from "react";
import { useAlerts } from "@/hooks/useAlerts";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { categoryLabels, categoryOrder } from "@/lib/severity";
import type { Alert, Severity, AlertCategory } from "@/types/alert";
import { MobileHeader } from "./MobileHeader";
import { MobileBottomNav, type MobileTab } from "./MobileBottomNav";
import { MobileAlertList } from "./MobileAlertList";
import { MobileAlertSheet } from "./MobileAlertSheet";
import { MobileSearch } from "./MobileSearch";

const MobileMapView = lazy(() =>
  import("./MobileMapView").then((m) => ({ default: m.MobileMapView })),
);

function useUTCClock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 10_000);
    return () => clearInterval(id);
  }, []);
  return `${String(now.getUTCHours()).padStart(2, "0")}:${String(now.getUTCMinutes()).padStart(2, "0")}Z`;
}

export function MobileApp() {
  const { alerts, isLoading, refetch } = useAlerts();
  const [regionFilter, setRegionFilter] = useState("Europe");
  const [activeTab, setActiveTab] = useState<MobileTab>("alerts");
  const [severityFilter, setSeverityFilter] = useState<Severity | "all">("all");
  const [categoryFilter, setCategoryFilter] = useState<Set<AlertCategory>>(new Set());
  const [selectedAlertId, setSelectedAlertId] = useState<string | null>(null);
  const clock = useUTCClock();

  // Filter alerts by region
  const regionAlerts = useMemo(
    () => alerts.filter((a) => alertMatchesRegionFilter(a, regionFilter)),
    [alerts, regionFilter],
  );

  // Apply category filter
  const categoryFiltered = useMemo(
    () =>
      categoryFilter.size === 0
        ? regionAlerts
        : regionAlerts.filter((a) => categoryFilter.has(a.category)),
    [regionAlerts, categoryFilter],
  );

  // Sort: most recent first
  const sorted = useMemo(
    () =>
      [...categoryFiltered].sort(
        (a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime(),
      ),
    [categoryFiltered],
  );

  // Count critical+high for badge
  const urgentCount = useMemo(
    () => categoryFiltered.filter((a) => a.severity === "critical" || a.severity === "high").length,
    [categoryFiltered],
  );

  // Categories with counts for the picker (based on region alerts, before category filter)
  const categoriesWithCounts = useMemo(() => {
    const countMap: Record<string, number> = {};
    for (const a of regionAlerts) countMap[a.category] = (countMap[a.category] ?? 0) + 1;
    return categoryOrder
      .filter((c) => countMap[c])
      .map((c) => ({ value: c, label: categoryLabels[c] ?? c, count: countMap[c] }));
  }, [regionAlerts]);

  // Find selected alert across all alerts (search results may not be in regionAlerts)
  const selectedAlert: Alert | null = useMemo(
    () => alerts.find((a) => a.alert_id === selectedAlertId) ?? null,
    [alerts, selectedAlertId],
  );

  const handleSelectAlert = useCallback((alertId: string) => {
    setSelectedAlertId(alertId);
  }, []);

  const handleCloseSheet = useCallback(() => {
    setSelectedAlertId(null);
  }, []);

  return (
    <div className="mobile-shell">
      <MobileHeader
        regionFilter={regionFilter}
        onRegionChange={setRegionFilter}
        categoryFilter={categoryFilter}
        onCategoryChange={setCategoryFilter}
        categoriesWithCounts={categoriesWithCounts}
        clock={clock}
      />

      <div className="mobile-content">
        {activeTab === "alerts" && (
          <MobileAlertList
            alerts={sorted}
            isLoading={isLoading}
            severityFilter={severityFilter}
            onSeverityChange={setSeverityFilter}
            onSelectAlert={handleSelectAlert}
            onRefresh={refetch}
          />
        )}

        {activeTab === "map" && (
          <Suspense
            fallback={
              <div className="mobile-empty">
                <div className="mobile-ptr-spinner" />
                <span>Loading map...</span>
              </div>
            }
          >
            <MobileMapView
              alerts={sorted}
              regionFilter={regionFilter}
              onSelectAlert={handleSelectAlert}
            />
          </Suspense>
        )}

        {activeTab === "search" && (
          <MobileSearch onSelectAlert={handleSelectAlert} />
        )}
      </div>

      <MobileBottomNav
        activeTab={activeTab}
        onTabChange={setActiveTab}
        alertCount={urgentCount}
      />

      <MobileAlertSheet alert={selectedAlert} onClose={handleCloseSheet} />
    </div>
  );
}
