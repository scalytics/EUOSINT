// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package contract

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type GroupEnvelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	SenderID      string          `json:"sender_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
}

type TaskRequestPayload struct {
	TaskID              string `json:"task_id"`
	Description         string `json:"description"`
	Content             string `json:"content"`
	RequesterID         string `json:"requester_id"`
	ParentTaskID        string `json:"parent_task_id,omitempty"`
	DelegationDepth     int    `json:"delegation_depth,omitempty"`
	OriginalRequesterID string `json:"original_requester_id,omitempty"`
	DeadlineAt          string `json:"deadline_at,omitempty"`
}

type TaskResponsePayload struct {
	TaskID      string `json:"task_id"`
	ResponderID string `json:"responder_id"`
	Content     string `json:"content"`
	Status      string `json:"status"`
}

type TaskStatusPayload struct {
	TaskID      string `json:"task_id"`
	ResponderID string `json:"responder_id"`
	Status      string `json:"status"`
	Summary     string `json:"summary,omitempty"`
}

type TracePayload struct {
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
	SpanType     string `json:"span_type"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	DurationMs   int    `json:"duration_ms"`
}

type LFSPointer struct {
	Version     int               `json:"kfs_lfs"`
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	Size        int64             `json:"size"`
	SHA256      string            `json:"sha256"`
	ContentType string            `json:"content_type,omitempty"`
	CreatedAt   string            `json:"created_at,omitempty"`
	ProxyID     string            `json:"proxy_id,omitempty"`
	Headers     map[string]string `json:"original_headers,omitempty"`
}

func DecodeEnvelope(value []byte) (GroupEnvelope, error) {
	var env GroupEnvelope
	if err := json.Unmarshal(value, &env); err != nil {
		return GroupEnvelope{}, err
	}
	if strings.TrimSpace(env.Type) == "" {
		return GroupEnvelope{}, fmt.Errorf("missing envelope type")
	}
	return env, nil
}

func DecodeLFSPointer(value []byte) (LFSPointer, bool, error) {
	var ptr LFSPointer
	if err := json.Unmarshal(value, &ptr); err != nil {
		return LFSPointer{}, false, nil
	}
	if ptr.Version == 0 || strings.TrimSpace(ptr.Bucket) == "" || strings.TrimSpace(ptr.Key) == "" {
		return LFSPointer{}, false, nil
	}
	return ptr, true, nil
}
