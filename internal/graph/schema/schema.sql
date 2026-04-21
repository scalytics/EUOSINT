CREATE TABLE IF NOT EXISTS entities (
  id            TEXT PRIMARY KEY,
  type          TEXT NOT NULL,
  canonical_id  TEXT NOT NULL,
  display_name  TEXT,
  first_seen    TEXT NOT NULL,
  last_seen     TEXT NOT NULL,
  attrs_json    TEXT NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entities_type_canonical ON entities(type, canonical_id);
CREATE INDEX IF NOT EXISTS idx_entities_last_seen ON entities(last_seen DESC);

CREATE TABLE IF NOT EXISTS edges (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  src_id         TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  dst_id         TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  type           TEXT NOT NULL,
  valid_from     TEXT NOT NULL,
  valid_to       TEXT,
  evidence_msg   TEXT REFERENCES messages(record_id) ON DELETE SET NULL,
  weight         REAL NOT NULL DEFAULT 1.0,
  attrs_json     TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_edges_src_type ON edges(src_id, type, valid_from);
CREATE INDEX IF NOT EXISTS idx_edges_dst_type ON edges(dst_id, type, valid_from);
CREATE INDEX IF NOT EXISTS idx_edges_evidence ON edges(evidence_msg);
CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_dedupe
  ON edges(src_id, dst_id, type, valid_from, IFNULL(valid_to, ''), IFNULL(evidence_msg, ''));

CREATE TABLE IF NOT EXISTS provenance (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  subject_kind  TEXT NOT NULL,
  subject_id    TEXT NOT NULL,
  stage         TEXT NOT NULL,
  policy_ver    TEXT,
  inputs_json   TEXT,
  decision      TEXT,
  reasons_json  TEXT,
  produced_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_provenance_subject ON provenance(subject_kind, subject_id);

CREATE TABLE IF NOT EXISTS entity_geometry (
  entity_id      TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
  geometry_type  TEXT NOT NULL,
  geojson        TEXT NOT NULL,
  srid           INTEGER NOT NULL DEFAULT 4326,
  min_lat        REAL,
  min_lon        REAL,
  max_lat        REAL,
  max_lon        REAL,
  z_min          REAL,
  z_max          REAL,
  observed_at    TEXT NOT NULL,
  valid_to       TEXT
);
CREATE INDEX IF NOT EXISTS idx_entity_geometry_bbox
  ON entity_geometry(min_lat, min_lon, max_lat, max_lon);
