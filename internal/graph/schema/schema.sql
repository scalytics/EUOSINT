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
