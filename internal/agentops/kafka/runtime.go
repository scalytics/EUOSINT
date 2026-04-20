// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"
	"crypto/tls"
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
	stateMu               sync.Mutex
	replayMu              sync.Mutex
	replayCancels         map[string]context.CancelFunc
	internal              state
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

type state struct {
	flows  map[string]*store.Flow
	traces map[string]*store.Trace
	tasks  map[string]*store.Task
	msgs   map[string]store.Message
	topic  map[string]*topicStat
}

type topicStat struct {
	Count          int
	Agents         map[string]struct{}
	LastMessageAt  string
	FirstMessageAt string
}

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
	doc := store.Document{
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
		replayCancels:         map[string]context.CancelFunc{},
		internal: state{
			flows:  map[string]*store.Flow{},
			traces: map[string]*store.Trace{},
			tasks:  map[string]*store.Task{},
			msgs:   map[string]store.Message{},
			topic:  map[string]*topicStat{},
		},
	}
	svc.bootstrapFromStore(fs.Snapshot())
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

	if err := s.file.Update(func(doc *store.Document) {
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

		pollCtx, cancel := context.WithTimeout(ctx, time.Duration(max(250, s.cfg.KafkaPollTimeoutMS))*time.Millisecond)
		fetches := client.PollFetches(pollCtx)
		cancel()
		if err := firstFatalError(fetches); err != nil {
			_ = s.file.Update(func(doc *store.Document) {
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
			_ = s.file.Update(func(doc *store.Document) {
				doc.Health.Connected = true
				doc.Health.LastPollAt = nowFunc().UTC().Format(time.RFC3339)
			})
			continue
		}

		toCommit := make([]*kgo.Record, 0, len(records))
		updated := false
		for _, rec := range records {
			reason, ok := s.handleRecord(rec)
			toCommit = append(toCommit, rec)
			updated = true
			if ok {
				_ = s.file.Update(func(doc *store.Document) {
					doc.Health.AcceptedCount++
				})
				continue
			}
			mirrored, mirrorErr := s.mirrorReject(ctx, client, rec, reason)
			_ = s.file.Update(func(doc *store.Document) {
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
		if updated {
			if err := s.persist(); err != nil {
				return err
			}
		}
	}
}

func (s *Service) bootstrapFromStore(doc store.Document) {
	for i := range doc.Flows {
		item := doc.Flows[i]
		s.internal.flows[item.ID] = &item
	}
	for i := range doc.Traces {
		item := doc.Traces[i]
		s.internal.traces[item.ID] = &item
	}
	for i := range doc.Tasks {
		item := doc.Tasks[i]
		s.internal.tasks[item.ID] = &item
	}
	for _, item := range doc.Messages {
		s.internal.msgs[item.ID] = item
	}
}

func (s *Service) handleRecord(rec *kgo.Record) (string, bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	recordID := fmt.Sprintf("%s:%d:%d", rec.Topic, rec.Partition, rec.Offset)
	if _, exists := s.internal.msgs[recordID]; exists {
		return "", true
	}

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
		s.internal.msgs[msg.ID] = msg
		s.updateTopicStats(msg.Topic, "", now)
		return "", true
	}

	env, err := contract.DecodeEnvelope(rec.Value)
	if err != nil {
		if msg, ok := rawMessage(recordID, rec, family, now); ok {
			s.updateFlow(msg)
			s.updateTopicStats(msg.Topic, msg.SenderID, now)
			s.internal.msgs[msg.ID] = msg
			return "", true
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
			s.updateTask(now, payload.TaskID, payload.ParentTaskID, payload.RequesterID, "", payload.OriginalRequesterID, "", payload.Description, "")
		}
	case "responses":
		var payload contract.TaskResponsePayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TaskID = payload.TaskID
			msg.Status = payload.Status
			msg.Preview = firstNonEmpty(payload.Content, msg.Preview)
			s.updateTask(now, payload.TaskID, "", "", payload.ResponderID, "", payload.Status, "", payload.Content)
		}
	case "tasks.status":
		var payload contract.TaskStatusPayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TaskID = payload.TaskID
			msg.Status = payload.Status
			msg.Preview = firstNonEmpty(payload.Summary, payload.Status, msg.Preview)
			s.updateTask(now, payload.TaskID, "", "", payload.ResponderID, "", payload.Status, "", payload.Summary)
		}
	case "traces":
		var payload contract.TracePayload
		if json.Unmarshal(env.Payload, &payload) == nil {
			msg.TraceID = payload.TraceID
			msg.Preview = firstNonEmpty(payload.Title, payload.Content, msg.Preview)
			s.updateTrace(now, payload, env.SenderID)
		}
	case "observe.audit":
		msg.Preview = firstNonEmpty(previewForPayload(env.Payload), "audit event")
	}

	s.updateFlow(msg)
	s.updateTopicStats(msg.Topic, msg.SenderID, now)
	s.internal.msgs[msg.ID] = msg
	return "", true
}

func (s *Service) updateFlow(msg store.Message) {
	flowID := fallbackID(msg.CorrelationID, msg.ID)
	item, ok := s.internal.flows[flowID]
	if !ok {
		item = &store.Flow{ID: flowID, FirstSeen: msg.Timestamp}
		s.internal.flows[flowID] = item
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
}

func (s *Service) updateTrace(now string, payload contract.TracePayload, senderID string) {
	if strings.TrimSpace(payload.TraceID) == "" {
		return
	}
	item, ok := s.internal.traces[payload.TraceID]
	if !ok {
		item = &store.Trace{ID: payload.TraceID, StartedAt: now}
		s.internal.traces[payload.TraceID] = item
	}
	item.SpanCount++
	item.Agents = appendUnique(item.Agents, senderID)
	item.SpanTypes = appendUnique(item.SpanTypes, payload.SpanType)
	item.LatestTitle = firstNonEmpty(payload.Title, item.LatestTitle)
	if payload.StartedAt != "" {
		item.StartedAt = payload.StartedAt
	}
	if payload.EndedAt != "" {
		item.EndedAt = payload.EndedAt
	}
	if payload.DurationMs > 0 {
		item.DurationMs = payload.DurationMs
	}
}

func (s *Service) updateTask(now, taskID, parentID, requesterID, responderID, originalRequesterID, status, description, summary string) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	item, ok := s.internal.tasks[taskID]
	if !ok {
		item = &store.Task{ID: taskID, FirstSeen: now}
		s.internal.tasks[taskID] = item
	}
	item.LastSeen = now
	item.ParentTaskID = firstNonEmpty(parentID, item.ParentTaskID)
	item.RequesterID = firstNonEmpty(requesterID, item.RequesterID)
	item.ResponderID = firstNonEmpty(responderID, item.ResponderID)
	item.OriginalRequesterID = firstNonEmpty(originalRequesterID, item.OriginalRequesterID)
	item.Status = firstNonEmpty(status, item.Status)
	item.Description = firstNonEmpty(description, item.Description)
	item.LastSummary = firstNonEmpty(summary, item.LastSummary)
}

func (s *Service) updateTopicStats(topic, senderID, timestamp string) {
	item, ok := s.internal.topic[topic]
	if !ok {
		item = &topicStat{Agents: map[string]struct{}{}, FirstMessageAt: timestamp}
		s.internal.topic[topic] = item
	}
	item.Count++
	if senderID != "" {
		item.Agents[senderID] = struct{}{}
	}
	item.LastMessageAt = timestamp
}

func (s *Service) persist() error {
	s.stateMu.Lock()
	flows := make([]store.Flow, 0, len(s.internal.flows))
	for _, item := range s.internal.flows {
		flows = append(flows, *item)
	}
	traces := make([]store.Trace, 0, len(s.internal.traces))
	for _, item := range s.internal.traces {
		traces = append(traces, *item)
	}
	tasks := make([]store.Task, 0, len(s.internal.tasks))
	for _, item := range s.internal.tasks {
		tasks = append(tasks, *item)
	}
	messages := make([]store.Message, 0, len(s.internal.msgs))
	for _, item := range s.internal.msgs {
		messages = append(messages, item)
	}
	healthTopics := make([]store.TopicHealth, 0, len(s.internal.topic))
	for name, item := range s.internal.topic {
		messagesPerHour := observedMessagesPerHour(item.Count, item.FirstMessageAt, item.LastMessageAt)
		healthTopics = append(healthTopics, store.TopicHealth{
			Topic:           name,
			MessagesPerHour: messagesPerHour,
			MessageDensity:  densityBucket(messagesPerHour),
			ActiveAgents:    len(item.Agents),
			IsStale:         topicIsStale(item.LastMessageAt),
			LastMessageAt:   item.LastMessageAt,
		})
	}
	s.stateMu.Unlock()

	sort.Slice(messages, func(i, j int) bool { return messages[i].Timestamp > messages[j].Timestamp })
	if limit := s.policy.Grouping.ReplayMaxRecords; limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}

	return s.file.Update(func(doc *store.Document) {
		doc.Enabled = true
		doc.UIMode = s.cfg.UIMode
		doc.Profile = s.cfg.Profile
		doc.GroupName = s.cfg.AgentOpsGroupName
		doc.Topics = append([]string{}, s.topics...)
		doc.Flows = flows
		doc.Traces = traces
		doc.Tasks = tasks
		doc.Messages = messages
		doc.FlowCount = len(flows)
		doc.TraceCount = len(traces)
		doc.TaskCount = len(tasks)
		doc.MessageCount = len(messages)
		doc.Health.Connected = true
		doc.Health.GroupID = s.cfg.AgentOpsGroupID
		doc.Health.EffectiveTopics = append([]string{}, s.topics...)
		doc.Health.LastPollAt = nowFunc().UTC().Format(time.RFC3339)
		doc.Health.TopicHealth = healthTopics
		if doc.Health.RejectedByReason == nil {
			doc.Health.RejectedByReason = map[string]int{}
		}
	})
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
	if err := s.file.Update(func(doc *store.Document) {
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
		_ = s.persist()
	}
	s.finishReplay(session.ID, processed, "completed", "")
}

func (s *Service) finishReplay(id string, count int, status string, lastError string) {
	s.replayMu.Lock()
	delete(s.replayCancels, id)
	active := len(s.replayCancels)
	s.replayMu.Unlock()

	_ = s.file.Update(func(doc *store.Document) {
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

func replayGroupIDs(doc store.Document) []string {
	out := make([]string, 0, len(doc.ReplaySessions))
	for _, item := range doc.ReplaySessions {
		if strings.TrimSpace(item.GroupID) != "" {
			out = append(out, item.GroupID)
		}
	}
	sort.Strings(out)
	return out
}

func setCurrentService(svc *Service) {
	currentMu.Lock()
	defer currentMu.Unlock()
	currentService = svc
}
