/*
 * kafSIEM
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

export interface DuplicateSample {
  title: string;
  count: number;
}

export interface DuplicateAudit {
  suppressed_variant_duplicates: number;
  repeated_title_groups_in_active: number;
  repeated_title_samples: DuplicateSample[];
}

export interface SourceHealthEntry {
  source_id: string;
  authority_name: string;
  type: string;
  status: "ok" | "error" | "skipped" | "pending";
  fetched_count: number;
  feed_url: string;
  error?: string;
  started_at: string;
  finished_at: string;
  active_count?: number;
  filtered_count?: number;
}

export interface SourceHealthDocument {
  generated_at: string;
  critical_source_prefixes: string[];
  fail_on_critical_source_gap: boolean;
  total_sources: number;
  sources_ok: number;
  sources_error: number;
  duplicate_audit: DuplicateAudit;
  sources: SourceHealthEntry[];
}
