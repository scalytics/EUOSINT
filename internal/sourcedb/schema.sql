CREATE TABLE IF NOT EXISTS agencies (
  id TEXT PRIMARY KEY,
  authority_name TEXT NOT NULL,
  language_code TEXT NOT NULL DEFAULT '',
  country TEXT NOT NULL,
  country_code TEXT NOT NULL,
  region TEXT NOT NULL,
  authority_type TEXT NOT NULL,
  base_url TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT 'national',
  level TEXT NOT NULL DEFAULT 'national',
  parent_agency_id TEXT NOT NULL DEFAULT '',
  jurisdiction_name TEXT NOT NULL DEFAULT '',
  mission_tags_json TEXT NOT NULL DEFAULT '[]',
  operational_relevance REAL NOT NULL DEFAULT 0,
  is_curated INTEGER NOT NULL DEFAULT 0,
  is_high_value INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agency_aliases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agency_id TEXT NOT NULL,
  alias TEXT NOT NULL,
  alias_type TEXT NOT NULL DEFAULT 'short_name',
  UNIQUE(agency_id, alias),
  FOREIGN KEY (agency_id) REFERENCES agencies(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sources (
  id TEXT PRIMARY KEY,
  agency_id TEXT NOT NULL,
  language_code TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  fetch_mode TEXT NOT NULL DEFAULT '',
  follow_redirects INTEGER NOT NULL DEFAULT 0,
  feed_url TEXT NOT NULL,
  feed_urls_json TEXT NOT NULL DEFAULT '[]',
  category TEXT NOT NULL,
  region_tag TEXT NOT NULL DEFAULT '',
  lat REAL NOT NULL DEFAULT 0,
  lng REAL NOT NULL DEFAULT 0,
  max_items INTEGER NOT NULL DEFAULT 0,
  include_keywords_json TEXT NOT NULL DEFAULT '[]',
  exclude_keywords_json TEXT NOT NULL DEFAULT '[]',
  source_quality REAL NOT NULL DEFAULT 0,
  promotion_status TEXT NOT NULL DEFAULT 'candidate',
  rejection_reason TEXT NOT NULL DEFAULT '',
  is_mirror INTEGER NOT NULL DEFAULT 0,
  preferred_source_rank INTEGER NOT NULL DEFAULT 0,
  reporting_label TEXT NOT NULL DEFAULT '',
  reporting_url TEXT NOT NULL DEFAULT '',
  reporting_phone TEXT NOT NULL DEFAULT '',
  reporting_notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  last_http_status INTEGER,
  last_ok_at TEXT,
  last_error TEXT,
  last_error_class TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (agency_id) REFERENCES agencies(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_categories (
  source_id TEXT NOT NULL,
  category TEXT NOT NULL,
  PRIMARY KEY (source_id, category),
  FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agency_category_coverage (
  agency_id TEXT NOT NULL,
  category TEXT NOT NULL,
  PRIMARY KEY (agency_id, category),
  FOREIGN KEY (agency_id) REFERENCES agencies(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_checks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id TEXT NOT NULL,
  checked_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  http_status INTEGER,
  final_url TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT '',
  ok INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  error_class TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id TEXT NOT NULL,
  run_started_at TEXT NOT NULL,
  run_finished_at TEXT NOT NULL,
  status TEXT NOT NULL,
  http_status INTEGER,
  fetched_count INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  error_class TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  etag TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_watermarks (
  source_id TEXT PRIMARY KEY,
  last_run_started_at TEXT NOT NULL DEFAULT '',
  last_run_finished_at TEXT NOT NULL DEFAULT '',
  last_status TEXT NOT NULL DEFAULT '',
  last_http_status INTEGER,
  last_fetched_count INTEGER NOT NULL DEFAULT 0,
  last_content_hash TEXT NOT NULL DEFAULT '',
  last_etag TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL DEFAULT '',
  last_success_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_candidates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agency_id TEXT,
  discovered_url TEXT NOT NULL,
  discovered_via TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'candidate',
  language_code TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  authority_type TEXT NOT NULL DEFAULT '',
  country TEXT NOT NULL DEFAULT '',
  country_code TEXT NOT NULL DEFAULT '',
  checked_at TEXT,
  notes TEXT NOT NULL DEFAULT '',
  UNIQUE(discovered_url),
  FOREIGN KEY (agency_id) REFERENCES agencies(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS source_term_overrides (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id TEXT NOT NULL,
  category TEXT NOT NULL,
  language_code TEXT NOT NULL DEFAULT '',
  term TEXT NOT NULL,
  term_type TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_id, category, language_code, term, term_type),
  FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS alerts (
  alert_id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  status TEXT NOT NULL,
  first_seen TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  title TEXT NOT NULL,
  canonical_url TEXT NOT NULL,
  category TEXT NOT NULL,
  subcategory TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL,
  signal_lane TEXT NOT NULL DEFAULT 'intel',
  region_tag TEXT NOT NULL,
  lat REAL NOT NULL DEFAULT 0,
  lng REAL NOT NULL DEFAULT 0,
  event_country TEXT NOT NULL DEFAULT '',
  event_country_code TEXT NOT NULL DEFAULT '',
  event_geo_source TEXT NOT NULL DEFAULT '',
  event_geo_confidence REAL NOT NULL DEFAULT 0,
  freshness_hours INTEGER NOT NULL DEFAULT 0,
  source_json TEXT NOT NULL,
  reporting_json TEXT NOT NULL DEFAULT '{}',
  triage_json TEXT NOT NULL DEFAULT 'null'
);

CREATE TABLE IF NOT EXISTS noise_feedback (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  alert_id TEXT NOT NULL,
  source_id TEXT NOT NULL DEFAULT '',
  verdict TEXT NOT NULL,
  analyst TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS registry_sync_state (
  sync_key TEXT PRIMARY KEY,
  last_hash TEXT NOT NULL DEFAULT '',
  last_synced_at TEXT NOT NULL DEFAULT '',
  source_count INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS zone_briefings_cache (
  lens_id TEXT PRIMARY KEY,
  payload_json TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sources_agency_id ON sources(agency_id);
CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status);
CREATE INDEX IF NOT EXISTS idx_sources_feed_url ON sources(feed_url);
CREATE INDEX IF NOT EXISTS idx_source_checks_source_id_checked_at ON source_checks(source_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_runs_source_id_started_at ON source_runs(source_id, run_started_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_candidates_status ON source_candidates(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_candidates_discovered_url ON source_candidates(discovered_url);
CREATE INDEX IF NOT EXISTS idx_source_term_overrides_source_id ON source_term_overrides(source_id);
CREATE INDEX IF NOT EXISTS idx_agency_category_coverage_category ON agency_category_coverage(category);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_source_id ON alerts(source_id);
CREATE INDEX IF NOT EXISTS idx_noise_feedback_alert ON noise_feedback(alert_id);
CREATE INDEX IF NOT EXISTS idx_noise_feedback_source ON noise_feedback(source_id);
CREATE INDEX IF NOT EXISTS idx_noise_feedback_verdict ON noise_feedback(verdict);
CREATE INDEX IF NOT EXISTS idx_registry_sync_state_updated ON registry_sync_state(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_zone_briefings_cache_expires ON zone_briefings_cache(expires_at);

CREATE TABLE IF NOT EXISTS cities (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  name_lower TEXT NOT NULL,
  ascii_name TEXT NOT NULL,
  ascii_lower TEXT NOT NULL,
  country_code TEXT NOT NULL,
  lat REAL NOT NULL,
  lng REAL NOT NULL,
  population INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_cities_name_lower ON cities(name_lower);
CREATE INDEX IF NOT EXISTS idx_cities_ascii_lower ON cities(ascii_lower);
CREATE INDEX IF NOT EXISTS idx_cities_country_code ON cities(country_code);

CREATE VIRTUAL TABLE IF NOT EXISTS agencies_fts USING fts5(
  agency_id UNINDEXED,
  authority_name,
  aliases,
  country,
  country_code,
  region,
  authority_type,
  base_url
);

CREATE VIRTUAL TABLE IF NOT EXISTS alerts_fts USING fts5(
  alert_id UNINDEXED,
  title,
  canonical_url UNINDEXED,
  category,
  severity UNINDEXED,
  region_tag,
  source_authority,
  source_country,
  source_country_code
);
