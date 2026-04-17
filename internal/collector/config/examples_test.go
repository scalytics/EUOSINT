package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleEnvFilesExposeAgentOpsRuntimeKeys(t *testing.T) {
	for _, name := range []string{"agentops-kafscale.env", "hybrid-agentops.env"} {
		path := filepath.Join("..", "..", "..", "docs", "examples", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(raw)
		for _, key := range []string{"AGENTOPS_ENABLED=", "AGENTOPS_BROKERS=", "AGENTOPS_GROUP_NAME=", "UI_MODE=", "PROFILE="} {
			if !strings.Contains(text, key) {
				t.Fatalf("%s missing %s", name, key)
			}
		}
	}
}
