/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import type { Severity, AlertCategory } from "@/types/alert";

export const severityColors: Record<Severity, string> = {
  critical: "#d96464",
  high: "#d98958",
  medium: "#d1b35a",
  low: "#4eaf82",
  info: "#4fa0b1",
};

export const severityBg: Record<Severity, string> = {
  critical: "bg-[#d96464]/15 text-[#d96464] border-[#d96464]/35",
  high: "bg-[#d98958]/15 text-[#d98958] border-[#d98958]/35",
  medium: "bg-[#d1b35a]/15 text-[#d1b35a] border-[#d1b35a]/35",
  low: "bg-[#4eaf82]/15 text-[#4eaf82] border-[#4eaf82]/35",
  info: "bg-[#4fa0b1]/15 text-[#4fa0b1] border-[#4fa0b1]/35",
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
  terrorism_tip: "Terrorism Tip",
  fraud_alert: "Fraud Alert",
  public_safety: "Public Safety",
  private_sector: "Private Sector",
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
];

export const categoryBadge: Record<AlertCategory, string> = {
  informational: "bg-cyan-500/15 text-cyan-300 border-cyan-500/30",
  cyber_advisory: "bg-sky-500/15 text-sky-300 border-sky-500/30",
  education_digital_capacity: "bg-cyan-500/15 text-cyan-300 border-cyan-500/30",
  humanitarian_tasking: "bg-teal-500/15 text-teal-300 border-teal-500/30",
  conflict_monitoring: "bg-fuchsia-500/15 text-fuchsia-300 border-fuchsia-500/30",
  humanitarian_security: "bg-blue-500/15 text-blue-300 border-blue-500/30",
  wanted_suspect: "bg-rose-500/15 text-rose-300 border-rose-500/30",
  missing_person: "bg-amber-500/15 text-amber-300 border-amber-500/30",
  public_appeal: "bg-indigo-500/15 text-indigo-300 border-indigo-500/30",
  fraud_alert: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30",
  public_safety: "bg-violet-500/15 text-violet-300 border-violet-500/30",
  terrorism_tip: "bg-red-500/15 text-red-300 border-red-500/30",
  private_sector: "bg-orange-500/15 text-orange-300 border-orange-500/30",
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
};

export function freshnessLabel(hours: number): string {
  if (hours < 1) return "Just now";
  if (hours < 24) return `${Math.round(hours)}h ago`;
  if (hours < 168) return `${Math.round(hours / 24)}d ago`;
  return `${Math.round(hours / 168)}w ago`;
}
