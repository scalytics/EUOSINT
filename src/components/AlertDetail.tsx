/*
 * kafSIEM
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
} from "lucide-react";

interface Props {
  alert: Alert | null;
  onClose: () => void;
}

export function AlertDetail({ alert, onClose }: Props) {
  if (!alert) return null;
  const subcategoryLabel = (alert.subcategory ?? "")
    .split("_")
    .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
    .join(" ")
    .trim();
  const eventCountry = alert.event_country || alert.source.country;
  const sourceCountry = alert.source.country;
  const geoSource = alert.event_geo_source || "registry";
  const geoConfidence = typeof alert.event_geo_confidence === "number" ? alert.event_geo_confidence : 0;
  const lowGeoConfidence = geoConfidence < 0.6;

  const corePlaybook = [
    "Extract core entities, locations, assets, and infrastructure.",
    "Map linked activity across nearby regions and related actors.",
    "Corroborate with independent sources; retain timestamps and source paths.",
    "Produce a working picture: what happened, what changed, what needs attention now.",
  ];

  const focusByCategory: Partial<Record<Alert["category"], string>> = {
    informational: "Focus: actionable signal only; discard commentary and stale context.",
    cyber_advisory: "Focus: key IOCs, likely intrusion path, and sector exposure.",
    public_appeal: "Focus: verified ask, jurisdiction, and immediate response pathway.",
    conflict_monitoring: "Focus: front-line movement, strike patterns, and logistics routes.",
    missing_person: "Focus: verified identity, last known movement, and official case references.",
    wanted_suspect: "Focus: confirmed identifiers, location confidence, and lawful reporting paths.",
    humanitarian_tasking: "Focus: access constraints, infrastructure status, and immediate aid gaps.",
    humanitarian_security: "Focus: civilian risk concentration, route safety, and service disruption.",
    education_digital_capacity: "Focus: target audience, operational need, and implementation constraints.",
    terrorism_tip: "Focus: threat credibility, target set, and near-term attack indicators.",
    fraud_alert: "Focus: fraud vector, impacted entities, and active campaign spread.",
    public_safety: "Focus: hazard radius, exposed population, and mitigation urgency.",
    private_sector: "Focus: operational exposure, supply-chain dependency, and business continuity risk.",
    travel_warning: "Focus: route viability, border posture, and traveler exposure points.",
    health_emergency: "Focus: transmission indicators, pressure points, and service continuity impact.",
    intelligence_report: "Focus: source reliability, corroboration depth, and decision relevance.",
    emergency_management: "Focus: incident command posture, resource gaps, and response tempo.",
    environmental_disaster: "Focus: impact footprint, infrastructure disruption, and secondary risk.",
    disease_outbreak: "Focus: spread trajectory, vulnerable clusters, and control effectiveness.",
    maritime_security: "Focus: vessel identity, corridor disruption, and port-chain impact.",
    logistics_incident: "Focus: chokepoints, reroute options, and cascading transport effects.",
    legislative: "Focus: enforcement timeline, affected operators, and compliance exposure.",
  };
  const categoryFocus = focusByCategory[alert.category] ?? "Focus: highest-confidence indicators and near-term escalation signals.";
  const signalLane = alert.signal_lane ?? (alert.severity === "info" ? "info" : "intel");

  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-3 border-b border-siem-border flex items-center justify-between">
        <h2 className="text-sm font-bold uppercase tracking-wider text-siem-muted">
          Alert Detail
        </h2>
        <button
          onClick={onClose}
          className="p-2 -mr-1 rounded-lg active:bg-siem-accent/12 active:text-siem-accent text-siem-muted transition-colors"
          style={{ WebkitTapHighlightColor: "transparent", minWidth: 44, minHeight: 44, display: "flex", alignItems: "center", justifyContent: "center" }}
        >
          <X size={18} />
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
              Event Country
            </div>
            <div className="flex items-center gap-1.5 text-sm">
              <MapPin size={12} className="text-siem-neutral" />
              {eventCountry}
            </div>
          </div>
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              Source Country
            </div>
            <div className="flex items-center gap-1.5 text-sm">
              <MapPin size={12} className="text-siem-neutral" />
              {sourceCountry}
            </div>
          </div>
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              Category
            </div>
            <div className="text-sm">{categoryLabels[alert.category]}</div>
            {subcategoryLabel && (
              <div className="mt-1 text-2xs uppercase tracking-wider text-siem-muted">
                {subcategoryLabel}
              </div>
            )}
          </div>
          <div className="bg-white/5 rounded-lg p-3 border border-siem-border">
            <div className="text-2xs uppercase tracking-wider text-siem-muted mb-1">
              Geo Confidence
            </div>
            <div className="text-sm">
              {Math.round(geoConfidence * 100)}% ({geoSource})
            </div>
            {lowGeoConfidence && (
              <div className="mt-1 text-2xs text-amber-300 uppercase tracking-wider">
                Source-country fallback
              </div>
            )}
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
            Scalytics Analyst Playbook
          </div>
          <ol className="space-y-1.5 text-xxs text-siem-text list-disc pl-4">
            {corePlaybook.map((step) => (
              <li key={step}>{step}</li>
            ))}
            <li key={categoryFocus}>{categoryFocus}</li>
          </ol>
          <div className="space-y-0.5 text-2xs text-siem-muted">
            <div>Primary geography: {eventCountry}</div>
            <div>Signal lane: {signalLane}</div>
            <div>Geo confidence: {Math.round(geoConfidence * 100)}%</div>
          </div>
          <p className="text-2xs text-siem-muted">
            Need help? We&apos;re here:{" "}
            <a
              href="https://scalytics.io/contact"
              target="_blank"
              rel="noopener noreferrer"
              className="text-siem-accent hover:text-siem-text"
            >
              scalytics.io/contact
            </a>
          </p>
        </div>

        {/* Go To Alert */}
        <a
          href={alert.canonical_url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center justify-center gap-2 w-full py-3.5 px-4 bg-siem-accent active:bg-siem-accent/80 text-white font-bold text-sm rounded-lg transition-colors"
          style={{ WebkitTapHighlightColor: "transparent", minHeight: 48, touchAction: "manipulation" }}
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
