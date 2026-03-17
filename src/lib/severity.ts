/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import type { Severity, AlertCategory } from "@/types/alert";
import { severityHex } from "@/lib/theme";

/**
 * Get the resolved hex colour for a severity level.
 * Reads from CSS custom properties at runtime — never hardcoded.
 */
export function severityColor(s: Severity): string {
  return severityHex(s);
}

/**
 * Tailwind class set for severity badge backgrounds.
 * References @theme tokens so colours update from a single source.
 */
export const severityBg: Record<Severity, string> = {
  critical: "bg-siem-critical/15 text-siem-critical border-siem-critical/35",
  high: "bg-siem-high/15 text-siem-high border-siem-high/35",
  medium: "bg-siem-medium/15 text-siem-medium border-siem-medium/35",
  low: "bg-siem-low/15 text-siem-low border-siem-low/35",
  info: "bg-siem-info/15 text-siem-info border-siem-info/35",
};

export const severityLabel: Record<Severity, string> = {
  critical: "critical",
  high: "high",
  medium: "medium",
  low: "low",
  info: "informational",
};

export const categoryLabels: Record<AlertCategory, string> = {
  informational: "INFORMATIONAL",
  missing_person: "Missing Person",
  wanted_suspect: "Wanted Suspect",
  public_appeal: "Public Appeal",
  cyber_advisory: "Cyber Advisory",
  education_digital_capacity: "Education & Digital Capacity",
  humanitarian_tasking: "Humanitarian Tasking",
  conflict_monitoring: "Conflict Monitoring",
  humanitarian_security: "Humanitarian Security",
  terrorism_tip: "Terrorism Activity",
  fraud_alert: "Fraud Alert",
  public_safety: "Public Safety",
  private_sector: "Private Sector",
  travel_warning: "Travel Warning",
  health_emergency: "Health Emergency",
  intelligence_report: "Intelligence Report",
  emergency_management: "Emergency Management",
  environmental_disaster: "Environmental Disaster",
  disease_outbreak: "Disease Outbreak",
  maritime_security: "Maritime Security",
  logistics_incident: "Logistics Incident",
  legislative: "Legislative",
};

export const categoryOrder: AlertCategory[] = [
  "informational",
  "humanitarian_tasking",
  "humanitarian_security",
  "conflict_monitoring",
  "education_digital_capacity",
  "cyber_advisory",
  "wanted_suspect",
  "missing_person",
  "public_appeal",
  "fraud_alert",
  "private_sector",
  "public_safety",
  "terrorism_tip",
  "travel_warning",
  "health_emergency",
  "intelligence_report",
  "emergency_management",
  "environmental_disaster",
  "disease_outbreak",
  "maritime_security",
  "logistics_incident",
  "legislative",
];

/**
 * Tailwind class set for category badge backgrounds.
 * References --color-cat-* tokens defined in @theme.
 */
export const categoryBadge: Record<AlertCategory, string> = {
  informational: "bg-cat-informational/15 text-cat-informational border-cat-informational/30",
  cyber_advisory: "bg-cat-cyber/15 text-cat-cyber border-cat-cyber/30",
  education_digital_capacity: "bg-cat-education/15 text-cat-education border-cat-education/30",
  humanitarian_tasking: "bg-cat-humanitarian/15 text-cat-humanitarian border-cat-humanitarian/30",
  conflict_monitoring: "bg-cat-conflict/15 text-cat-conflict border-cat-conflict/30",
  humanitarian_security: "bg-cat-humsec/15 text-cat-humsec border-cat-humsec/30",
  wanted_suspect: "bg-cat-wanted/15 text-cat-wanted border-cat-wanted/30",
  missing_person: "bg-cat-missing/15 text-cat-missing border-cat-missing/30",
  public_appeal: "bg-cat-appeal/15 text-cat-appeal border-cat-appeal/30",
  fraud_alert: "bg-cat-fraud/15 text-cat-fraud border-cat-fraud/30",
  public_safety: "bg-cat-safety/15 text-cat-safety border-cat-safety/30",
  terrorism_tip: "bg-cat-terrorism/15 text-cat-terrorism border-cat-terrorism/30",
  private_sector: "bg-cat-private/15 text-cat-private border-cat-private/30",
  travel_warning: "bg-cat-travel/15 text-cat-travel border-cat-travel/30",
  health_emergency: "bg-cat-health/15 text-cat-health border-cat-health/30",
  intelligence_report: "bg-cat-intel/15 text-cat-intel border-cat-intel/30",
  emergency_management: "bg-cat-emergency/15 text-cat-emergency border-cat-emergency/30",
  environmental_disaster: "bg-cat-environment/15 text-cat-environment border-cat-environment/30",
  disease_outbreak: "bg-cat-disease/15 text-cat-disease border-cat-disease/30",
  maritime_security: "bg-cat-maritime/15 text-cat-maritime border-cat-maritime/30",
  logistics_incident: "bg-cat-logistics/15 text-cat-logistics border-cat-logistics/30",
  legislative: "bg-cat-legislative/15 text-cat-legislative border-cat-legislative/30",
};

export const categoryIcons: Record<AlertCategory, string> = {
  informational: "Info",
  missing_person: "UserSearch",
  wanted_suspect: "ShieldAlert",
  public_appeal: "Megaphone",
  cyber_advisory: "ShieldCheck",
  education_digital_capacity: "GraduationCap",
  humanitarian_tasking: "MapPinned",
  conflict_monitoring: "Radar",
  humanitarian_security: "Shield",
  terrorism_tip: "AlertTriangle",
  fraud_alert: "BadgeDollarSign",
  public_safety: "Siren",
  private_sector: "Building",
  travel_warning: "Plane",
  health_emergency: "HeartPulse",
  intelligence_report: "Eye",
  emergency_management: "Siren",
  environmental_disaster: "CloudRain",
  disease_outbreak: "Bug",
  maritime_security: "Anchor",
  logistics_incident: "Ship",
  legislative: "Landmark",
};

export function freshnessLabel(hours: number): string {
  if (hours < 1) return "Just now";
  if (hours < 24) return `${Math.round(hours)}h ago`;
  if (hours < 168) return `${Math.round(hours / 24)}d ago`;
  return `${Math.round(hours / 168)}w ago`;
}
