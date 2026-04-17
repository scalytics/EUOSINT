package config

import (
	"path/filepath"
	"testing"
)

func TestExamplePolicyIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "examples", "agentops_policy.yaml")
	policy, err := LoadPolicy(path, "core")
	if err != nil {
		t.Fatalf("expected example policy to load, got %v", err)
	}
	if policy.GroupName != "core" {
		t.Fatalf("unexpected example group name %q", policy.GroupName)
	}
}
