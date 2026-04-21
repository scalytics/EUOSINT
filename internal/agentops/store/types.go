// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package store

type Snapshot struct {
	GeneratedAt    string          `json:"generated_at"`
	Enabled        bool            `json:"enabled"`
	UIMode         string          `json:"ui_mode"`
	Profile        string          `json:"profile"`
	GroupName      string          `json:"group_name"`
	Topics         []string        `json:"topics"`
	FlowCount      int             `json:"flow_count"`
	TraceCount     int             `json:"trace_count"`
	TaskCount      int             `json:"task_count"`
	MessageCount   int             `json:"message_count"`
	Health         Health          `json:"health"`
	ReplaySessions []ReplaySession `json:"replay_sessions"`
	Flows          []Flow          `json:"flows"`
	Traces         []Trace         `json:"traces"`
	Tasks          []Task          `json:"tasks"`
	Messages       []Message       `json:"messages"`
}

type Health struct {
	Connected             bool           `json:"connected"`
	EffectiveTopics       []string       `json:"effective_topics"`
	GroupID               string         `json:"group_id"`
	AcceptedCount         int            `json:"accepted_count"`
	RejectedCount         int            `json:"rejected_count"`
	MirroredCount         int            `json:"mirrored_count"`
	MirrorFailedCount     int            `json:"mirror_failed_count"`
	RejectedByReason      map[string]int `json:"rejected_by_reason"`
	LastReject            string         `json:"last_reject,omitempty"`
	LastMirrorError       string         `json:"last_mirror_error,omitempty"`
	LastPollAt            string         `json:"last_poll_at,omitempty"`
	ReplayStatus          string         `json:"replay_status,omitempty"`
	ReplayActive          int            `json:"replay_active"`
	ReplayLastError       string         `json:"replay_last_error,omitempty"`
	ReplayLastFinishedAt  string         `json:"replay_last_finished_at,omitempty"`
	ReplayLastRecordCount int            `json:"replay_last_record_count"`
	TopicHealth           []TopicHealth  `json:"topic_health"`
}

type ReplaySession struct {
	ID           string   `json:"id"`
	GroupID      string   `json:"group_id"`
	Status       string   `json:"status"`
	StartedAt    string   `json:"started_at"`
	FinishedAt   string   `json:"finished_at,omitempty"`
	MessageCount int      `json:"message_count"`
	Topics       []string `json:"topics,omitempty"`
	LastError    string   `json:"last_error,omitempty"`
}

type ConsumerGroupMember struct {
	MemberID   string `json:"member_id"`
	ClientID   string `json:"client_id"`
	ClientHost string `json:"client_host"`
	InstanceID string `json:"instance_id,omitempty"`
}

type ConsumerGroup struct {
	GroupID      string                `json:"group_id"`
	State        string                `json:"state"`
	ProtocolType string                `json:"protocol_type"`
	Protocol     string                `json:"protocol"`
	Members      []ConsumerGroupMember `json:"members"`
}

type OperatorState struct {
	Supported      bool            `json:"supported"`
	LiveGroupID    string          `json:"live_group_id,omitempty"`
	ReplayGroupIDs []string        `json:"replay_group_ids"`
	Groups         []ConsumerGroup `json:"groups"`
	LastError      string          `json:"last_error,omitempty"`
}

type Flow struct {
	ID            string   `json:"id"`
	TopicCount    int      `json:"topic_count"`
	SenderCount   int      `json:"sender_count"`
	Topics        []string `json:"topics"`
	Senders       []string `json:"senders"`
	TraceIDs      []string `json:"trace_ids"`
	TaskIDs       []string `json:"task_ids"`
	FirstSeen     string   `json:"first_seen"`
	LastSeen      string   `json:"last_seen"`
	LatestStatus  string   `json:"latest_status,omitempty"`
	MessageCount  int      `json:"message_count"`
	LatestPreview string   `json:"latest_preview,omitempty"`
}

type Trace struct {
	ID          string   `json:"id"`
	SpanCount   int      `json:"span_count"`
	Agents      []string `json:"agents"`
	SpanTypes   []string `json:"span_types"`
	LatestTitle string   `json:"latest_title,omitempty"`
	StartedAt   string   `json:"started_at,omitempty"`
	EndedAt     string   `json:"ended_at,omitempty"`
	DurationMs  int      `json:"duration_ms,omitempty"`
}

type Task struct {
	ID                  string `json:"id"`
	ParentTaskID        string `json:"parent_task_id,omitempty"`
	DelegationDepth     int    `json:"delegation_depth,omitempty"`
	RequesterID         string `json:"requester_id,omitempty"`
	ResponderID         string `json:"responder_id,omitempty"`
	OriginalRequesterID string `json:"original_requester_id,omitempty"`
	Status              string `json:"status,omitempty"`
	Description         string `json:"description,omitempty"`
	LastSummary         string `json:"last_summary,omitempty"`
	FirstSeen           string `json:"first_seen"`
	LastSeen            string `json:"last_seen"`
}

type Message struct {
	ID            string      `json:"id"`
	Topic         string      `json:"topic"`
	TopicFamily   string      `json:"topic_family"`
	Partition     int32       `json:"partition"`
	Offset        int64       `json:"offset"`
	Timestamp     string      `json:"timestamp"`
	EnvelopeType  string      `json:"envelope_type,omitempty"`
	SenderID      string      `json:"sender_id,omitempty"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	TraceID       string      `json:"trace_id,omitempty"`
	TaskID        string      `json:"task_id,omitempty"`
	ParentTaskID  string      `json:"parent_task_id,omitempty"`
	Status        string      `json:"status,omitempty"`
	Preview       string      `json:"preview,omitempty"`
	Content       string      `json:"content,omitempty"`
	LFS           *LFSPointer `json:"lfs,omitempty"`
}

type LFSPointer struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	ProxyID     string `json:"proxy_id,omitempty"`
	Path        string `json:"path"`
}

type TopicHealth struct {
	Topic           string  `json:"topic"`
	MessagesPerHour float64 `json:"messages_per_hour"`
	MessageDensity  string  `json:"message_density"`
	ActiveAgents    int     `json:"active_agents"`
	IsStale         bool    `json:"is_stale"`
	LastMessageAt   string  `json:"last_message_at,omitempty"`
}
