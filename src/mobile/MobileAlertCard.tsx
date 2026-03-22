import type { Alert } from "@/types/alert";
import { severityColor } from "@/lib/severity";
import { freshnessLabel, categoryLabels } from "@/lib/severity";

interface Props {
  alert: Alert;
  onSelect: (alertId: string) => void;
}

const severityBadgeColor: Record<string, string> = {
  critical: "#dc2626",
  high: "#ef4444",
  medium: "#f59e0b",
  low: "#3b82f6",
  info: "#64748b",
};

export function MobileAlertCard({ alert, onSelect }: Props) {
  const sevColor = severityColor(alert.severity);
  const badgeBg = severityBadgeColor[alert.severity] ?? "#64748b";

  return (
    <div className="mobile-alert-card" onClick={() => onSelect(alert.alert_id)}>
      <div className="mobile-alert-sev" style={{ background: sevColor }} />
      <div className="mobile-alert-body">
        <div className="mobile-alert-title">{alert.title}</div>
        <div className="mobile-alert-meta">
          <span
            className="mobile-alert-badge"
            style={{ background: badgeBg }}
          >
            {alert.severity}
          </span>
          <span>{categoryLabels[alert.category] ?? alert.category}</span>
          <span>&middot;</span>
          <span>{freshnessLabel(alert.freshness_hours)}</span>
          <span>&middot;</span>
          <span className="truncate">{alert.source.authority_name}</span>
        </div>
      </div>
    </div>
  );
}
