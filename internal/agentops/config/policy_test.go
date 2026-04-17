package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicyValid(t *testing.T) {
	policy := DefaultPolicy("core")
	if policy.GroupName != "core" {
		t.Fatalf("expected group name core, got %q", policy.GroupName)
	}
	if err := ValidatePolicy(policy); err != nil {
		t.Fatalf("default policy should be valid: %v", err)
	}
}

func TestLoadPolicyFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentops_policy.yaml")
	raw := []byte(`
version: 1
group_name: core
topic_mode: manual
required_topics:
  - announce
  - requests
optional_topics:
  - orchestrator
grouping:
  flow_key: correlation_id
  replay_max_records: 300
ui:
  show_topic_health: true
  show_memory: true
  show_orchestrator: false
hybrid:
  enabled_categories:
    - cyber_advisory
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(path, "ignored")
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if policy.TopicMode != "manual" || policy.Grouping.ReplayMaxRecords != 300 {
		t.Fatalf("unexpected policy values: %#v", policy)
	}
	if len(policy.Hybrid.EnabledCategories) != 1 || policy.Hybrid.EnabledCategories[0] != "cyber_advisory" {
		t.Fatalf("unexpected hybrid categories: %#v", policy.Hybrid.EnabledCategories)
	}
}

func TestLoadPolicyMissingFileFallsBackToDefault(t *testing.T) {
	policy, err := LoadPolicy(filepath.Join(t.TempDir(), "missing.yaml"), "core")
	if err != nil {
		t.Fatalf("expected missing file fallback, got %v", err)
	}
	if policy.GroupName != "core" || policy.TopicMode != "auto" {
		t.Fatalf("unexpected fallback policy %#v", policy)
	}
}

func TestValidatePolicyRejectsInvalidTopicMode(t *testing.T) {
	policy := DefaultPolicy("core")
	policy.TopicMode = "broken"
	if err := ValidatePolicy(policy); err == nil {
		t.Fatal("expected invalid topic_mode error")
	}
}

func TestValidatePolicyRejectsInvalidTopicFamily(t *testing.T) {
	policy := DefaultPolicy("core")
	policy.RequiredTopics = append(policy.RequiredTopics, "broken")
	if err := ValidatePolicy(policy); err == nil {
		t.Fatal("expected invalid topic family error")
	}
}

func TestValidatePolicyRejectsInvalidReplayLimit(t *testing.T) {
	policy := DefaultPolicy("core")
	policy.Grouping.ReplayMaxRecords = 0
	if err := ValidatePolicy(policy); err == nil {
		t.Fatal("expected invalid replay max records error")
	}
}
