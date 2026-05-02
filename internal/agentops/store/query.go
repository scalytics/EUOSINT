package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const defaultPageLimit = 50

type pageCursor struct {
	LastSeen string `json:"last_seen"`
	ID       string `json:"id"`
}

func (s *SqliteStore) ListFlows(ctx context.Context, filter FlowFilter, page Pagination) ([]Flow, Cursor, error) {
	limit := normalizeLimit(page.Limit)
	cursor, err := decodeCursor(page.After)
	if err != nil {
		return nil, "", err
	}

	args := []any{}
	clauses := []string{}
	joins := []string{}
	if topic := strings.TrimSpace(filter.Topic); topic != "" {
		joins = append(joins, `JOIN flow_participants fp_topic ON fp_topic.flow_id = f.id AND fp_topic.kind = 'topic'`)
		clauses = append(clauses, "fp_topic.value = ?")
		args = append(args, topic)
	}
	if sender := strings.TrimSpace(filter.Sender); sender != "" {
		joins = append(joins, `JOIN flow_participants fp_sender ON fp_sender.flow_id = f.id AND fp_sender.kind = 'sender'`)
		clauses = append(clauses, "fp_sender.value = ?")
		args = append(args, sender)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		clauses = append(clauses, "COALESCE(f.latest_status, '') = ?")
		args = append(args, status)
	}
	if text := strings.TrimSpace(filter.Text); text != "" {
		clauses = append(clauses, "(f.id LIKE ? OR COALESCE(f.latest_preview, '') LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like)
	}
	if cursor.LastSeen != "" && cursor.ID != "" {
		clauses = append(clauses, "(f.last_seen < ? OR (f.last_seen = ? AND f.id < ?))")
		args = append(args, cursor.LastSeen, cursor.LastSeen, cursor.ID)
	}

	query := `
		SELECT DISTINCT f.id, f.first_seen, f.last_seen, f.message_count, f.latest_status, f.latest_preview
		  FROM flows f
	`
	if len(joins) > 0 {
		query += "\n" + strings.Join(joins, "\n")
	}
	if len(clauses) > 0 {
		query += "\n WHERE " + strings.Join(clauses, " AND ")
	}
	query += "\n ORDER BY f.last_seen DESC, f.id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	flows := make([]Flow, 0, limit+1)
	index := map[string]*Flow{}
	for rows.Next() {
		var flow Flow
		var latestStatus, latestPreview sql.NullString
		if err := rows.Scan(&flow.ID, &flow.FirstSeen, &flow.LastSeen, &flow.MessageCount, &latestStatus, &latestPreview); err != nil {
			return nil, "", err
		}
		flow.LatestStatus = latestStatus.String
		flow.LatestPreview = latestPreview.String
		flows = append(flows, flow)
		index[flow.ID] = &flows[len(flows)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	next, flows := trimFlowPage(flows, limit)
	if err := attachFlowParticipants(ctx, s.db, index); err != nil {
		return nil, "", err
	}
	return flows, next, nil
}

func (s *SqliteStore) GetFlow(ctx context.Context, id string) (Flow, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, first_seen, last_seen, message_count, latest_status, latest_preview
		  FROM flows
		 WHERE id = ?
	`, id)

	var flow Flow
	var latestStatus, latestPreview sql.NullString
	if err := row.Scan(&flow.ID, &flow.FirstSeen, &flow.LastSeen, &flow.MessageCount, &latestStatus, &latestPreview); err != nil {
		return Flow{}, err
	}
	flow.LatestStatus = latestStatus.String
	flow.LatestPreview = latestPreview.String
	index := map[string]*Flow{flow.ID: &flow}
	if err := attachFlowParticipants(ctx, s.db, index); err != nil {
		return Flow{}, err
	}
	return flow, nil
}

func (s *SqliteStore) ListMessagesForFlow(ctx context.Context, flowID string, page Pagination) ([]Message, Cursor, error) {
	limit := normalizeLimit(page.Limit)
	cursor, err := decodeCursor(page.After)
	if err != nil {
		return nil, "", err
	}

	args := []any{flowID}
	query := `
		SELECT record_id, topic, topic_family, partition, offset, timestamp, envelope_type, sender_id,
		       correlation_id, trace_id, task_id, parent_task_id, status, preview, content,
		       lfs_bucket, lfs_key, lfs_size, lfs_sha256, lfs_content_type, lfs_created_at, lfs_proxy_id
		  FROM messages
		 WHERE correlation_id = ?
	`
	if cursor.LastSeen != "" && cursor.ID != "" {
		query += ` AND (timestamp < ? OR (timestamp = ? AND record_id < ?))`
		args = append(args, cursor.LastSeen, cursor.LastSeen, cursor.ID)
	}
	query += ` ORDER BY timestamp DESC, record_id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	messages := make([]Message, 0, limit+1)
	for rows.Next() {
		msg, err := scanMessageRow(rows)
		if err != nil {
			return nil, "", err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	next := Cursor("")
	if len(messages) > limit {
		last := messages[limit-1]
		next, err = encodeCursor(last.Timestamp, last.ID)
		if err != nil {
			return nil, "", err
		}
		messages = messages[:limit]
	}
	return messages, next, nil
}

func (s *SqliteStore) ListTracesForFlow(ctx context.Context, flowID string) ([]Trace, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.span_count, t.latest_title, t.started_at, t.ended_at, t.duration_ms
		  FROM traces t
		  JOIN flow_participants fp ON fp.value = t.id
		 WHERE fp.flow_id = ? AND fp.kind = 'trace'
		 ORDER BY t.started_at DESC, t.id DESC
	`, flowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	traces := []Trace{}
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
		traces = append(traces, trace)
		index[trace.ID] = &traces[len(traces)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachTraceSlicesContext(ctx, s.db, index); err != nil {
		return nil, err
	}
	return traces, nil
}

func (s *SqliteStore) ListTasksForFlow(ctx context.Context, flowID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.parent_task_id, t.delegation_depth, t.requester_id, t.responder_id,
		       t.original_requester_id, t.status, t.description, t.last_summary, t.first_seen, t.last_seen
		  FROM tasks t
		  JOIN flow_participants fp ON fp.value = t.id
		 WHERE fp.flow_id = ? AND fp.kind = 'task'
		 ORDER BY t.last_seen DESC, t.id DESC
	`, flowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SqliteStore) TopicHealth(ctx context.Context) ([]TopicHealth, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT topic, message_count, active_agents, last_message_at
		  FROM topic_stats
		 ORDER BY topic
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []TopicHealth
	for rows.Next() {
		var item TopicHealth
		var messageCount int
		var lastMessageAt sql.NullString
		if err := rows.Scan(&item.Topic, &messageCount, &item.ActiveAgents, &lastMessageAt); err != nil {
			return nil, err
		}
		item.MessagesPerHour = float64(messageCount)
		item.LastMessageAt = lastMessageAt.String
		item.IsStale = storeTopicIsStale(item.LastMessageAt)
		item.MessageDensity = storeDensityBucket(item.MessagesPerHour)
		topics = append(topics, item)
	}
	return topics, rows.Err()
}

func (s *SqliteStore) RecentReplays(ctx context.Context, limit int) ([]ReplaySession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, status, started_at, finished_at, message_count, topics_json, last_error
		  FROM replay_sessions
		 ORDER BY started_at DESC, id DESC
		 LIMIT ?
	`, normalizeLimit(limit))
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

func (s *SqliteStore) LatestHealth(ctx context.Context) (Health, error) {
	return readHealthContext(ctx, s.db)
}

func trimFlowPage(flows []Flow, limit int) (Cursor, []Flow) {
	if len(flows) <= limit {
		return "", flows
	}
	last := flows[limit-1]
	next, err := encodeCursor(last.LastSeen, last.ID)
	if err != nil {
		return "", flows[:limit]
	}
	return next, flows[:limit]
}

func attachFlowParticipants(ctx context.Context, db *sql.DB, index map[string]*Flow) error {
	if len(index) == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `SELECT flow_id, kind, value FROM flow_participants ORDER BY flow_id, kind, value`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var flowID, kind, value string
		if err := rows.Scan(&flowID, &kind, &value); err != nil {
			return err
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
	if err := rows.Err(); err != nil {
		return err
	}
	for _, flow := range index {
		flow.TopicCount = len(flow.Topics)
		flow.SenderCount = len(flow.Senders)
	}
	return nil
}

func scanMessageRow(scanner interface{ Scan(...any) error }) (Message, error) {
	var msg Message
	var envelopeType, senderID, correlationID, traceID, taskID, parentTaskID, status, preview, content sql.NullString
	var lfsBucket, lfsKey, lfsSHA, lfsContentType, lfsCreatedAt, lfsProxyID sql.NullString
	var lfsSize sql.NullInt64
	if err := scanner.Scan(
		&msg.ID, &msg.Topic, &msg.TopicFamily, &msg.Partition, &msg.Offset, &msg.Timestamp,
		&envelopeType, &senderID, &correlationID, &traceID, &taskID, &parentTaskID,
		&status, &preview, &content, &lfsBucket, &lfsKey, &lfsSize, &lfsSHA,
		&lfsContentType, &lfsCreatedAt, &lfsProxyID,
	); err != nil {
		return Message{}, err
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
	return msg, nil
}

func scanTaskRow(scanner interface{ Scan(...any) error }) (Task, error) {
	var task Task
	var parentTaskID, requesterID, responderID, originalRequesterID, status, description, lastSummary sql.NullString
	if err := scanner.Scan(
		&task.ID, &parentTaskID, &task.DelegationDepth, &requesterID, &responderID,
		&originalRequesterID, &status, &description, &lastSummary, &task.FirstSeen, &task.LastSeen,
	); err != nil {
		return Task{}, err
	}
	task.ParentTaskID = parentTaskID.String
	task.RequesterID = requesterID.String
	task.ResponderID = responderID.String
	task.OriginalRequesterID = originalRequesterID.String
	task.Status = status.String
	task.Description = description.String
	task.LastSummary = lastSummary.String
	return task, nil
}

func attachTraceSlicesContext(ctx context.Context, db *sql.DB, index map[string]*Trace) error {
	if len(index) == 0 {
		return nil
	}
	agents, err := db.QueryContext(ctx, `SELECT trace_id, agent_id FROM trace_agents ORDER BY trace_id, agent_id`)
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

	spanTypes, err := db.QueryContext(ctx, `SELECT trace_id, span_type FROM trace_span_types ORDER BY trace_id, span_type`)
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

func encodeCursor(lastSeen, id string) (Cursor, error) {
	raw, err := json.Marshal(pageCursor{LastSeen: lastSeen, ID: id})
	if err != nil {
		return "", err
	}
	return Cursor(base64.RawURLEncoding.EncodeToString(raw)), nil
}

func decodeCursor(cursor Cursor) (pageCursor, error) {
	if strings.TrimSpace(string(cursor)) == "" {
		return pageCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(string(cursor))
	if err != nil {
		return pageCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var parsed pageCursor
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return pageCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	if parsed.LastSeen == "" || parsed.ID == "" {
		return pageCursor{}, errors.New("decode cursor: missing last_seen or id")
	}
	return parsed, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultPageLimit
	}
	if limit > 500 {
		return 500
	}
	return limit
}
