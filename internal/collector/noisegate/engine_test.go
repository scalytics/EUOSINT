package noisegate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
)

func TestLoadPolicyAndVersion(t *testing.T) {
	path := filepath.Join("..", "..", "..", "registry", "noise_policy.json")
	engine, err := Load(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if engine == nil {
		t.Fatal("expected engine")
	}
	if engine.Version() == "" {
		t.Fatal("expected policy version")
	}
}

func TestEvaluateKeepsSexualAssaultPoliceNotice(t *testing.T) {
	path := filepath.Join("..", "..", "..", "registry", "noise_policy.json")
	engine, err := Load(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	decision := engine.Evaluate(model.RegistrySource{
		Category: "public_appeal",
		Source:   model.SourceMetadata{AuthorityType: "police"},
	}, parse.FeedItem{
		Title:   "Police appeal after sexual assault incident",
		Summary: "Witnesses requested by investigators",
		Link:    "https://example.test/notice",
	})
	if decision.Outcome == OutcomeDrop || decision.Outcome == OutcomeDowngrade {
		t.Fatalf("expected keep outcome, got %q (%+v)", decision.Outcome, decision)
	}
}

func TestEvaluateDropsCelebrityGossipSpam(t *testing.T) {
	path := filepath.Join("..", "..", "..", "registry", "noise_policy.json")
	engine, err := Load(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	decision := engine.Evaluate(model.RegistrySource{}, parse.FeedItem{
		Title: "Celebrity gossip giveaway and lottery update",
		Link:  "https://example.test/spam",
	})
	if decision.Outcome != OutcomeDrop {
		t.Fatalf("expected drop outcome, got %q (%+v)", decision.Outcome, decision)
	}
}

func TestABPolicySelectionDeterministic(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.json")
	pathB := filepath.Join(dir, "b.json")
	if err := os.WriteFile(pathA, []byte(`{"version":"A","hard_block_terms":[],"downgrade_terms":[],"actionable_overrides":["intel"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte(`{"version":"B","hard_block_terms":["rumor"],"downgrade_terms":[],"actionable_overrides":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := LoadAB(pathA, pathB, 100)
	if err != nil {
		t.Fatalf("load AB policies: %v", err)
	}
	item := parse.FeedItem{Title: "Rumor post", Link: "https://example.test/x"}
	decision1 := engine.Evaluate(model.RegistrySource{Source: model.SourceMetadata{SourceID: "src-1"}}, item)
	decision2 := engine.Evaluate(model.RegistrySource{Source: model.SourceMetadata{SourceID: "src-1"}}, item)
	if decision1.PolicyVariant != "b" || decision1.PolicyVersion != "B" {
		t.Fatalf("expected variant b / version B, got %+v", decision1)
	}
	if decision1.PolicyVariant != decision2.PolicyVariant || decision1.PolicyVersion != decision2.PolicyVersion {
		t.Fatalf("expected deterministic AB selection, got %+v vs %+v", decision1, decision2)
	}
}
