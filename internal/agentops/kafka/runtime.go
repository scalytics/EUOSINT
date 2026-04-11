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

	agentcfg "github.com/scalytics/euosint/internal/agentops/config"
	"github.com/scalytics/euosint/internal/agentops/contract"
	"github.com/scalytics/euosint/internal/agentops/store"
	collectorcfg "github.com/scalytics/euosint/internal/collector/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type Service struct {
	cfg           collectorcfg.Config
	policy        agentcfg.Policy
	topics        []string
	file          *store.FileStore
	clientFactory func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error)
	stateMu       sync.Mutex
	internal      state
}

type agentopsClient interface {
	PollFetches(context.Context) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	Close()
}

var (
	currentMu            sync.RWMutex
	currentService       *Service
	defaultClientFactory = func(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (agentopsClient, error) {
		return newClient(cfg, topics, groupID, clientID)
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
	fs, err := store.NewFileStore(cfg.AgentOpsOutputPath, doc)
	if err != nil {
		return fmt.Errorf("agentops store: %w", err)
	}
	svc := &Service{
		cfg:           cfg,
		policy:        policy,
		topics:        topics,
		file:          fs,
		clientFactory: defaultClientFactory,
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
				doc.Health.LastPollAt = time.Now().UTC().Format(time.RFC3339)
			})
			continue
		}

		toCommit := make([]*kgo.Record, 0, len(records))
		updated := false
		for _, rec := range records {
			reason, ok := s.handleRecord(rec)
			toCommit = append(toCommit, rec)
			updated = true
			if !ok {
				_ = s.file.Update(func(doc *store.Document) {
					doc.Health.RejectedCount++
					doc.Health.LastReject = reason
					doc.Health.RejectedByReason[reason]++
				})
			}
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
		healthTopics = append(healthTopics, store.TopicHealth{
			Topic:           name,
			MessagesPerHour: float64(item.Count),
			ActiveAgents:    len(item.Agents),
			IsStale:         len(item.Agents) == 0,
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
		doc.Health.AcceptedCount = len(messages)
		doc.Health.LastPollAt = time.Now().UTC().Format(time.RFC3339)
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

func (s *Service) startReplay(ctx context.Context) (store.ReplaySession, error) {
	session := store.ReplaySession{
		ID:        time.Now().UTC().Format("20060102T150405.000000000"),
		GroupID:   newReplayGroupID(s.cfg.AgentOpsReplayPrefix, time.Now().UTC()),
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.file.Update(func(doc *store.Document) {
		doc.ReplaySessions = append([]store.ReplaySession{session}, doc.ReplaySessions...)
		if len(doc.ReplaySessions) > 10 {
			doc.ReplaySessions = doc.ReplaySessions[:10]
		}
	}); err != nil {
		return store.ReplaySession{}, err
	}
	go s.runReplay(context.WithoutCancel(ctx), session)
	return session, nil
}

func (s *Service) runReplay(ctx context.Context, session store.ReplaySession) {
	clientFactory := s.clientFactory
	if clientFactory == nil {
		clientFactory = defaultClientFactory
	}
	client, err := clientFactory(s.cfg, s.topics, session.GroupID, s.cfg.AgentOpsClientID+"-replay")
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
		pollCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		fetches := client.PollFetches(pollCtx)
		cancel()
		if err := firstFatalError(fetches); err != nil {
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
	_ = s.file.Update(func(doc *store.Document) {
		for i := range doc.ReplaySessions {
			if doc.ReplaySessions[i].ID == id {
				doc.ReplaySessions[i].Status = status
				doc.ReplaySessions[i].MessageCount = count
				doc.ReplaySessions[i].FinishedAt = time.Now().UTC().Format(time.RFC3339)
				break
			}
		}
		if lastError != "" {
			doc.Health.LastReject = lastError
		}
	})
}

func newClient(cfg collectorcfg.Config, topics []string, groupID string, clientID string) (*kgo.Client, error) {
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
	return kgo.NewClient(opts...)
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
		return time.Now().UTC().Format(time.RFC3339)
	}
	return rec.Timestamp.UTC().Format(time.RFC3339)
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
		prefix = "euosint-agentops-replay"
	}
	return fmt.Sprintf("%s-%s", prefix, now.UTC().Format("20060102t150405"))
}

func setCurrentService(svc *Service) {
	currentMu.Lock()
	defer currentMu.Unlock()
	currentService = svc
}
