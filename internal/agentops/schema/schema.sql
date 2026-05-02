CREATE TABLE IF NOT EXISTS messages (
  record_id            TEXT PRIMARY KEY,
  topic                TEXT NOT NULL,
  topic_family         TEXT NOT NULL,
  partition            INTEGER NOT NULL,
  offset               INTEGER NOT NULL,
  timestamp            TEXT NOT NULL,
  envelope_type        TEXT,
  sender_id            TEXT,
  correlation_id       TEXT,
  trace_id             TEXT,
  task_id              TEXT,
  parent_task_id       TEXT,
  status               TEXT,
  preview              TEXT,
  content              TEXT,
  lfs_bucket           TEXT,
  lfs_key              TEXT,
  lfs_size             INTEGER,
  lfs_sha256           TEXT,
  lfs_content_type     TEXT,
  lfs_created_at       TEXT,
  lfs_proxy_id         TEXT,
  outcome              TEXT NOT NULL,
  reject_reason        TEXT
);
CREATE INDEX IF NOT EXISTS idx_messages_corr ON messages(correlation_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_messages_trace ON messages(trace_id);
CREATE INDEX IF NOT EXISTS idx_messages_task ON messages(task_id);
CREATE INDEX IF NOT EXISTS idx_messages_family_ts ON messages(topic_family, timestamp);

CREATE TABLE IF NOT EXISTS flows (
  id                   TEXT PRIMARY KEY,
  first_seen           TEXT NOT NULL,
  last_seen            TEXT NOT NULL,
  message_count        INTEGER NOT NULL DEFAULT 0,
  latest_status        TEXT,
  latest_preview       TEXT
);
CREATE INDEX IF NOT EXISTS idx_flows_last_seen ON flows(last_seen DESC);

CREATE TABLE IF NOT EXISTS flow_participants (
  flow_id              TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  kind                 TEXT NOT NULL,
  value                TEXT NOT NULL,
  PRIMARY KEY (flow_id, kind, value)
);

CREATE TABLE IF NOT EXISTS traces (
  id                   TEXT PRIMARY KEY,
  span_count           INTEGER NOT NULL DEFAULT 0,
  latest_title         TEXT,
  started_at           TEXT,
  ended_at             TEXT,
  duration_ms          INTEGER
);

CREATE TABLE IF NOT EXISTS trace_agents (
  trace_id             TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  agent_id             TEXT NOT NULL,
  PRIMARY KEY (trace_id, agent_id)
);

CREATE TABLE IF NOT EXISTS trace_span_types (
  trace_id             TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  span_type            TEXT NOT NULL,
  PRIMARY KEY (trace_id, span_type)
);

CREATE TABLE IF NOT EXISTS tasks (
  id                   TEXT PRIMARY KEY,
  parent_task_id       TEXT,
  delegation_depth     INTEGER NOT NULL DEFAULT 0,
  requester_id         TEXT,
  responder_id         TEXT,
  original_requester_id TEXT,
  status               TEXT,
  description          TEXT,
  last_summary         TEXT,
  first_seen           TEXT NOT NULL,
  last_seen            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_task_id);

CREATE TABLE IF NOT EXISTS topic_stats (
  topic                TEXT PRIMARY KEY,
  message_count        INTEGER NOT NULL DEFAULT 0,
  active_agents        INTEGER NOT NULL DEFAULT 0,
  first_message_at     TEXT,
  last_message_at      TEXT
);

CREATE TABLE IF NOT EXISTS topic_agents (
  topic                TEXT NOT NULL REFERENCES topic_stats(topic) ON DELETE CASCADE,
  agent_id             TEXT NOT NULL,
  PRIMARY KEY (topic, agent_id)
);

CREATE TABLE IF NOT EXISTS replay_sessions (
  id                   TEXT PRIMARY KEY,
  group_id             TEXT NOT NULL,
  status               TEXT NOT NULL,
  started_at           TEXT NOT NULL,
  finished_at          TEXT,
  message_count        INTEGER NOT NULL DEFAULT 0,
  topics_json          TEXT,
  last_error           TEXT
);

CREATE TABLE IF NOT EXISTS replay_requests (
  id                   TEXT PRIMARY KEY,
  requested_at         TEXT NOT NULL,
  status               TEXT NOT NULL DEFAULT 'pending',
  topics_json          TEXT,
  started_session_id   TEXT,
  last_error           TEXT
);
CREATE INDEX IF NOT EXISTS idx_replay_requests_status_requested_at
  ON replay_requests(status, requested_at);

CREATE TABLE IF NOT EXISTS health_snapshots (
  taken_at               TEXT PRIMARY KEY,
  connected              INTEGER NOT NULL,
  group_id               TEXT,
  accepted_count         INTEGER NOT NULL DEFAULT 0,
  rejected_count         INTEGER NOT NULL DEFAULT 0,
  mirrored_count         INTEGER NOT NULL DEFAULT 0,
  mirror_failed_count    INTEGER NOT NULL DEFAULT 0,
  last_reject            TEXT,
  last_mirror_error      TEXT,
  last_poll_at           TEXT,
  replay_status          TEXT,
  replay_active          INTEGER NOT NULL DEFAULT 0,
  replay_last_error      TEXT,
  replay_last_finished   TEXT,
  replay_last_count      INTEGER NOT NULL DEFAULT 0,
  rejected_by_reason_json TEXT
);
