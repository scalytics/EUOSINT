// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package translate

import (
	"context"
	"testing"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/parse"
	"github.com/scalytics/euosint/internal/collector/vet"
)

type fakeCompleter struct {
	content string
	err     error
}

func (f fakeCompleter) Complete(ctx context.Context, messages []vet.Message) (string, error) {
	return f.content, f.err
}

func TestDecodeAlertLLMResponse(t *testing.T) {
	got, err := decodeAlertLLMResponse("```json\n{\"yes\":true,\"translation\":\"Wanted suspect in Berlin\",\"category_id\":\"wanted_suspect\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Yes || got.Translation != "Wanted suspect in Berlin" || got.CategoryID != "wanted_suspect" {
		t.Fatalf("unexpected response %#v", got)
	}
}

func TestBatchLLMFiltersAndOverridesCategory(t *testing.T) {
	cfg := config.Default()
	cfg.AlertLLMModel = "gpt-test"
	items := []parse.FeedItem{{Title: "Titulo", Link: "https://example.test/a"}}
	classified, err := BatchLLM(context.Background(), cfg, fakeCompleter{content: `{"yes":true,"translation":"Missing child in Madrid","category_id":"missing_person"}`}, "public_appeal", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(classified) != 1 {
		t.Fatalf("expected 1 classified item, got %d", len(classified))
	}
	if classified[0].Item.Title != "Missing child in Madrid" || classified[0].Category != "missing_person" {
		t.Fatalf("unexpected classified item %#v", classified[0])
	}
}

func TestBatchLLMDropsNoise(t *testing.T) {
	cfg := config.Default()
	items := []parse.FeedItem{{Title: "General update", Link: "https://example.test/a"}}
	classified, err := BatchLLM(context.Background(), cfg, fakeCompleter{content: `{"yes":false,"translation":"","category_id":""}`}, "public_appeal", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(classified) != 0 {
		t.Fatalf("expected no classified items, got %d", len(classified))
	}
}
