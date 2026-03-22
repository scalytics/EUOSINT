import { useRef, useMemo } from "react";
import type { Alert, Severity } from "@/types/alert";
import { MobileAlertCard } from "./MobileAlertCard";
import { usePullToRefresh } from "./usePullToRefresh";

const SEVERITY_FILTERS: Array<{ label: string; value: Severity | "all" }> = [
  { label: "All", value: "all" },
  { label: "Critical", value: "critical" },
  { label: "High", value: "high" },
  { label: "Medium", value: "medium" },
  { label: "Low", value: "low" },
];

interface Props {
  alerts: Alert[];
  isLoading: boolean;
  severityFilter: Severity | "all";
  onSeverityChange: (s: Severity | "all") => void;
  onSelectAlert: (alertId: string) => void;
  onRefresh: () => void;
}

export function MobileAlertList({
  alerts,
  isLoading,
  severityFilter,
  onSeverityChange,
  onSelectAlert,
  onRefresh,
}: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const { isRefreshing, pullDistance, onTouchStart, onTouchMove, onTouchEnd } =
    usePullToRefresh(scrollRef, onRefresh);

  const filtered = useMemo(
    () =>
      severityFilter === "all"
        ? alerts
        : alerts.filter((a) => a.severity === severityFilter),
    [alerts, severityFilter],
  );

  const sevCounts = useMemo(() => {
    const c: Record<string, number> = { all: alerts.length };
    for (const a of alerts) c[a.severity] = (c[a.severity] ?? 0) + 1;
    return c;
  }, [alerts]);

  if (isLoading && alerts.length === 0) {
    return (
      <div className="mobile-alert-list">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className="mobile-skeleton" style={{ marginTop: i === 0 ? 12 : 8 }} />
        ))}
      </div>
    );
  }

  return (
    <div
      ref={scrollRef}
      className="mobile-alert-list"
      onTouchStart={onTouchStart}
      onTouchMove={onTouchMove}
      onTouchEnd={onTouchEnd}
    >
      {/* Pull-to-refresh indicator */}
      {(pullDistance > 0 || isRefreshing) && (
        <div
          className="mobile-ptr-indicator"
          style={{ opacity: isRefreshing ? 1 : Math.min(1, pullDistance / 60) }}
        >
          <div className="mobile-ptr-spinner" />
        </div>
      )}

      {/* Severity filter pills */}
      <div className="mobile-severity-pills">
        {SEVERITY_FILTERS.map(({ label, value }) => (
          <button
            key={value}
            className={`mobile-severity-pill ${severityFilter === value ? "active" : ""}`}
            onClick={() => onSeverityChange(value)}
          >
            {label}
            {(sevCounts[value] ?? 0) > 0 && (
              <span className="ml-1 opacity-60">{sevCounts[value]}</span>
            )}
          </button>
        ))}
      </div>

      {/* Alert cards */}
      {filtered.length === 0 ? (
        <div className="mobile-empty">
          <span className="text-2xl">No alerts</span>
          <span>
            {severityFilter !== "all"
              ? `No ${severityFilter} alerts in this region`
              : "Pull down to refresh"}
          </span>
        </div>
      ) : (
        filtered.map((alert) => (
          <MobileAlertCard
            key={alert.alert_id}
            alert={alert}
            onSelect={onSelectAlert}
          />
        ))
      )}
    </div>
  );
}
