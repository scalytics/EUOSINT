/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

export type Severity = "critical" | "high" | "medium" | "low" | "info";
export type AlertStatus = "active" | "updated" | "removed" | "filtered";
export type SignalLane = "alarm" | "intel" | "info";
export type AlertCategory =
  | "informational"
  | "missing_person"
  | "wanted_suspect"
  | "public_appeal"
  | "cyber_advisory"
  | "education_digital_capacity"
  | "humanitarian_tasking"
  | "conflict_monitoring"
  | "humanitarian_security"
  | "terrorism_tip"
  | "fraud_alert"
  | "public_safety"
  | "private_sector"
  | "travel_warning"
  | "health_emergency"
  | "intelligence_report"
  | "emergency_management"
  | "environmental_disaster"
  | "disease_outbreak"
  | "maritime_security"
  | "logistics_incident"
  | "legislative";
export type AuthorityType =
  | "police"
  | "national_security"
  | "intelligence"
  | "regulatory"
  | "public_safety_program"
  | "cert"
  | "private_sector"
  | "osint";

export interface AuthoritySource {
  source_id: string;
  authority_name: string;
  country: string;
  country_code: string;
  region: string;
  authority_type: AuthorityType;
  base_url: string;
}

export interface Alert {
  alert_id: string;
  source_id: string;
  source: AuthoritySource;
  title: string;
  canonical_url: string;
  first_seen: string;
  last_seen: string;
  status: AlertStatus;
  category: AlertCategory;
  severity: Severity;
  signal_lane?: SignalLane;
  region_tag: string;
  lat: number;
  lng: number;
  event_country?: string;
  event_country_code?: string;
  event_geo_source?: string;
  event_geo_confidence?: number;
  freshness_hours: number;
  reporting?: ReportingInfo;
  triage?: AlertTriage;
}

export interface ReportingInfo {
  label: string;
  url?: string;
  phone?: string;
  email?: string;
  notes?: string;
}

export interface AlertTriage {
  relevance_score: number;
  threshold: number;
  confidence: "high" | "medium" | "low";
  disposition: "retained" | "filtered_review";
  publication_type?: string;
  weak_signals?: string[];
  metadata?: {
    author?: string;
    tags?: string[];
  };
}
