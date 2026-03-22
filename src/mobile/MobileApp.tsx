import { useState, useEffect, useMemo, lazy, Suspense, useCallback } from "react";
import { useAlerts } from "@/hooks/useAlerts";
import { alertMatchesRegionFilter } from "@/lib/regions";
import type { Alert, Severity } from "@/types/alert";
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
  const [selectedAlertId, setSelectedAlertId] = useState<string | null>(null);
  const clock = useUTCClock();

  // Filter alerts by region
  const regionAlerts = useMemo(
    () => alerts.filter((a) => alertMatchesRegionFilter(a, regionFilter)),
    [alerts, regionFilter],
  );

  // Sort: most recent first
  const sorted = useMemo(
    () =>
      [...regionAlerts].sort(
        (a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime(),
      ),
    [regionAlerts],
  );

  // Count critical+high for badge
  const urgentCount = useMemo(
    () => regionAlerts.filter((a) => a.severity === "critical" || a.severity === "high").length,
    [regionAlerts],
  );

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
