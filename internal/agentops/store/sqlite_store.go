// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scalytics/kafSIEM/internal/agentops/schema"
	graphschema "github.com/scalytics/kafSIEM/internal/graph/schema"
)

type SqliteStore struct {
	db  *sql.DB
	mu  sync.Mutex
	doc Snapshot
}

func NewSqliteStore(path string, initial Snapshot) (*SqliteStore, error) {
	dbPath := sqlitePath(path)
	db, err := schema.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := graphschema.Apply(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	fs := &SqliteStore{
		db:  db,
		doc: cloneDocument(initial),
	}

	empty, err := fs.isEmpty()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if empty && hasDocumentData(initial) {
		if err := fs.replaceWithDocument(cloneDocument(initial)); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	if err := fs.reload(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return fs, nil
}

func (s *SqliteStore) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneDocument(s.doc)
}

func (s *SqliteStore) Update(apply func(*Snapshot)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc := cloneDocument(s.doc)
	apply(&doc)
	doc.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	if err := s.replaceWithDocument(doc); err != nil {
		return err
	}
	s.doc = doc
	return nil
}

func (s *SqliteStore) Apply(apply func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = apply(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.reload()
}

func (s *SqliteStore) Close() error {
	return s.db.Close()
}

func (s *SqliteStore) reload() error {
	doc, err := s.readDocument()
	if err != nil {
		return err
	}
	if doc.GeneratedAt == "" && s.doc.GeneratedAt != "" {
		doc.GeneratedAt = s.doc.GeneratedAt
	}
	doc.Enabled = doc.Enabled || s.doc.Enabled
	doc.UIMode = firstNonEmpty(doc.UIMode, s.doc.UIMode)
	doc.Profile = firstNonEmpty(doc.Profile, s.doc.Profile)
	doc.GroupName = firstNonEmpty(doc.GroupName, s.doc.GroupName)
	if len(doc.Topics) == 0 {
		doc.Topics = append([]string{}, s.doc.Topics...)
	}
	if len(doc.Health.EffectiveTopics) == 0 {
		doc.Health.EffectiveTopics = append([]string{}, s.doc.Health.EffectiveTopics...)
	}
	s.doc = doc
	return nil
}

func (s *SqliteStore) replaceWithDocument(doc Snapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, stmt := range []string{
		"DELETE FROM flow_participants",
		"DELETE FROM topic_agents",
		"DELETE FROM trace_agents",
		"DELETE FROM trace_span_types",
		"DELETE FROM replay_sessions",
		"DELETE FROM health_snapshots",
		"DELETE FROM messages",
		"DELETE FROM flows",
		"DELETE FROM traces",
		"DELETE FROM tasks",
		"DELETE FROM topic_stats",
	} {
		if _, err = tx.Exec(stmt); err != nil {
			return err
		}
	}

	if err = insertMessages(tx, doc.Messages); err != nil {
		return err
	}
	if err = insertFlows(tx, doc.Flows); err != nil {
		return err
	}
	if err = insertTraces(tx, doc.Traces); err != nil {
		return err
	}
	if err = insertTasks(tx, doc.Tasks); err != nil {
		return err
	}
	if err = insertTopicHealth(tx, doc.Health.TopicHealth); err != nil {
		return err
	}
	if err = insertReplaySessions(tx, doc.ReplaySessions); err != nil {
		return err
	}
	if err = insertHealthSnapshot(tx, doc); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SqliteStore) isEmpty() (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM (
		SELECT id FROM flows
		UNION ALL SELECT id FROM traces
		UNION ALL SELECT id FROM tasks
		UNION ALL SELECT record_id FROM messages
		UNION ALL SELECT id FROM replay_sessions
		UNION ALL SELECT taken_at FROM health_snapshots
	)`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *SqliteStore) readDocument() (Snapshot, error) {
	doc := Snapshot{
		Health: Health{
			RejectedByReason: map[string]int{},
		},
	}

	var err error
	if doc.Messages, err = readMessages(s.db); err != nil {
		return Snapshot{}, err
	}
	if doc.Flows, err = readFlows(s.db); err != nil {
		return Snapshot{}, err
	}
	if doc.Traces, err = readTraces(s.db); err != nil {
		return Snapshot{}, err
	}
	if doc.Tasks, err = readTasks(s.db); err != nil {
		return Snapshot{}, err
	}
	if doc.ReplaySessions, err = readReplaySessions(s.db); err != nil {
		return Snapshot{}, err
	}
	if doc.Health, err = readHealth(s.db); err != nil {
		return Snapshot{}, err
	}

	doc.FlowCount = len(doc.Flows)
	doc.TraceCount = len(doc.Traces)
	doc.TaskCount = len(doc.Tasks)
	doc.MessageCount = len(doc.Messages)
	doc.GeneratedAt = firstNonEmpty(doc.Health.LastPollAt, doc.Health.ReplayLastFinishedAt)
	return doc, nil
}

func hasDocumentData(doc Snapshot) bool {
	return doc.GeneratedAt != "" ||
		doc.Enabled ||
		doc.UIMode != "" ||
		doc.Profile != "" ||
		doc.GroupName != "" ||
		len(doc.Topics) > 0 ||
		len(doc.ReplaySessions) > 0 ||
		len(doc.Flows) > 0 ||
		len(doc.Traces) > 0 ||
		len(doc.Tasks) > 0 ||
		len(doc.Messages) > 0 ||
		doc.Health.Connected ||
		doc.Health.GroupID != "" ||
		doc.Health.AcceptedCount != 0 ||
		doc.Health.RejectedCount != 0 ||
		doc.Health.MirroredCount != 0 ||
		doc.Health.MirrorFailedCount != 0 ||
		len(doc.Health.RejectedByReason) > 0 ||
		len(doc.Health.TopicHealth) > 0
}

func sqlitePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(trimmed), ".json") {
		return strings.TrimSuffix(trimmed, filepath.Ext(trimmed)) + ".db"
	}
	return trimmed
}

func insertMessages(tx *sql.Tx, messages []Message) error {
	stmt, err := tx.Prepare(`
		INSERT INTO messages (
			record_id, topic, topic_family, partition, offset, timestamp,
			envelope_type, sender_id, correlation_id, trace_id, task_id,
			parent_task_id, status, preview, content, lfs_bucket, lfs_key,
			lfs_size, lfs_sha256, lfs_content_type, lfs_created_at, lfs_proxy_id,
			outcome, reject_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, msg := range messages {
		lfsBucket, lfsKey, lfsSHA, lfsContentType, lfsCreatedAt, lfsProxyID := "", "", "", "", "", ""
		var lfsSize int64
		if msg.LFS != nil {
			lfsBucket = msg.LFS.Bucket
			lfsKey = msg.LFS.Key
			lfsSize = msg.LFS.Size
			lfsSHA = msg.LFS.SHA256
			lfsContentType = msg.LFS.ContentType
			lfsCreatedAt = msg.LFS.CreatedAt
			lfsProxyID = msg.LFS.ProxyID
		}
		if _, err := stmt.Exec(
			msg.ID, msg.Topic, msg.TopicFamily, msg.Partition, msg.Offset, msg.Timestamp,
			emptyToNil(msg.EnvelopeType), emptyToNil(msg.SenderID), emptyToNil(msg.CorrelationID),
			emptyToNil(msg.TraceID), emptyToNil(msg.TaskID), emptyToNil(msg.ParentTaskID),
			emptyToNil(msg.Status), emptyToNil(msg.Preview), emptyToNil(msg.Content),
			emptyToNil(lfsBucket), emptyToNil(lfsKey), lfsSize, emptyToNil(lfsSHA),
			emptyToNil(lfsContentType), emptyToNil(lfsCreatedAt), emptyToNil(lfsProxyID),
			"accepted", nil,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertFlows(tx *sql.Tx, flows []Flow) error {
	stmt, err := tx.Prepare(`INSERT INTO flows (id, first_seen, last_seen, message_count, latest_status, latest_preview) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	partStmt, err := tx.Prepare(`INSERT INTO flow_participants (flow_id, kind, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer partStmt.Close()

	for _, flow := range flows {
		if _, err := stmt.Exec(flow.ID, flow.FirstSeen, flow.LastSeen, flow.MessageCount, emptyToNil(flow.LatestStatus), emptyToNil(flow.LatestPreview)); err != nil {
			return err
		}
		for _, topic := range flow.Topics {
			if _, err := partStmt.Exec(flow.ID, "topic", topic); err != nil {
				return err
			}
		}
		for _, sender := range flow.Senders {
			if _, err := partStmt.Exec(flow.ID, "sender", sender); err != nil {
				return err
			}
		}
		for _, traceID := range flow.TraceIDs {
			if _, err := partStmt.Exec(flow.ID, "trace", traceID); err != nil {
				return err
			}
		}
		for _, taskID := range flow.TaskIDs {
			if _, err := partStmt.Exec(flow.ID, "task", taskID); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertTraces(tx *sql.Tx, traces []Trace) error {
	stmt, err := tx.Prepare(`INSERT INTO traces (id, span_count, latest_title, started_at, ended_at, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	agentStmt, err := tx.Prepare(`INSERT INTO trace_agents (trace_id, agent_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer agentStmt.Close()
	spanStmt, err := tx.Prepare(`INSERT INTO trace_span_types (trace_id, span_type) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer spanStmt.Close()

	for _, trace := range traces {
		if _, err := stmt.Exec(trace.ID, trace.SpanCount, emptyToNil(trace.LatestTitle), emptyToNil(trace.StartedAt), emptyToNil(trace.EndedAt), trace.DurationMs); err != nil {
			return err
		}
		for _, agent := range trace.Agents {
			if _, err := agentStmt.Exec(trace.ID, agent); err != nil {
				return err
			}
		}
		for _, spanType := range trace.SpanTypes {
			if _, err := spanStmt.Exec(trace.ID, spanType); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertTasks(tx *sql.Tx, tasks []Task) error {
	stmt, err := tx.Prepare(`
		INSERT INTO tasks (
			id, parent_task_id, delegation_depth, requester_id, responder_id,
			original_requester_id, status, description, last_summary, first_seen, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, task := range tasks {
		if _, err := stmt.Exec(
			task.ID, emptyToNil(task.ParentTaskID), task.DelegationDepth,
			emptyToNil(task.RequesterID), emptyToNil(task.ResponderID),
			emptyToNil(task.OriginalRequesterID), emptyToNil(task.Status),
			emptyToNil(task.Description), emptyToNil(task.LastSummary),
			task.FirstSeen, task.LastSeen,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertTopicHealth(tx *sql.Tx, topics []TopicHealth) error {
	stmt, err := tx.Prepare(`INSERT INTO topic_stats (topic, message_count, active_agents, first_message_at, last_message_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	agentStmt, err := tx.Prepare(`INSERT INTO topic_agents (topic, agent_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer agentStmt.Close()

	for _, topic := range topics {
		messageCount := int(topic.MessagesPerHour)
		if _, err := stmt.Exec(topic.Topic, messageCount, topic.ActiveAgents, emptyToNil(topic.LastMessageAt), emptyToNil(topic.LastMessageAt)); err != nil {
			return err
		}
		for i := 0; i < topic.ActiveAgents; i++ {
			if _, err := agentStmt.Exec(topic.Topic, "synthetic-agent-"+strconvI(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertReplaySessions(tx *sql.Tx, sessions []ReplaySession) error {
	stmt, err := tx.Prepare(`INSERT INTO replay_sessions (id, group_id, status, started_at, finished_at, message_count, topics_json, last_error) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, session := range sessions {
		topicsJSON := "[]"
		if len(session.Topics) > 0 {
			raw, err := json.Marshal(session.Topics)
			if err != nil {
				return err
			}
			topicsJSON = string(raw)
		}
		if _, err := stmt.Exec(session.ID, session.GroupID, session.Status, session.StartedAt, emptyToNil(session.FinishedAt), session.MessageCount, topicsJSON, emptyToNil(session.LastError)); err != nil {
			return err
		}
	}
	return nil
}

func insertHealthSnapshot(tx *sql.Tx, doc Snapshot) error {
	rejectedJSON, err := json.Marshal(doc.Health.RejectedByReason)
	if err != nil {
		return err
	}
	takenAt := firstNonEmpty(doc.GeneratedAt, time.Now().UTC().Format(time.RFC3339))
	_, err = tx.Exec(`
		INSERT INTO health_snapshots (
			taken_at, connected, group_id, accepted_count, rejected_count, mirrored_count,
			mirror_failed_count, last_reject, last_mirror_error, last_poll_at, replay_status,
			replay_active, replay_last_error, replay_last_finished, replay_last_count,
			rejected_by_reason_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		takenAt, boolToInt(doc.Health.Connected), emptyToNil(doc.Health.GroupID), doc.Health.AcceptedCount,
		doc.Health.RejectedCount, doc.Health.MirroredCount, doc.Health.MirrorFailedCount,
		emptyToNil(doc.Health.LastReject), emptyToNil(doc.Health.LastMirrorError),
		emptyToNil(doc.Health.LastPollAt), emptyToNil(doc.Health.ReplayStatus),
		doc.Health.ReplayActive, emptyToNil(doc.Health.ReplayLastError),
		emptyToNil(doc.Health.ReplayLastFinishedAt), doc.Health.ReplayLastRecordCount,
		string(rejectedJSON),
	)
	return err
}

func readMessages(db *sql.DB) ([]Message, error) {
	rows, err := db.Query(`
		SELECT record_id, topic, topic_family, partition, offset, timestamp, envelope_type, sender_id,
		       correlation_id, trace_id, task_id, parent_task_id, status, preview, content,
		       lfs_bucket, lfs_key, lfs_size, lfs_sha256, lfs_content_type, lfs_created_at, lfs_proxy_id
		  FROM messages
		 ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var msg Message
		var envelopeType, senderID, correlationID, traceID, taskID, parentTaskID, status, preview, content sql.NullString
		var lfsBucket, lfsKey, lfsSHA, lfsContentType, lfsCreatedAt, lfsProxyID sql.NullString
		var lfsSize sql.NullInt64
		if err := rows.Scan(
			&msg.ID, &msg.Topic, &msg.TopicFamily, &msg.Partition, &msg.Offset, &msg.Timestamp,
			&envelopeType, &senderID, &correlationID, &traceID, &taskID, &parentTaskID,
			&status, &preview, &content, &lfsBucket, &lfsKey, &lfsSize, &lfsSHA,
			&lfsContentType, &lfsCreatedAt, &lfsProxyID,
		); err != nil {
			return nil, err
		}
		msg.EnvelopeType = envelopeType.String
		msg.SenderID = senderID.String
		msg.CorrelationID = correlationID.String
		msg.TraceID = traceID.String
		msg.TaskID = taskID.String
		msg.ParentTaskID = parentTaskID.String
		msg.Status = status.String
		msg.Preview = preview.String
		msg.Content = content.String
		if lfsBucket.Valid || lfsKey.Valid {
			msg.LFS = &LFSPointer{
				Bucket:      lfsBucket.String,
				Key:         lfsKey.String,
				Size:        lfsSize.Int64,
				SHA256:      lfsSHA.String,
				ContentType: lfsContentType.String,
				CreatedAt:   lfsCreatedAt.String,
				ProxyID:     lfsProxyID.String,
				Path:        "s3://" + lfsBucket.String + "/" + lfsKey.String,
			}
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func readFlows(db *sql.DB) ([]Flow, error) {
	rows, err := db.Query(`SELECT id, first_seen, last_seen, message_count, latest_status, latest_preview FROM flows ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Flow{}
	index := map[string]*Flow{}
	for rows.Next() {
		var flow Flow
		var latestStatus, latestPreview sql.NullString
		if err := rows.Scan(&flow.ID, &flow.FirstSeen, &flow.LastSeen, &flow.MessageCount, &latestStatus, &latestPreview); err != nil {
			return nil, err
		}
		flow.LatestStatus = latestStatus.String
		flow.LatestPreview = latestPreview.String
		out = append(out, flow)
		index[flow.ID] = &out[len(out)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	partRows, err := db.Query(`SELECT flow_id, kind, value FROM flow_participants ORDER BY flow_id, kind, value`)
	if err != nil {
		return nil, err
	}
	defer partRows.Close()
	for partRows.Next() {
		var flowID, kind, value string
		if err := partRows.Scan(&flowID, &kind, &value); err != nil {
			return nil, err
		}
		flow := index[flowID]
		if flow == nil {
			continue
		}
		switch kind {
		case "topic":
			flow.Topics = append(flow.Topics, value)
		case "sender":
			flow.Senders = append(flow.Senders, value)
		case "trace":
			flow.TraceIDs = append(flow.TraceIDs, value)
		case "task":
			flow.TaskIDs = append(flow.TaskIDs, value)
		}
	}
	for i := range out {
		out[i].TopicCount = len(out[i].Topics)
		out[i].SenderCount = len(out[i].Senders)
	}
	return out, partRows.Err()
}

func readTraces(db *sql.DB) ([]Trace, error) {
	rows, err := db.Query(`SELECT id, span_count, latest_title, started_at, ended_at, duration_ms FROM traces ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Trace{}
	index := map[string]*Trace{}
	for rows.Next() {
		var trace Trace
		var latestTitle, startedAt, endedAt sql.NullString
		if err := rows.Scan(&trace.ID, &trace.SpanCount, &latestTitle, &startedAt, &endedAt, &trace.DurationMs); err != nil {
			return nil, err
		}
		trace.LatestTitle = latestTitle.String
		trace.StartedAt = startedAt.String
		trace.EndedAt = endedAt.String
		out = append(out, trace)
		index[trace.ID] = &out[len(out)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachTraceSlices(db, index); err != nil {
		return nil, err
	}
	return out, nil
}

func attachTraceSlices(db *sql.DB, index map[string]*Trace) error {
	agents, err := db.Query(`SELECT trace_id, agent_id FROM trace_agents ORDER BY trace_id, agent_id`)
	if err != nil {
		return err
	}
	defer agents.Close()
	for agents.Next() {
		var traceID, agentID string
		if err := agents.Scan(&traceID, &agentID); err != nil {
			return err
		}
		if trace := index[traceID]; trace != nil {
			trace.Agents = append(trace.Agents, agentID)
		}
	}
	if err := agents.Err(); err != nil {
		return err
	}

	spanTypes, err := db.Query(`SELECT trace_id, span_type FROM trace_span_types ORDER BY trace_id, span_type`)
	if err != nil {
		return err
	}
	defer spanTypes.Close()
	for spanTypes.Next() {
		var traceID, spanType string
		if err := spanTypes.Scan(&traceID, &spanType); err != nil {
			return err
		}
		if trace := index[traceID]; trace != nil {
			trace.SpanTypes = append(trace.SpanTypes, spanType)
		}
	}
	return spanTypes.Err()
}

func readTasks(db *sql.DB) ([]Task, error) {
	rows, err := db.Query(`
		SELECT id, parent_task_id, delegation_depth, requester_id, responder_id,
		       original_requester_id, status, description, last_summary, first_seen, last_seen
		  FROM tasks
		 ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		var task Task
		var parentTaskID, requesterID, responderID, originalRequesterID, status, description, lastSummary sql.NullString
		if err := rows.Scan(
			&task.ID, &parentTaskID, &task.DelegationDepth, &requesterID, &responderID,
			&originalRequesterID, &status, &description, &lastSummary, &task.FirstSeen, &task.LastSeen,
		); err != nil {
			return nil, err
		}
		task.ParentTaskID = parentTaskID.String
		task.RequesterID = requesterID.String
		task.ResponderID = responderID.String
		task.OriginalRequesterID = originalRequesterID.String
		task.Status = status.String
		task.Description = description.String
		task.LastSummary = lastSummary.String
		out = append(out, task)
	}
	return out, rows.Err()
}

func readReplaySessions(db *sql.DB) ([]ReplaySession, error) {
	rows, err := db.Query(`SELECT id, group_id, status, started_at, finished_at, message_count, topics_json, last_error FROM replay_sessions ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReplaySession
	for rows.Next() {
		var session ReplaySession
		var finishedAt, topicsJSON, lastError sql.NullString
		if err := rows.Scan(&session.ID, &session.GroupID, &session.Status, &session.StartedAt, &finishedAt, &session.MessageCount, &topicsJSON, &lastError); err != nil {
			return nil, err
		}
		session.FinishedAt = finishedAt.String
		session.LastError = lastError.String
		if topicsJSON.Valid && topicsJSON.String != "" {
			_ = json.Unmarshal([]byte(topicsJSON.String), &session.Topics)
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func readHealth(db *sql.DB) (Health, error) {
	health := Health{
		RejectedByReason: map[string]int{},
	}
	row := db.QueryRow(`
		SELECT connected, group_id, accepted_count, rejected_count, mirrored_count,
		       mirror_failed_count, last_reject, last_mirror_error, last_poll_at,
		       replay_status, replay_active, replay_last_error, replay_last_finished,
		       replay_last_count, rejected_by_reason_json
		  FROM health_snapshots
		 ORDER BY taken_at DESC
		 LIMIT 1
	`)

	var connected int
	var groupID, lastReject, lastMirrorError, lastPollAt, replayStatus, replayLastError, replayLastFinished, rejectedJSON sql.NullString
	err := row.Scan(
		&connected, &groupID, &health.AcceptedCount, &health.RejectedCount, &health.MirroredCount,
		&health.MirrorFailedCount, &lastReject, &lastMirrorError, &lastPollAt,
		&replayStatus, &health.ReplayActive, &replayLastError, &replayLastFinished,
		&health.ReplayLastRecordCount, &rejectedJSON,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Health{}, err
	}
	if err == nil {
		health.Connected = connected == 1
		health.GroupID = groupID.String
		health.LastReject = lastReject.String
		health.LastMirrorError = lastMirrorError.String
		health.LastPollAt = lastPollAt.String
		health.ReplayStatus = replayStatus.String
		health.ReplayLastError = replayLastError.String
		health.ReplayLastFinishedAt = replayLastFinished.String
		if rejectedJSON.Valid && rejectedJSON.String != "" {
			_ = json.Unmarshal([]byte(rejectedJSON.String), &health.RejectedByReason)
		}
	}

	topics, err := db.Query(`SELECT topic, active_agents, last_message_at FROM topic_stats ORDER BY topic`)
	if err != nil {
		return Health{}, err
	}
	defer topics.Close()
	for topics.Next() {
		var item TopicHealth
		var activeAgents int
		var lastMessageAt sql.NullString
		if err := topics.Scan(&item.Topic, &activeAgents, &lastMessageAt); err != nil {
			return Health{}, err
		}
		item.ActiveAgents = activeAgents
		item.LastMessageAt = lastMessageAt.String
		item.IsStale = storeTopicIsStale(item.LastMessageAt)
		item.MessageDensity = storeDensityBucket(item.MessagesPerHour)
		health.TopicHealth = append(health.TopicHealth, item)
		health.EffectiveTopics = append(health.EffectiveTopics, item.Topic)
	}
	return health, topics.Err()
}

func cloneDocument(doc Snapshot) Snapshot {
	raw, _ := json.Marshal(doc)
	var out Snapshot
	_ = json.Unmarshal(raw, &out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func strconvI(i int) string {
	return strconv.Itoa(i)
}

func storeTopicIsStale(lastMessageAt string) bool {
	if strings.TrimSpace(lastMessageAt) == "" {
		return true
	}
	ts, err := time.Parse(time.RFC3339, lastMessageAt)
	if err != nil {
		return true
	}
	return time.Since(ts) > time.Hour
}

func storeDensityBucket(messagesPerHour float64) string {
	switch {
	case messagesPerHour >= 60:
		return "high"
	case messagesPerHour >= 10:
		return "medium"
	default:
		return "low"
	}
}
