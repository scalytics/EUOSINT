// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package vet

import (
	"testing"

	"github.com/scalytics/euosint/internal/collector/parse"
)

func TestDecodeVerdictExtractsJSONBlock(t *testing.T) {
	verdict, err := decodeVerdict("```json\n{\"approve\":true,\"promotion_status\":\"active\",\"level\":\"federal\",\"source_quality\":0.9,\"operational_relevance\":0.8,\"mission_tags\":[\"organized_crime\"],\"reason\":\"high signal\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	verdict.normalize()
	if !verdict.Approve || verdict.PromotionStatus != "active" || verdict.Level != "federal" {
		t.Fatalf("unexpected verdict %#v", verdict)
	}
}

func TestDecodeVerdictAcceptsNumericStrings(t *testing.T) {
	verdict, err := decodeVerdict(`{"approve":true,"promotion_status":"active","level":"national","source_quality":"0.9","operational_relevance":"0.8","mission_tags":["organized_crime"],"reason":"high signal"}`)
	if err != nil {
		t.Fatal(err)
	}
	verdict.normalize()
	if float64(verdict.SourceQuality) != 0.9 {
		t.Fatalf("expected source_quality 0.9, got %v", verdict.SourceQuality)
	}
	if float64(verdict.OperationalRelevance) != 0.8 {
		t.Fatalf("expected operational_relevance 0.8, got %v", verdict.OperationalRelevance)
	}
}

func TestDecodeVerdictAcceptsQualitativeScores(t *testing.T) {
	verdict, err := decodeVerdict(`{"approve":true,"promotion_status":"active","level":"national","source_quality":"low","operational_relevance":"medium","mission_tags":["organized_crime"],"reason":"high signal"}`)
	if err != nil {
		t.Fatal(err)
	}
	verdict.normalize()
	if float64(verdict.SourceQuality) != 0.25 {
		t.Fatalf("expected source_quality 0.25, got %v", verdict.SourceQuality)
	}
	if float64(verdict.OperationalRelevance) != 0.5 {
		t.Fatalf("expected operational_relevance 0.5, got %v", verdict.OperationalRelevance)
	}
}

func TestDecodeVerdictAcceptsPercentScores(t *testing.T) {
	verdict, err := decodeVerdict(`{"approve":true,"promotion_status":"active","level":"national","source_quality":"80%","operational_relevance":"50%","mission_tags":["organized_crime"],"reason":"high signal"}`)
	if err != nil {
		t.Fatal(err)
	}
	verdict.normalize()
	if float64(verdict.SourceQuality) != 0.8 {
		t.Fatalf("expected source_quality 0.8, got %v", verdict.SourceQuality)
	}
	if float64(verdict.OperationalRelevance) != 0.5 {
		t.Fatalf("expected operational_relevance 0.5, got %v", verdict.OperationalRelevance)
	}
}

func TestDecodeVerdictAcceptsNoneScores(t *testing.T) {
	verdict, err := decodeVerdict(`{"approve":true,"promotion_status":"active","level":"national","source_quality":"none","operational_relevance":"n/a","mission_tags":["organized_crime"],"reason":"no numeric score"}`)
	if err != nil {
		t.Fatal(err)
	}
	verdict.normalize()
	if float64(verdict.SourceQuality) != 0 {
		t.Fatalf("expected source_quality 0, got %v", verdict.SourceQuality)
	}
	if float64(verdict.OperationalRelevance) != 0 {
		t.Fatalf("expected operational_relevance 0, got %v", verdict.OperationalRelevance)
	}
}

func TestDeterministicRejectsMissingSamplesOnly(t *testing.T) {
	if _, reject := deterministicReject(Input{AuthorityName: "City of Valletta Police Department", Samples: []Sample{{Title: "x"}}}); reject {
		t.Fatal("expected local police to be evaluated by the model, not deterministically rejected")
	}
	if _, reject := deterministicReject(Input{AuthorityName: "Europol", Samples: nil}); !reject {
		t.Fatal("expected no-sample deterministic reject")
	}
}

func TestSamplesFromFeedItemsHonorsLimit(t *testing.T) {
	items := []parse.FeedItem{
		{Title: "One", Link: "https://one"},
		{Title: "Two", Link: "https://two"},
	}
	samples := SamplesFromFeedItems(items, 1)
	if len(samples) != 1 || samples[0].Title != "One" {
		t.Fatalf("unexpected samples %#v", samples)
	}
}
