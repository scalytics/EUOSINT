package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserFacingDocsAvoidLegacySIEMQueueLanguage(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "..", "README.md"),
		filepath.Join("..", "..", "..", "docs", "agentops-operator-guide.md"),
		filepath.Join("..", "..", "..", "src", "agentops", "pages", "AgentOpsRuntimeDesk.tsx"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(strings.ToLower(string(raw)), "siem alert queue") {
			t.Fatalf("legacy siem queue language still present in %s", path)
		}
	}
}
