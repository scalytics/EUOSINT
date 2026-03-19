/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import type { Alert } from "@/types/alert";
import {
  severityBg,
  severityLabel,
  categoryLabels,
  freshnessLabel,
} from "@/lib/severity";
import {
  ExternalLink,
  Clock,
  Building2,
  MapPin,
  Radio,
  X,
  Phone,
  Mail,
  AlertTriangle,
} from "lucide-react";

interface Props {
  alert: Alert | null;
  onClose: () => void;
}

export function AlertDetail({ alert, onClose }: Props) {
  if (!alert) return null;
  const playbook =
    alert.category === "cyber_advisory"
      ? [
          "Pivot domains/IPs in passive DNS, WHOIS history, and certificate transparency.",
          "Search malware/hash indicators in public sandboxes and open threat intel feeds.",
          "Document overlaps with known campaigns and map affected sectors/regions.",
        ]
      : alert.category === "missing_person" || alert.category === "wanted_suspect"
      ? [
          "Extract names, aliases, locations, vehicles, and timeline references from the bulletin.",
          "Cross-check only with official/verified public posts and avoid reposting unverified claims.",
          "Package evidence links and report through official authority channels listed below.",
        ]
      : alert.category === "humanitarian_tasking" ||
        alert.category === "humanitarian_security" ||
        alert.category === "conflict_monitoring"
      ? [
          "Map incident location and nearby critical infrastructure using open geodata sources.",
          "Validate claims with multi-source corroboration (satellite, media, local official notices).",
          "Share structured findings with aid partners using minimal sensitive personal data.",
        ]
      : alert.category === "education_digital_capacity"
      ? [
          "Extract target geography, school/audience scope, skills requested, and deadline/status.",
          "Validate that the opportunity is active and identify official contact/onboarding channels.",
          "Prepare a scoped contribution plan (training, cyber hygiene, tooling, mentorship) and report via official path.",
        ]
      : [
          "Collect key entities (people, places, orgs, infrastructure) from the bulletin.",
          "Corroborate across independent public sources and time-stamp your evidence.",
          "Submit concise findings via the official reporting path when relevant.",
        ];

  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-3 border-b border-siem-border flex items-center justify-between">
        <h2 className="text-sm font-bold uppercase tracking-wider text-siem-muted">
          Alert Detail
        </h2>
        <button
          onClick={onClose}
          className="p-1 rounded hover:bg-siem-accent/12 hover:text-siem-accent text-siem-muted transition-colors"
        >
          <X size={14} />
        </button>
      </div>
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Severity + Status */}
        <div className="flex items-center gap-2">
          <span
            className={`inline-flex items-center px-2.5 py-1 text-xs font-bold uppercase tracking-wider rounded border ${
              severityBg[alert.severity]
            }`}
          >
            {severityLabel[alert.severity]}
          </span>
          <span className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded bg-white/5 text-siem-muted border border-siem-border">
            <Radio size={10} />
            {alert.status.toUpperCase()}
          </span>
        </div>

        {/* Title */}
        <h3 className="text-base font-semibold leading-snug text-siem-text">
          {alert.title}
        </h3>

        {/* Metadata Grid */}
        <div className="grid grid-cols-2 gap-3">
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              Authority
            </div>
            <div className="flex items-center gap-1.5 text-sm">
              <Building2 size={12} className="text-siem-neutral" />
              {alert.source.authority_name}
            </div>
          </div>
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              Country
            </div>
            <div className="flex items-center gap-1.5 text-sm">
              <MapPin size={12} className="text-siem-neutral" />
              {alert.event_country || alert.source.country}
            </div>
          </div>
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              Category
            </div>
            <div className="text-sm">{categoryLabels[alert.category]}</div>
          </div>
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              First Seen
            </div>
            <div className="flex items-center gap-1.5 text-sm">
              <Clock size={12} className="text-siem-neutral" />
              {freshnessLabel(alert.freshness_hours)}
            </div>
          </div>
        </div>

        {/* Authority Type */}
        <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
          <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
            Authority Type
          </div>
          <div className="text-sm capitalize">
            {alert.source.authority_type.replace("_", " ")}
          </div>
        </div>

        {/* Notice Age */}
        <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
          <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
            Notice Age
          </div>
          <div className="text-sm font-mono font-bold text-siem-text">
            {freshnessLabel(alert.freshness_hours)}
          </div>
          <div className="text-xxs text-siem-muted mt-1">
            Last confirmed: {freshnessLabel(
              (Date.now() - new Date(alert.last_seen).getTime()) / 3600000
            )}
          </div>
        </div>

        {alert.triage && (
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border space-y-2">
            <div className="text-2xs uppercase tracking-wider text-siem-muted">
              Relevance Scoring
            </div>
            <div className="flex items-center justify-between gap-2">
              <div className="text-sm font-mono font-bold text-siem-text">
                {Math.round(alert.triage.relevance_score * 100)} / 100
              </div>
              <div className="text-2xs uppercase tracking-wider text-siem-muted">
                {alert.triage.confidence} confidence
              </div>
            </div>
            <div className="h-2 rounded bg-white/10 overflow-hidden">
              <div
                className="h-full bg-siem-accent transition-all"
                style={{ width: `${Math.max(0, Math.min(100, alert.triage.relevance_score * 100))}%` }}
              />
            </div>
            <div className="text-xxs text-siem-muted">
              Threshold: {Math.round(alert.triage.threshold * 100)} | Disposition:{" "}
              {alert.triage.disposition.replace("_", " ")}
            </div>
          </div>
        )}

        <div className="rounded-lg border border-siem-border bg-white/5 p-3 space-y-2">
          <div className="text-2xs uppercase tracking-wider text-siem-muted">
            How To Help (OSINT Playbook)
          </div>
          <ol className="space-y-1.5 text-xxs text-siem-text list-decimal pl-4">
            {playbook.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ol>
          <p className="text-2xs text-siem-muted">
            Do not contact suspects or victims directly. Use only official channels.
          </p>
        </div>

        {/* Go To Alert */}
        <a
          href={alert.canonical_url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center justify-center gap-2 w-full py-3 px-4 bg-siem-accent hover:bg-siem-accent/80 text-white font-bold text-sm rounded-lg transition-colors"
        >
          <ExternalLink size={16} />
          GO TO OFFICIAL ALERT
        </a>

        <p className="text-2xs text-siem-muted text-center leading-relaxed">
          This link opens the official authority bulletin.
          <br />
          No content is stored on this platform.
        </p>

      </div>
    </div>
  );
}
