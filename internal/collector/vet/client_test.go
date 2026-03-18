// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package vet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scalytics/euosint/internal/collector/config"
)

func TestClientCompleteUsesOpenAICompatibleEndpoint(t *testing.T) {
	var gotAuth string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = payload["model"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"approve":true}`}},
			},
		})
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.VettingBaseURL = server.URL + "/v1"
	cfg.VettingAPIKey = "secret"
	cfg.VettingModel = "gpt-test"
	client := NewClient(cfg)
	content, err := client.Complete(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if gotModel != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %q", gotModel)
	}
	if content != `{"approve":true}` {
		t.Fatalf("unexpected content %q", content)
	}
}

func TestCompletionsURLNormalizesBase(t *testing.T) {
	if got := completionsURL("http://localhost:11434/v1"); got != "http://localhost:11434/v1/chat/completions" {
		t.Fatalf("unexpected ollama/vllm url %q", got)
	}
	if got := completionsURL("https://gateway.example/openai/v1/chat/completions"); got != "https://gateway.example/openai/v1/chat/completions" {
		t.Fatalf("unexpected passthrough url %q", got)
	}
}
