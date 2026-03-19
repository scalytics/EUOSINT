package noisegate

import (
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
