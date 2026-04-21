package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scalytics/kafSIEM/api/specgen"
)

func TestGeneratedArtifactsAreUpToDate(t *testing.T) {
	root := filepath.Clean(filepath.Join(".."))
	for _, output := range specgen.Outputs() {
		path := filepath.Join(root, output.Path)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", output.Path, err)
		}
		if string(body) != output.Content {
			t.Fatalf("%s is stale; run `go generate ./api/...`", output.Path)
		}
	}
}
