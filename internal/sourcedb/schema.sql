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
  severity TEXT NOT NULL,
  region_tag TEXT NOT NULL,
  lat REAL NOT NULL DEFAULT 0,
  lng REAL NOT NULL DEFAULT 0,
  freshness_hours INTEGER NOT NULL DEFAULT 0,
  source_json TEXT NOT NULL,
  reporting_json TEXT NOT NULL DEFAULT '{}',
  triage_json TEXT NOT NULL DEFAULT 'null'
);

CREATE INDEX IF NOT EXISTS idx_sources_agency_id ON sources(agency_id);
CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status);
CREATE INDEX IF NOT EXISTS idx_sources_feed_url ON sources(feed_url);
CREATE INDEX IF NOT EXISTS idx_source_checks_source_id_checked_at ON source_checks(source_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_candidates_status ON source_candidates(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_candidates_discovered_url ON source_candidates(discovered_url);
CREATE INDEX IF NOT EXISTS idx_source_term_overrides_source_id ON source_term_overrides(source_id);
CREATE INDEX IF NOT EXISTS idx_agency_category_coverage_category ON agency_category_coverage(category);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_source_id ON alerts(source_id);

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
