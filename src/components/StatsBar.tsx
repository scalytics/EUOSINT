/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import type { Alert } from "@/types/alert";
import type { SourceHealthDocument } from "@/types/source-health";
import { Activity, AlertTriangle, Globe2, ShieldAlert, Siren, Workflow } from "lucide-react";

interface Props {
  alerts: Alert[];
  sourceHealth: SourceHealthDocument | null;
}

export function StatsBar({ alerts, sourceHealth }: Props) {
  const metrics = [
    {
      icon: Activity,
      label: "Active",
      value: alerts.filter((alert) => alert.status === "active").length,
      tone: "text-emerald-300",
    },
    {
      icon: AlertTriangle,
      label: "Critical",
      value: alerts.filter((alert) => alert.severity === "critical").length,
      tone: "text-rose-300",
    },
    {
      icon: ShieldAlert,
      label: "High",
      value: alerts.filter((alert) => alert.severity === "high").length,
      tone: "text-amber-300",
    },
    {
      icon: Globe2,
      label: "Countries",
      value: new Set(alerts.map((alert) => alert.source.country_code)).size,
      tone: "text-siem-accent",
    },
    {
      icon: Workflow,
      label: "Feeds OK",
      value: sourceHealth?.sources_ok ?? 0,
      tone: "text-siem-text",
    },
    {
      icon: Siren,
      label: "Feed errors",
      value: sourceHealth?.sources_error ?? 0,
      tone: "text-siem-muted",
    },
  ];

  return (
    <div className="border-b border-siem-border bg-siem-panel/82 px-4 py-2.5 md:px-5">
      <div className="grid grid-cols-2 gap-2 md:grid-cols-6">
        {metrics.map((metric) => (
          <div
            key={metric.label}
            className="rounded-2xl border border-siem-border bg-siem-panel-strong px-3 py-2.5"
          >
            <div className="flex items-center justify-between">
              <metric.icon size={14} className={metric.tone} />
              <span className={`text-lg font-semibold ${metric.tone}`}>{metric.value}</span>
            </div>
            <div className="mt-2 text-[10px] uppercase tracking-[0.18em] text-siem-muted">
              {metric.label}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
