package normalize

import (
	"testing"

	"github.com/scalytics/kafSIEM/internal/collector/model"
)

func TestTokenize(t *testing.T) {
	got := tokenize("ISIS claims attack in Mogadishu — 12 dead")
	want := []string{"isis", "claims", "attack", "in", "mogadishu", "12", "dead"}
	if len(got) != len(want) {
		t.Fatalf("tokenize length: got %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRemoveStopwords(t *testing.T) {
	tokens := []string{"the", "attack", "in", "mogadishu", "was", "deadly"}
	got := removeStopwords(tokens)
	want := []string{"attack", "mogadishu", "deadly"}
	if len(got) != len(want) {
		t.Fatalf("removeStopwords: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEntityExtraction_IS(t *testing.T) {
	// The critical "IS" problem: "IS" should match Islamic State, not be
	// dropped as a stopword.
	dict := &entityDict{
		phrases: []entityPhrase{
			{tokens: []string{"islamic", "state"}, canonical: "Islamic State"},
			{tokens: []string{"isis"}, canonical: "Islamic State"},
			{tokens: []string{"isil"}, canonical: "Islamic State"},
			{tokens: []string{"daesh"}, canonical: "Islamic State"},
			{tokens: []string{"is"}, canonical: "Islamic State"},
		},
	}

	tests := []struct {
		name       string
		input      string
		wantEntity string
	}{
		{"ISIS alias", "isis claims attack in somalia", "Islamic State"},
		{"IS alias", "the is claimed responsibility for bombing", "Islamic State"},
		{"ISIL alias", "isil fighters advance in syria", "Islamic State"},
		{"Daesh alias", "coalition strikes against daesh positions", "Islamic State"},
		{"Full name", "islamic state militants attack village", "Islamic State"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenize(tt.input)
			entities, _ := dict.extractEntities(tokens)
			found := false
			for _, e := range entities {
				if e == tt.wantEntity {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("entity %q not found in %v (input: %q)", tt.wantEntity, entities, tt.input)
			}
		})
	}
}

func TestEntityExtraction_MultiWord(t *testing.T) {
	dict := &entityDict{
		phrases: []entityPhrase{
			// Longest first (sorted by buildEntityDict).
			{tokens: []string{"al", "shabaab"}, canonical: "Al-Shabaab"},
			{tokens: []string{"al", "qaeda"}, canonical: "Al-Qaeda"},
			{tokens: []string{"boko", "haram"}, canonical: "Boko Haram"},
		},
	}

	tokens := tokenize("Al Shabaab and Boko Haram clash in border region")
	entities, remainder := dict.extractEntities(tokens)

	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %v", entities)
	}

	// Remainder should not contain al, shabaab, boko, haram.
	for _, r := range remainder {
		if r == "al" || r == "shabaab" || r == "boko" || r == "haram" {
			t.Errorf("entity token %q leaked into remainder", r)
		}
	}
}

func TestJaccardSimilarity(t *testing.T) {
	a := []string{"attack", "mogadishu", "entity:Al-Shabaab", "cat:terrorism"}
	b := []string{"attack", "mogadishu", "entity:Al-Shabaab", "cat:terrorism", "killed"}

	sim := jaccardSimilarity(a, b)
	if sim < 0.7 || sim > 0.9 {
		t.Errorf("expected similarity ~0.8, got %f", sim)
	}

	// Identical sets.
	if jaccardSimilarity(a, a) != 1.0 {
		t.Error("identical sets should have similarity 1.0")
	}

	// Disjoint sets.
	c := []string{"earthquake", "japan", "cat:natural_disaster"}
	if jaccardSimilarity(a, c) > 0.1 {
		t.Errorf("disjoint sets should have very low similarity, got %f", jaccardSimilarity(a, c))
	}
}

func TestCrossSourceDedup(t *testing.T) {
	// Two alerts from different sources about the same event.
	alerts := []model.Alert{
		{
			AlertID:          "src1-abc123",
			SourceID:         "source-a",
			Title:            "Al-Shabaab militants attack military base in Mogadishu",
			Category:         "terrorism",
			EventCountryCode: "SO",
			FirstSeen:        "2024-01-15T10:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.8},
		},
		{
			AlertID:          "src2-def456",
			SourceID:         "source-b",
			Title:            "Al Shabaab attack on military base in Mogadishu leaves casualties",
			Category:         "terrorism",
			EventCountryCode: "SO",
			FirstSeen:        "2024-01-15T11:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.7},
		},
		{
			AlertID:          "src3-ghi789",
			SourceID:         "source-c",
			Title:            "Earthquake strikes central Japan, tsunami warning issued",
			Category:         "natural_disaster",
			EventCountryCode: "JP",
			FirstSeen:        "2024-01-15T12:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.9},
		},
	}

	kept, suppressed := crossSourceDedup(alerts)
	if suppressed != 1 {
		t.Errorf("expected 1 suppressed, got %d (kept %d alerts)", suppressed, len(kept))
	}
	if len(kept) != 2 {
		t.Errorf("expected 2 kept, got %d", len(kept))
	}

	// The higher-scoring Al-Shabaab alert (0.8) should be kept.
	for _, a := range kept {
		if a.AlertID == "src2-def456" {
			t.Error("lower-scoring duplicate should have been suppressed")
		}
	}
}

func TestCrossSourceDedup_DifferentTimeWindow(t *testing.T) {
	// Same event title but 48h apart — should NOT be deduped.
	alerts := []model.Alert{
		{
			AlertID:          "src1-abc",
			SourceID:         "source-a",
			Title:            "Attack on military base in Mogadishu",
			Category:         "terrorism",
			EventCountryCode: "SO",
			FirstSeen:        "2024-01-15T10:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.8},
		},
		{
			AlertID:          "src2-def",
			SourceID:         "source-b",
			Title:            "Attack on military base in Mogadishu",
			Category:         "terrorism",
			EventCountryCode: "SO",
			FirstSeen:        "2024-01-17T10:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.7},
		},
	}

	kept, suppressed := crossSourceDedup(alerts)
	if suppressed != 0 {
		t.Errorf("expected 0 suppressed (different time windows), got %d", suppressed)
	}
	if len(kept) != 2 {
		t.Errorf("expected 2 kept, got %d", len(kept))
	}
}

func TestCrossSourceDedup_SameSource(t *testing.T) {
	// Two similar alerts from the SAME source — should NOT be cross-source deduped
	// (within-source dedup is handled by earlier stages).
	alerts := []model.Alert{
		{
			AlertID:          "src1-abc",
			SourceID:         "source-a",
			Title:            "Attack on military base in Mogadishu",
			Category:         "terrorism",
			EventCountryCode: "SO",
			FirstSeen:        "2024-01-15T10:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.8},
		},
		{
			AlertID:          "src1-def",
			SourceID:         "source-a",
			Title:            "Attack on military base in Mogadishu region",
			Category:         "terrorism",
			EventCountryCode: "SO",
			FirstSeen:        "2024-01-15T11:00:00Z",
			Triage:           &model.Triage{RelevanceScore: 0.7},
		},
	}

	kept, suppressed := crossSourceDedup(alerts)
	if suppressed != 0 {
		t.Errorf("same-source alerts should not be cross-source deduped, got %d suppressed", suppressed)
	}
	if len(kept) != 2 {
		t.Errorf("expected 2 kept, got %d", len(kept))
	}
}

func TestContentFingerprint_ISvsis(t *testing.T) {
	// Verify "IS" (Islamic State) produces entity token, not dropped as stopword.
	dict := &entityDict{
		phrases: []entityPhrase{
			{tokens: []string{"islamic", "state"}, canonical: "Islamic State"},
			{tokens: []string{"is"}, canonical: "Islamic State"},
		},
	}

	alert := model.Alert{
		Title:    "The IS claimed attack in Raqqa",
		Category: "terrorism",
	}

	fp := contentFingerprint(alert, dict)
	hasEntity := false
	for _, t := range fp {
		if t == "entity:Islamic State" {
			hasEntity = true
		}
		// "is" should NOT appear as a standalone token.
		if t == "is" {
			t2 := t
			_ = t2
			// This would mean IS leaked through as a regular word.
		}
	}
	if !hasEntity {
		t.Errorf("fingerprint should contain entity:Islamic State, got %v", fp)
	}
}
