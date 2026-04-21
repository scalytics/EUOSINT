// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	agentcfg "github.com/scalytics/kafSIEM/internal/agentops/config"
	"github.com/scalytics/kafSIEM/internal/agentops/contract"
	"github.com/scalytics/kafSIEM/internal/agentops/store"
	collectorcfg "github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/graph"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type Service struct {
	cfg                   collectorcfg.Config
	policy                agentcfg.Policy
	topics                []string
	file                  *store.SqliteStore
	clientFactory         func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error)
	operatorClientFactory func(cfg collectorcfg.Config, clientID string) (operatorClient, error)
	replayStarter         func(context.Context, []string) (store.ReplaySession, error)
	replayMu              sync.Mutex
	replayCancels         map[string]context.CancelFunc
}

type agentopsClient interface {
	PollFetches(context.Context) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	ProduceSync(context.Context, *kgo.Record) error
	Close()
}

type operatorClient interface {
	ListGroups(context.Context) ([]store.ConsumerGroup, error)
	Close()
}

var (
	currentMu            sync.RWMutex
	currentService       *Service
	nowFunc              = time.Now
	defaultClientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return newClient(cfg, topics, groupID, clientID)
	}
	defaultOperatorClientFactory = func(cfg collectorcfg.Config, clientID string) (operatorClient, error) {
		return newOperatorClient(cfg, clientID)
	}
)

type kafkaClient struct {
	inner *kgo.Client
}

type kafkaOperatorClient struct {
	inner *kgo.Client
}

func (c *kafkaClient) PollFetches(ctx context.Context) kgo.Fetches {
	return c.inner.PollFetches(ctx)
}

func (c *kafkaClient) CommitRecords(ctx context.Context, records ...*kgo.Record) error {
	return c.inner.CommitRecords(ctx, records...)
}

func (c *kafkaClient) ProduceSync(ctx context.Context, record *kgo.Record) error {
	return c.inner.ProduceSync(ctx, record).FirstErr()
}

func (c *kafkaClient) Close() {
	c.inner.Close()
}

func (c *kafkaOperatorClient) ListGroups(ctx context.Context) ([]store.ConsumerGroup, error) {
	listReq := kmsg.NewPtrListGroupsRequest()
	listReq.Version = 5
	listResp, err := listReq.RequestWith(ctx, c.inner)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]string, 0, len(listResp.Groups))
	out := make([]store.ConsumerGroup, 0, len(listResp.Groups))
	for _, item := range listResp.Groups {
		groupIDs = append(groupIDs, item.Group)
		out = append(out, store.ConsumerGroup{
			GroupID:      item.Group,
			State:        item.GroupState,
			ProtocolType: item.ProtocolType,
		})
	}
	if len(groupIDs) == 0 {
		return out, nil
	}
	describeReq := kmsg.NewPtrDescribeGroupsRequest()
	describeReq.Version = 5
	describeReq.Groups = groupIDs
	describeResp, err := describeReq.RequestWith(ctx, c.inner)
	if err != nil {
		return nil, err
	}
	described := make(map[string]store.ConsumerGroup, len(describeResp.Groups))
	for _, item := range describeResp.Groups {
		members := make([]store.ConsumerGroupMember, 0, len(item.Members))
		for _, member := range item.Members {
			instanceID := ""
			if member.InstanceID != nil {
				instanceID = *member.InstanceID
			}
			members = append(members, store.ConsumerGroupMember{
				MemberID:   member.MemberID,
				ClientID:   member.ClientID,
				ClientHost: member.ClientHost,
				InstanceID: instanceID,
			})
		}
		described[item.Group] = store.ConsumerGroup{
			GroupID:      item.Group,
			State:        item.State,
			ProtocolType: item.ProtocolType,
			Protocol:     item.Protocol,
			Members:      members,
		}
	}
	for i := range out {
		if item, ok := described[out[i].GroupID]; ok {
			out[i] = item
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GroupID < out[j].GroupID })
	return out, nil
}

func (c *kafkaOperatorClient) Close() {
	c.inner.Close()
}

func Start(ctx context.Context, cfg collectorcfg.Config) error {
	if !cfg.AgentOpsEnabled {
		return nil
	}
	policy, err := agentcfg.LoadPolicy(cfg.AgentOpsPolicyPath, cfg.AgentOpsGroupName)
	if err != nil {
		return fmt.Errorf("agentops policy: %w", err)
	}
	topics := contract.DeriveTopics(cfg.AgentOpsGroupName, policy.RequiredTopics, policy.OptionalTopics, cfg.AgentOpsTopics, cfg.AgentOpsTopicMode)
	doc := store.Snapshot{
		Enabled:   true,
		UIMode:    cfg.UIMode,
		Profile:   cfg.Profile,
		GroupName: cfg.AgentOpsGroupName,
		Topics:    topics,
		Health: store.Health{
			GroupID:          cfg.AgentOpsGroupID,
			EffectiveTopics:  topics,
			RejectedByReason: map[string]int{},
		},
	}
	fs, err := store.NewSqliteStore(cfg.AgentOpsOutputPath, doc)
	if err != nil {
		return fmt.Errorf("agentops store: %w", err)
	}
	svc := &Service{
		cfg:                   cfg,
		policy:                policy,
		topics:                topics,
		file:                  fs,
		clientFactory:         defaultClientFactory,
		operatorClientFactory: defaultOperatorClientFactory,
		replayStarter:         nil,
		replayCancels:         map[string]context.CancelFunc{},
	}
	setCurrentService(svc)
	return svc.run(ctx)
}

func (s *Service) run(ctx context.Context) error {
	clientFactory := s.clientFactory
	if clientFactory == nil {
		clientFactory = defaultClientFactory
	}
	client, err := clientFactory(s.cfg, s.topics, s.cfg.AgentOpsGroupID, s.cfg.AgentOpsClientID)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := s.file.Update(func(doc *store.Snapshot) {
		doc.Enabled = true
		doc.UIMode = s.cfg.UIMode
		doc.Profile = s.cfg.Profile
		doc.GroupName = s.cfg.AgentOpsGroupName
		doc.Topics = append([]string{}, s.topics...)
		doc.Health.GroupID = s.cfg.AgentOpsGroupID
		doc.Health.EffectiveTopics = append([]string{}, s.topics...)
	}); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if err := s.processReplayRequests(ctx); err != nil {
			_ = s.file.Update(func(doc *store.Snapshot) {
				doc.Health.LastReject = err.Error()
			})
		}

		pollCtx, cancel := context.WithTimeout(ctx, time.Duration(max(250, s.cfg.KafkaPollTimeoutMS))*time.Millisecond)
		fetches := client.PollFetches(pollCtx)
		cancel()
		if err := firstFatalError(fetches); err != nil {
			_ = s.file.Update(func(doc *store.Snapshot) {
				doc.Health.Connected = false
				doc.Health.LastReject = err.Error()
			})
			return err
		}
		records := make([]*kgo.Record, 0)
		fetches.EachRecord(func(rec *kgo.Record) {
			records = append(records, rec)
		})
		if len(records) == 0 {
			_ = s.file.Update(func(doc *store.Snapshot) {
				doc.Health.Connected = true
				doc.Health.LastPollAt = nowFunc().UTC().Format(time.RFC3339)
			})
			continue
		}

		toCommit := make([]*kgo.Record, 0, len(records))
		for _, rec := range records {
			reason, ok := s.handleRecord(rec)
			toCommit = append(toCommit, rec)
			if ok {
				continue
			}
			mirrored, mirrorErr := s.mirrorReject(ctx, client, rec, reason)
			_ = s.file.Update(func(doc *store.Snapshot) {
				doc.Health.RejectedCount++
				doc.Health.LastReject = reason
				doc.Health.RejectedByReason[reason]++
				if mirrored {
					doc.Health.MirroredCount++
				}
				if mirrorErr != nil {
					doc.Health.MirrorFailedCount++
					doc.Health.LastMirrorError = mirrorErr.Error()
				}
			})
		}
		if len(toCommit) > 0 {
			commitCtx, cancelCommit := context.WithTimeout(ctx, 5*time.Second)
			err = client.CommitRecords(commitCtx, toCommit...)
			cancelCommit()
			if err != nil {
				return err
			}
		}
	}
}

func (s *Service) processReplayRequests(ctx context.Context) error {
	type request struct {
		id     string
		topics []string
	}
	var next request
	if err := s.file.Apply(func(tx *sql.Tx) error {
		var id, topicsJSON string
		err := tx.QueryRow(`
			SELECT id, COALESCE(topics_json, '[]')
			  FROM replay_requests
			 WHERE status = 'pending'
			 ORDER BY requested_at ASC, id ASC
			 LIMIT 1
		`).Scan(&id, &topicsJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		next.id = id
		_ = json.Unmarshal([]byte(topicsJSON), &next.topics)
		_, err = tx.Exec(`UPDATE replay_requests SET status = 'claimed', last_error = NULL WHERE id = ?`, id)
		return err
	}); err != nil {
		return err
	}
	if next.id == "" {
		return nil
	}
	starter := s.replayStarter
	if starter == nil {
		starter = s.startReplayWithTopics
	}
	session, err := starter(ctx, next.topics)
	if err != nil {
		_ = s.file.Apply(func(tx *sql.Tx) error {
			_, execErr := tx.Exec(`UPDATE replay_requests SET status = 'failed', last_error = ? WHERE id = ?`, err.Error(), next.id)
			return execErr
		})
		return err
	}
	return s.file.Apply(func(tx *sql.Tx) error {
		_, execErr := tx.Exec(`UPDATE replay_requests SET status = 'accepted', started_session_id = ?, last_error = NULL WHERE id = ?`, session.ID, next.id)
		return execErr
	})
}

func (s *Service) handleRecord(rec *kgo.Record) (string, bool) {
	recordID := fmt.Sprintf("%s:%d:%d", rec.Topic, rec.Partition, rec.Offset)
	now := recordTimestamp(rec)
	family := contract.ClassifyTopic(rec.Topic, s.cfg.AgentOpsGroupName)
	if family == "" {
		return "unknown_topic", false
	}

	if ptr, ok, _ := contract.DecodeLFSPointer(rec.Value); ok {
		msg := store.Message{
			ID:          recordID,
			Topic:       rec.Topic,
			TopicFamily: family,
			Partition:   rec.Partition,
			Offset:      rec.Offset,
			Timestamp:   now,
			Preview:     "LFS-backed payload",
			LFS: &store.LFSPointer{
				Bucket:      ptr.Bucket,
				Key:         ptr.Key,
				Size:        ptr.Size,
				SHA256:      ptr.SHA256,
				ContentType: ptr.ContentType,
				CreatedAt:   ptr.CreatedAt,
				ProxyID:     ptr.ProxyID,
				Path:        fmt.Sprintf("s3://%s/%s", ptr.Bucket, ptr.Key),
			},
		}
		return s.acceptMessage("lfs", msg, nil, func(doc *store.Snapshot) {
			updateFlow(doc, msg)
			doc.Health.AcceptedCount++
		})
	}

	env, err := contract.DecodeEnvelope(rec.Value)
	if err != nil {
		if msg, ok := rawMessage(recordID, rec, family, now); ok {
			return s.acceptMessage("raw", msg, nil, func(doc *store.Snapshot) {
				updateFlow(doc, msg)
				doc.Health.AcceptedCount++
			})
		}
		return "invalid_envelope", false
	}
	msg := store.Message{
		ID:            recordID,
		Topic:         rec.Topic,
		TopicFamily:   family,
		Partition:     rec.Partition,
		Offset:        rec.Offset,
		Timestamp:     now,
		EnvelopeType:  env.Type,
		SenderID:      env.SenderID,
		CorrelationID: fallbackID(env.CorrelationID, recordID),
		Preview:       previewForPayload(env.Payload),
		Content:       compactJSON(env.Payload),
	}

	switch family {
	case "requests":
		var payload contract.TaskRequestPayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TaskID = payload.TaskID
			msg.ParentTaskID = payload.ParentTaskID
			msg.Preview = firstNonEmpty(payload.Description, payload.Content, msg.Preview)
		}
	case "responses":
		var payload contract.TaskResponsePayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TaskID = payload.TaskID
			msg.Status = payload.Status
			msg.Preview = firstNonEmpty(payload.Content, msg.Preview)
		}
	case "tasks.status":
		var payload contract.TaskStatusPayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TaskID = payload.TaskID
			msg.Status = payload.Status
			msg.Preview = firstNonEmpty(payload.Summary, payload.Status, msg.Preview)
		}
	case "traces":
		var payload contract.TracePayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TraceID = payload.TraceID
			msg.Preview = firstNonEmpty(payload.Title, payload.Content, msg.Preview)
		}
	case "observe.audit":
		msg.Preview = firstNonEmpty(previewForPayload(env.Payload), "audit event")
	}

	return s.acceptMessage(family, msg, env.Payload, func(doc *store.Snapshot) {
		updateFlow(doc, msg)
		updateTraceMessage(doc, now, msg, env.Payload)
		updateTaskMessage(doc, now, msg, env.Payload)
		doc.Health.AcceptedCount++
	})
}

func (s *Service) acceptMessage(branch string, msg store.Message, payload json.RawMessage, mutate func(*store.Snapshot)) (string, bool) {
	err := s.file.Update(func(doc *store.Snapshot) {
		if messageExists(doc.Messages, msg.ID) {
			return
		}
		if mutate != nil {
			mutate(doc)
		}
		doc.Messages = append(doc.Messages, msg)
		sort.Slice(doc.Messages, func(i, j int) bool { return doc.Messages[i].Timestamp > doc.Messages[j].Timestamp })
		if limit := s.policy.Grouping.ReplayMaxRecords; limit > 0 && len(doc.Messages) > limit {
			doc.Messages = doc.Messages[:limit]
		}
		doc.MessageCount = len(doc.Messages)
		doc.FlowCount = len(doc.Flows)
		doc.TraceCount = len(doc.Traces)
		doc.TaskCount = len(doc.Tasks)
		doc.Health.Connected = true
		doc.Health.GroupID = s.cfg.AgentOpsGroupID
		doc.Health.EffectiveTopics = append([]string{}, s.topics...)
		doc.Health.LastPollAt = nowFunc().UTC().Format(time.RFC3339)
		doc.Health.TopicHealth = rebuildTopicHealth(doc.Messages)
		if doc.Health.RejectedByReason == nil {
			doc.Health.RejectedByReason = map[string]int{}
		}
	})
	if err != nil {
		return "store_update_failed", false
	}
	if err := s.appendGraph(branch, msg, payload); err != nil {
		return "graph_write_failed", false
	}
	return "", true
}

func (s *Service) bootstrapFromStore(doc store.Snapshot) {
	if s.file == nil {
		return
	}
	_ = s.file.Update(func(current *store.Snapshot) {
		*current = doc
		current.Health.TopicHealth = rebuildTopicHealth(current.Messages)
		current.FlowCount = len(current.Flows)
		current.TraceCount = len(current.Traces)
		current.TaskCount = len(current.Tasks)
		current.MessageCount = len(current.Messages)
		if current.Health.RejectedByReason == nil {
			current.Health.RejectedByReason = map[string]int{}
		}
	})
}

func (s *Service) persist() error {
	if s.file == nil {
		return nil
	}
	return s.file.Update(func(doc *store.Snapshot) {
		sort.Slice(doc.Messages, func(i, j int) bool { return doc.Messages[i].Timestamp > doc.Messages[j].Timestamp })
		if limit := s.policy.Grouping.ReplayMaxRecords; limit > 0 && len(doc.Messages) > limit {
			doc.Messages = doc.Messages[:limit]
		}
		sort.Slice(doc.Flows, func(i, j int) bool { return doc.Flows[i].LastSeen > doc.Flows[j].LastSeen })
		sort.Slice(doc.Traces, func(i, j int) bool { return doc.Traces[i].StartedAt > doc.Traces[j].StartedAt })
		sort.Slice(doc.Tasks, func(i, j int) bool { return doc.Tasks[i].LastSeen > doc.Tasks[j].LastSeen })
		doc.FlowCount = len(doc.Flows)
		doc.TraceCount = len(doc.Traces)
		doc.TaskCount = len(doc.Tasks)
		doc.MessageCount = len(doc.Messages)
		doc.Health.Connected = true
		doc.Health.GroupID = s.cfg.AgentOpsGroupID
		doc.Health.EffectiveTopics = append([]string{}, s.topics...)
		doc.Health.LastPollAt = nowFunc().UTC().Format(time.RFC3339)
		doc.Health.TopicHealth = rebuildTopicHealth(doc.Messages)
		if doc.Health.RejectedByReason == nil {
			doc.Health.RejectedByReason = map[string]int{}
		}
	})
}

func updateFlow(doc *store.Snapshot, msg store.Message) {
	flowID := fallbackID(msg.CorrelationID, msg.ID)
	item := findFlow(doc.Flows, flowID)
	if item == nil {
		doc.Flows = append(doc.Flows, store.Flow{ID: flowID, FirstSeen: msg.Timestamp, LastSeen: msg.Timestamp})
		item = &doc.Flows[len(doc.Flows)-1]
	}
	item.LastSeen = msg.Timestamp
	item.MessageCount++
	item.LatestStatus = firstNonEmpty(msg.Status, item.LatestStatus)
	item.LatestPreview = firstNonEmpty(msg.Preview, item.LatestPreview)
	item.Topics = appendUnique(item.Topics, msg.Topic)
	item.Senders = appendUnique(item.Senders, msg.SenderID)
	item.TraceIDs = appendUnique(item.TraceIDs, msg.TraceID)
	item.TaskIDs = appendUnique(item.TaskIDs, msg.TaskID)
	item.TopicCount = len(item.Topics)
	item.SenderCount = len(item.Senders)
	sort.Slice(doc.Flows, func(i, j int) bool { return doc.Flows[i].LastSeen > doc.Flows[j].LastSeen })
}

func updateTraceMessage(doc *store.Snapshot, now string, msg store.Message, payload json.RawMessage) {
	if strings.TrimSpace(msg.TraceID) == "" {
		return
	}
	var tracePayload contract.TracePayload
	if json.Unmarshal(payload, &tracePayload) != nil || strings.TrimSpace(tracePayload.TraceID) == "" {
		return
	}
	item := findTrace(doc.Traces, tracePayload.TraceID)
	if item == nil {
		doc.Traces = append(doc.Traces, store.Trace{ID: tracePayload.TraceID, StartedAt: now})
		item = &doc.Traces[len(doc.Traces)-1]
	}
	item.SpanCount++
	item.Agents = appendUnique(item.Agents, msg.SenderID)
	item.SpanTypes = appendUnique(item.SpanTypes, tracePayload.SpanType)
	item.LatestTitle = firstNonEmpty(tracePayload.Title, item.LatestTitle)
	if tracePayload.StartedAt != "" {
		item.StartedAt = tracePayload.StartedAt
	}
	if tracePayload.EndedAt != "" {
		item.EndedAt = tracePayload.EndedAt
	}
	if tracePayload.DurationMs > 0 {
		item.DurationMs = tracePayload.DurationMs
	}
	sort.Slice(doc.Traces, func(i, j int) bool { return doc.Traces[i].StartedAt > doc.Traces[j].StartedAt })
}

func updateTaskMessage(doc *store.Snapshot, now string, msg store.Message, payload json.RawMessage) {
	if strings.TrimSpace(msg.TaskID) == "" {
		return
	}
	item := findTask(doc.Tasks, msg.TaskID)
	if item == nil {
		doc.Tasks = append(doc.Tasks, store.Task{ID: msg.TaskID, FirstSeen: now})
		item = &doc.Tasks[len(doc.Tasks)-1]
	}
	item.LastSeen = now
	switch msg.TopicFamily {
	case "requests":
		var taskPayload contract.TaskRequestPayload
		if json.Unmarshal(payload, &taskPayload) == nil {
			item.ParentTaskID = firstNonEmpty(taskPayload.ParentTaskID, item.ParentTaskID)
			item.RequesterID = firstNonEmpty(taskPayload.RequesterID, item.RequesterID)
			item.OriginalRequesterID = firstNonEmpty(taskPayload.OriginalRequesterID, item.OriginalRequesterID)
			item.Description = firstNonEmpty(taskPayload.Description, item.Description)
		}
	case "responses":
		var taskPayload contract.TaskResponsePayload
		if json.Unmarshal(payload, &taskPayload) == nil {
			item.ResponderID = firstNonEmpty(taskPayload.ResponderID, item.ResponderID)
			item.Status = firstNonEmpty(taskPayload.Status, item.Status)
			item.LastSummary = firstNonEmpty(taskPayload.Content, item.LastSummary)
		}
	case "tasks.status":
		var taskPayload contract.TaskStatusPayload
		if json.Unmarshal(payload, &taskPayload) == nil {
			item.ResponderID = firstNonEmpty(taskPayload.ResponderID, item.ResponderID)
			item.Status = firstNonEmpty(taskPayload.Status, item.Status)
			item.LastSummary = firstNonEmpty(taskPayload.Summary, item.LastSummary)
		}
	}
	sort.Slice(doc.Tasks, func(i, j int) bool { return doc.Tasks[i].LastSeen > doc.Tasks[j].LastSeen })
}

func StartReplay(ctx context.Context) (store.ReplaySession, error) {
	currentMu.RLock()
	svc := currentService
	currentMu.RUnlock()
	if svc == nil {
		return store.ReplaySession{}, errors.New("agentops runtime not active")
	}
	return svc.startReplay(ctx)
}

func LoadOperatorState(ctx context.Context) (store.OperatorState, error) {
	currentMu.RLock()
	svc := currentService
	currentMu.RUnlock()
	if svc == nil {
		return store.OperatorState{}, errors.New("agentops runtime not active")
	}
	return svc.loadOperatorState(ctx)
}

func (s *Service) startReplay(ctx context.Context) (store.ReplaySession, error) {
	if !s.cfg.AgentOpsReplayEnabled {
		return store.ReplaySession{}, errors.New("agentops replay disabled")
	}
	return s.startReplayWithTopics(ctx, nil)
}

func (s *Service) startReplayWithTopics(ctx context.Context, topics []string) (store.ReplaySession, error) {
	if len(topics) == 0 {
		topics = append([]string{}, s.topics...)
	}
	now := nowFunc().UTC()
	session := store.ReplaySession{
		ID:        now.Format("20060102T150405.000000000"),
		GroupID:   newReplayGroupID(s.cfg.AgentOpsReplayPrefix, now),
		Status:    "running",
		StartedAt: now.Format(time.RFC3339),
		Topics:    append([]string{}, topics...),
	}
	if err := s.file.Update(func(doc *store.Snapshot) {
		doc.ReplaySessions = append([]store.ReplaySession{session}, doc.ReplaySessions...)
		if len(doc.ReplaySessions) > 10 {
			doc.ReplaySessions = doc.ReplaySessions[:10]
		}
		doc.Health.ReplayStatus = "running"
		doc.Health.ReplayActive++
	}); err != nil {
		return store.ReplaySession{}, err
	}
	replayCtx, cancel := context.WithCancel(context.Background())
	s.replayMu.Lock()
	if s.replayCancels == nil {
		s.replayCancels = map[string]context.CancelFunc{}
	}
	s.replayCancels[session.ID] = cancel
	s.replayMu.Unlock()
	go s.runReplay(replayCtx, session)
	return session, nil
}

func (s *Service) runReplay(ctx context.Context, session store.ReplaySession) {
	clientFactory := s.clientFactory
	if clientFactory == nil {
		clientFactory = defaultClientFactory
	}
	topics := session.Topics
	if len(topics) == 0 {
		topics = s.topics
	}
	client, err := clientFactory(s.cfg, topics, session.GroupID, s.cfg.AgentOpsClientID+"-replay")
	if err != nil {
		s.finishReplay(session.ID, 0, "failed", err.Error())
		return
	}
	defer client.Close()

	idlePolls := 0
	processed := 0
	limit := s.policy.Grouping.ReplayMaxRecords
	if limit <= 0 {
		limit = 5000
	}
	for processed < limit {
		if err := ctx.Err(); err != nil {
			s.finishReplay(session.ID, processed, "canceled", "")
			return
		}
		pollCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		fetches := client.PollFetches(pollCtx)
		cancel()
		if err := firstFatalError(fetches); err != nil {
			if errors.Is(err, context.Canceled) {
				s.finishReplay(session.ID, processed, "canceled", "")
				return
			}
			s.finishReplay(session.ID, processed, "failed", err.Error())
			return
		}
		batch := make([]*kgo.Record, 0)
		fetches.EachRecord(func(rec *kgo.Record) {
			batch = append(batch, rec)
		})
		if len(batch) == 0 {
			idlePolls++
			if idlePolls >= 3 {
				break
			}
			continue
		}
		idlePolls = 0
		for _, rec := range batch {
			s.handleRecord(rec)
			processed++
			if processed >= limit {
				break
			}
		}
		commitCtx, cancelCommit := context.WithTimeout(ctx, 5*time.Second)
		_ = client.CommitRecords(commitCtx, batch...)
		cancelCommit()
	}
	s.finishReplay(session.ID, processed, "completed", "")
}

func (s *Service) finishReplay(id string, count int, status string, lastError string) {
	s.replayMu.Lock()
	delete(s.replayCancels, id)
	active := len(s.replayCancels)
	s.replayMu.Unlock()

	_ = s.file.Update(func(doc *store.Snapshot) {
		for i := range doc.ReplaySessions {
			if doc.ReplaySessions[i].ID == id {
				doc.ReplaySessions[i].Status = status
				doc.ReplaySessions[i].MessageCount = count
				doc.ReplaySessions[i].FinishedAt = nowFunc().UTC().Format(time.RFC3339)
				doc.ReplaySessions[i].LastError = lastError
				break
			}
		}
		if lastError != "" {
			doc.Health.LastReject = lastError
			doc.Health.ReplayLastError = lastError
		}
		doc.Health.ReplayStatus = status
		doc.Health.ReplayActive = active
		doc.Health.ReplayLastFinishedAt = nowFunc().UTC().Format(time.RFC3339)
		doc.Health.ReplayLastRecordCount = count
	})
}

func (s *Service) cancelReplay(id string) bool {
	s.replayMu.Lock()
	cancel, ok := s.replayCancels[id]
	s.replayMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (s *Service) loadOperatorState(ctx context.Context) (store.OperatorState, error) {
	operatorFactory := s.operatorClientFactory
	if operatorFactory == nil {
		operatorFactory = defaultOperatorClientFactory
	}
	client, err := operatorFactory(s.cfg, s.cfg.AgentOpsClientID+"-ops")
	if err != nil {
		return store.OperatorState{Supported: false, LiveGroupID: s.cfg.AgentOpsGroupID, ReplayGroupIDs: replayGroupIDs(s.file.Snapshot())}, err
	}
	defer client.Close()
	groups, err := client.ListGroups(ctx)
	if err != nil {
		return store.OperatorState{Supported: false, LiveGroupID: s.cfg.AgentOpsGroupID, ReplayGroupIDs: replayGroupIDs(s.file.Snapshot())}, err
	}
	return store.OperatorState{
		Supported:      true,
		LiveGroupID:    s.cfg.AgentOpsGroupID,
		ReplayGroupIDs: replayGroupIDs(s.file.Snapshot()),
		Groups:         groups,
	}, nil
}

func newClient(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
	if len(cfg.AgentOpsBrokers) == 0 {
		return nil, errors.New("AGENTOPS_BROKERS is required when AGENTOPS_ENABLED=true")
	}
	if strings.TrimSpace(groupID) == "" {
		return nil, errors.New("AGENTOPS_GROUP_ID is required when AGENTOPS_ENABLED=true")
	}
	if len(topics) == 0 {
		return nil, errors.New("no AgentOps topics resolved")
	}
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.AgentOpsBrokers...),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	}
	if strings.TrimSpace(clientID) != "" {
		opts = append(opts, kgo.ClientID(clientID))
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.AgentOpsSecurityProtocol)) {
	case "", "PLAINTEXT":
	case "SSL":
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: cfg.AgentOpsTLSInsecureSkipVerify}))
	case "SASL_SSL":
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: cfg.AgentOpsTLSInsecureSkipVerify}))
		fallthrough
	case "SASL_PLAINTEXT":
		mech, err := saslMechanism(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.SASL(mech))
	default:
		return nil, fmt.Errorf("unsupported AGENTOPS_SECURITY_PROTOCOL %q", cfg.AgentOpsSecurityProtocol)
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &kafkaClient{inner: client}, nil
}

func newOperatorClient(cfg collectorcfg.Config, clientID string) (operatorClient, error) {
	if len(cfg.AgentOpsBrokers) == 0 {
		return nil, errors.New("AGENTOPS_BROKERS is required when AGENTOPS_ENABLED=true")
	}
	opts := []kgo.Opt{kgo.SeedBrokers(cfg.AgentOpsBrokers...)}
	if strings.TrimSpace(clientID) != "" {
		opts = append(opts, kgo.ClientID(clientID))
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.AgentOpsSecurityProtocol)) {
	case "", "PLAINTEXT":
	case "SSL":
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: cfg.AgentOpsTLSInsecureSkipVerify}))
	case "SASL_SSL":
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: cfg.AgentOpsTLSInsecureSkipVerify}))
		fallthrough
	case "SASL_PLAINTEXT":
		mech, err := saslMechanism(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.SASL(mech))
	default:
		return nil, fmt.Errorf("unsupported AGENTOPS_SECURITY_PROTOCOL %q", cfg.AgentOpsSecurityProtocol)
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &kafkaOperatorClient{inner: client}, nil
}

func saslMechanism(cfg collectorcfg.Config) (sasl.Mechanism, error) {
	user := strings.TrimSpace(cfg.AgentOpsUsername)
	pass := strings.TrimSpace(cfg.AgentOpsPassword)
	if user == "" || pass == "" {
		return nil, errors.New("AGENTOPS_USERNAME and AGENTOPS_PASSWORD are required for SASL")
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.AgentOpsSASLMechanism)) {
	case "", "PLAIN":
		return plain.Auth{User: user, Pass: pass}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: user, Pass: pass}.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return scram.Auth{User: user, Pass: pass}.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("unsupported AGENTOPS_SASL_MECHANISM %q", cfg.AgentOpsSASLMechanism)
	}
}

func firstFatalError(fetches kgo.Fetches) error {
	if err := fetches.Err0(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	for _, fe := range fetches.Errors() {
		if errors.Is(fe.Err, context.Canceled) || errors.Is(fe.Err, context.DeadlineExceeded) {
			continue
		}
		return fe.Err
	}
	return nil
}

func recordTimestamp(rec *kgo.Record) string {
	if rec.Timestamp.IsZero() {
		return nowFunc().UTC().Format(time.RFC3339)
	}
	return rec.Timestamp.UTC().Format(time.RFC3339)
}

func rawMessage(recordID string, rec *kgo.Record, family string, now string) (store.Message, bool) {
	raw := strings.TrimSpace(string(rec.Value))
	if raw == "" || !utf8.Valid(rec.Value) || json.Valid(rec.Value) {
		return store.Message{}, false
	}
	switch raw[0] {
	case '{', '[', '"':
		return store.Message{}, false
	}
	msg := store.Message{
		ID:            recordID,
		Topic:         rec.Topic,
		TopicFamily:   family,
		Partition:     rec.Partition,
		Offset:        rec.Offset,
		Timestamp:     now,
		EnvelopeType:  "raw",
		CorrelationID: recordID,
		Preview:       previewForPayload(json.RawMessage(raw)),
		Content:       raw,
	}
	return msg, true
}

func previewForPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	raw := strings.TrimSpace(string(payload))
	if len(raw) > 180 {
		return raw[:180] + "..."
	}
	return raw
}

func compactJSON(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var anyValue any
	if err := json.Unmarshal(payload, &anyValue); err != nil {
		return string(payload)
	}
	data, err := json.Marshal(anyValue)
	if err != nil {
		return string(payload)
	}
	return string(data)
}

func appendUnique(values []string, next string) []string {
	next = strings.TrimSpace(next)
	if next == "" {
		return values
	}
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fallbackID(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func newReplayGroupID(prefix string, now time.Time) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "kafsiem-agentops-replay"
	}
	return fmt.Sprintf("%s-%s", prefix, now.UTC().Format("20060102t150405"))
}

func (s *Service) mirrorReject(ctx context.Context, client agentopsClient, rec *kgo.Record, reason string) (bool, error) {
	if strings.TrimSpace(s.cfg.AgentOpsRejectTopic) == "" {
		return false, errors.New("agentops reject topic not configured")
	}
	payload := map[string]any{
		"reason":       reason,
		"source_topic": rec.Topic,
		"partition":    rec.Partition,
		"offset":       rec.Offset,
		"timestamp":    recordTimestamp(rec),
		"raw_value":    string(rec.Value),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	produceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = client.ProduceSync(produceCtx, &kgo.Record{Topic: s.cfg.AgentOpsRejectTopic, Value: data})
	if err != nil {
		return false, err
	}
	return true, nil
}

func observedMessagesPerHour(count int, first, last string) float64 {
	if count <= 0 {
		return 0
	}
	firstTime, errFirst := time.Parse(time.RFC3339, first)
	lastTime, errLast := time.Parse(time.RFC3339, last)
	if errFirst != nil || errLast != nil || !lastTime.After(firstTime) {
		return float64(count)
	}
	hours := lastTime.Sub(firstTime).Hours()
	if hours < 1 {
		hours = 1
	}
	return float64(count) / hours
}

func densityBucket(messagesPerHour float64) string {
	switch {
	case messagesPerHour >= 100:
		return "high"
	case messagesPerHour >= 10:
		return "medium"
	case messagesPerHour > 0:
		return "low"
	default:
		return "idle"
	}
}

func topicIsStale(lastMessageAt string) bool {
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(lastMessageAt))
	if err != nil {
		return true
	}
	return nowFunc().UTC().Sub(last.UTC()) > 30*time.Minute
}

func replayGroupIDs(doc store.Snapshot) []string {
	out := make([]string, 0, len(doc.ReplaySessions))
	for _, item := range doc.ReplaySessions {
		if strings.TrimSpace(item.GroupID) != "" {
			out = append(out, item.GroupID)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Service) appendGraph(branch string, msg store.Message, payload json.RawMessage) error {
	return s.file.Apply(func(tx *sql.Tx) error {
		topicID := graph.EntityID("topic", msg.Topic)
		if err := graph.UpsertEntity(tx, graph.Entity{
			ID:          topicID,
			Type:        "topic",
			CanonicalID: msg.Topic,
			DisplayName: msg.Topic,
			FirstSeen:   msg.Timestamp,
			LastSeen:    msg.Timestamp,
			Attrs:       map[string]any{"family": msg.TopicFamily},
		}); err != nil {
			return err
		}
		correlationID := graph.EntityID("correlation", fallbackID(msg.CorrelationID, msg.ID))
		if err := graph.UpsertEntity(tx, graph.Entity{
			ID:          correlationID,
			Type:        "correlation",
			CanonicalID: fallbackID(msg.CorrelationID, msg.ID),
			DisplayName: fallbackID(msg.CorrelationID, msg.ID),
			FirstSeen:   msg.Timestamp,
			LastSeen:    msg.Timestamp,
		}); err != nil {
			return err
		}
		if err := s.appendGraphEdge(tx, branch, msg.Timestamp, msg.ID, "mentions", correlationID, topicID); err != nil {
			return err
		}
		if strings.TrimSpace(msg.SenderID) != "" {
			agentID := graph.EntityID("agent", msg.SenderID)
			if err := graph.UpsertEntity(tx, graph.Entity{
				ID:          agentID,
				Type:        "agent",
				CanonicalID: msg.SenderID,
				DisplayName: msg.SenderID,
				FirstSeen:   msg.Timestamp,
				LastSeen:    msg.Timestamp,
			}); err != nil {
				return err
			}
			if err := s.appendGraphEdge(tx, branch, msg.Timestamp, msg.ID, "member_of", agentID, correlationID); err != nil {
				return err
			}
		}
		switch branch {
		case "requests":
			return s.appendRequestGraph(tx, branch, msg, payload)
		case "responses":
			return s.appendResponseGraph(tx, branch, msg, payload)
		case "traces":
			return s.appendTraceGraph(tx, branch, msg, payload)
		case "tasks.status":
			return s.appendTaskStatusGraph(tx, branch, msg, payload)
		default:
			return nil
		}
	})
}

func (s *Service) appendRequestGraph(tx *sql.Tx, branch string, msg store.Message, payload json.RawMessage) error {
	var taskPayload contract.TaskRequestPayload
	if json.Unmarshal(payload, &taskPayload) != nil || strings.TrimSpace(taskPayload.TaskID) == "" {
		return nil
	}
	taskID := graph.EntityID("task", taskPayload.TaskID)
	if err := graph.UpsertEntity(tx, graph.Entity{
		ID:          taskID,
		Type:        "task",
		CanonicalID: taskPayload.TaskID,
		DisplayName: firstNonEmpty(taskPayload.Description, taskPayload.TaskID),
		FirstSeen:   msg.Timestamp,
		LastSeen:    msg.Timestamp,
	}); err != nil {
		return err
	}
	requester := firstNonEmpty(taskPayload.RequesterID, msg.SenderID)
	if requester != "" {
		agentID := graph.EntityID("agent", requester)
		if err := graph.UpsertEntity(tx, graph.Entity{
			ID:          agentID,
			Type:        "agent",
			CanonicalID: requester,
			DisplayName: requester,
			FirstSeen:   msg.Timestamp,
			LastSeen:    msg.Timestamp,
		}); err != nil {
			return err
		}
		if err := s.appendGraphEdge(tx, branch, msg.Timestamp, msg.ID, "sent", agentID, taskID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(taskPayload.ParentTaskID) != "" {
		parentID := graph.EntityID("task", taskPayload.ParentTaskID)
		if err := graph.UpsertEntity(tx, graph.Entity{
			ID:          parentID,
			Type:        "task",
			CanonicalID: taskPayload.ParentTaskID,
			DisplayName: taskPayload.ParentTaskID,
			FirstSeen:   msg.Timestamp,
			LastSeen:    msg.Timestamp,
		}); err != nil {
			return err
		}
		if err := s.appendGraphEdge(tx, branch, msg.Timestamp, msg.ID, "delegated_to", parentID, taskID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) appendResponseGraph(tx *sql.Tx, branch string, msg store.Message, payload json.RawMessage) error {
	var taskPayload contract.TaskResponsePayload
	if json.Unmarshal(payload, &taskPayload) != nil || strings.TrimSpace(taskPayload.TaskID) == "" {
		return nil
	}
	taskID := graph.EntityID("task", taskPayload.TaskID)
	if err := graph.UpsertEntity(tx, graph.Entity{
		ID:          taskID,
		Type:        "task",
		CanonicalID: taskPayload.TaskID,
		DisplayName: taskPayload.TaskID,
		FirstSeen:   msg.Timestamp,
		LastSeen:    msg.Timestamp,
		Attrs:       map[string]any{"status": taskPayload.Status},
	}); err != nil {
		return err
	}
	responder := firstNonEmpty(taskPayload.ResponderID, msg.SenderID)
	if responder == "" {
		return nil
	}
	agentID := graph.EntityID("agent", responder)
	if err := graph.UpsertEntity(tx, graph.Entity{
		ID:          agentID,
		Type:        "agent",
		CanonicalID: responder,
		DisplayName: responder,
		FirstSeen:   msg.Timestamp,
		LastSeen:    msg.Timestamp,
	}); err != nil {
		return err
	}
	if err := s.appendGraphEdge(tx, branch, msg.Timestamp, msg.ID, "responded", agentID, taskID); err != nil {
		return err
	}
	if isTerminalTaskStatus(taskPayload.Status) {
		return graph.CloseOpenEdges(tx, taskID, []string{"sent", "responded", "delegated_to"}, msg.Timestamp)
	}
	return nil
}

func (s *Service) appendTraceGraph(tx *sql.Tx, branch string, msg store.Message, payload json.RawMessage) error {
	var tracePayload contract.TracePayload
	if json.Unmarshal(payload, &tracePayload) != nil || strings.TrimSpace(tracePayload.TraceID) == "" || strings.TrimSpace(msg.SenderID) == "" {
		return nil
	}
	traceID := graph.EntityID("trace", tracePayload.TraceID)
	if err := graph.UpsertEntity(tx, graph.Entity{
		ID:          traceID,
		Type:        "trace",
		CanonicalID: tracePayload.TraceID,
		DisplayName: firstNonEmpty(tracePayload.Title, tracePayload.TraceID),
		FirstSeen:   msg.Timestamp,
		LastSeen:    msg.Timestamp,
		Attrs:       map[string]any{"span_type": tracePayload.SpanType},
	}); err != nil {
		return err
	}
	agentID := graph.EntityID("agent", msg.SenderID)
	if err := graph.UpsertEntity(tx, graph.Entity{
		ID:          agentID,
		Type:        "agent",
		CanonicalID: msg.SenderID,
		DisplayName: msg.SenderID,
		FirstSeen:   msg.Timestamp,
		LastSeen:    msg.Timestamp,
	}); err != nil {
		return err
	}
	return s.appendGraphEdge(tx, branch, msg.Timestamp, msg.ID, "spans", agentID, traceID)
}

func (s *Service) appendTaskStatusGraph(tx *sql.Tx, branch string, msg store.Message, payload json.RawMessage) error {
	var taskPayload contract.TaskStatusPayload
	if json.Unmarshal(payload, &taskPayload) != nil || strings.TrimSpace(taskPayload.TaskID) == "" {
		return nil
	}
	taskID := graph.EntityID("task", taskPayload.TaskID)
	if err := graph.UpsertEntity(tx, graph.Entity{
		ID:          taskID,
		Type:        "task",
		CanonicalID: taskPayload.TaskID,
		DisplayName: taskPayload.TaskID,
		FirstSeen:   msg.Timestamp,
		LastSeen:    msg.Timestamp,
		Attrs:       map[string]any{"status": taskPayload.Status, "summary": taskPayload.Summary},
	}); err != nil {
		return err
	}
	if isTerminalTaskStatus(taskPayload.Status) {
		return graph.CloseOpenEdges(tx, taskID, []string{"sent", "responded", "delegated_to"}, msg.Timestamp)
	}
	return nil
}

func (s *Service) appendGraphEdge(tx *sql.Tx, branch, producedAt, evidenceMsg, edgeType, srcID, dstID string) error {
	edgeID, inserted, err := graph.AppendEdge(tx, graph.Edge{
		SrcID:       srcID,
		DstID:       dstID,
		Type:        edgeType,
		ValidFrom:   producedAt,
		EvidenceMsg: evidenceMsg,
	})
	if err != nil {
		return err
	}
	decision := "existing"
	if inserted {
		decision = "inserted"
	}
	return graph.AppendProvenance(tx, graph.Provenance{
		SubjectKind: "edge",
		SubjectID:   fmt.Sprintf("%d", edgeID),
		Stage:       "graph",
		PolicyVer:   fmt.Sprintf("%d", s.policy.Version),
		Inputs:      map[string]any{"evidence_msg": evidenceMsg, "src_id": srcID, "dst_id": dstID, "type": edgeType},
		Decision:    decision,
		Reasons:     []string{branch},
		ProducedAt:  producedAt,
	})
}

func isTerminalTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func messageExists(messages []store.Message, id string) bool {
	for _, item := range messages {
		if item.ID == id {
			return true
		}
	}
	return false
}

func findFlow(flows []store.Flow, id string) *store.Flow {
	for i := range flows {
		if flows[i].ID == id {
			return &flows[i]
		}
	}
	return nil
}

func findTrace(traces []store.Trace, id string) *store.Trace {
	for i := range traces {
		if traces[i].ID == id {
			return &traces[i]
		}
	}
	return nil
}

func findTask(tasks []store.Task, id string) *store.Task {
	for i := range tasks {
		if tasks[i].ID == id {
			return &tasks[i]
		}
	}
	return nil
}

func rebuildTopicHealth(messages []store.Message) []store.TopicHealth {
	type topicState struct {
		count     int
		firstSeen string
		lastSeen  string
		agents    map[string]struct{}
	}
	stats := map[string]*topicState{}
	for _, msg := range messages {
		item, ok := stats[msg.Topic]
		if !ok {
			item = &topicState{firstSeen: msg.Timestamp, lastSeen: msg.Timestamp, agents: map[string]struct{}{}}
			stats[msg.Topic] = item
		}
		item.count++
		if item.firstSeen == "" || msg.Timestamp < item.firstSeen {
			item.firstSeen = msg.Timestamp
		}
		if item.lastSeen == "" || msg.Timestamp > item.lastSeen {
			item.lastSeen = msg.Timestamp
		}
		if strings.TrimSpace(msg.SenderID) != "" {
			item.agents[msg.SenderID] = struct{}{}
		}
	}
	out := make([]store.TopicHealth, 0, len(stats))
	for topic, item := range stats {
		messagesPerHour := observedMessagesPerHour(item.count, item.firstSeen, item.lastSeen)
		out = append(out, store.TopicHealth{
			Topic:           topic,
			MessagesPerHour: messagesPerHour,
			MessageDensity:  densityBucket(messagesPerHour),
			ActiveAgents:    len(item.agents),
			IsStale:         topicIsStale(item.lastSeen),
			LastMessageAt:   item.lastSeen,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Topic < out[j].Topic })
	return out
}

func setCurrentService(svc *Service) {
	currentMu.Lock()
	defer currentMu.Unlock()
	currentService = svc
}
