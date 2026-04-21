package packs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirDefaultsWithoutPacks(t *testing.T) {
	registry, err := LoadDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if !registry.AllowsEntityType("agent") || !registry.AllowsEntityType("location") {
		t.Fatalf("unexpected core entity types %#v", registry.EntityTypes)
	}
	if len(registry.MapLayers) == 0 || registry.MapLayers[0].Source != "core" {
		t.Fatalf("unexpected core map layers %#v", registry.MapLayers)
	}
}

func TestLoadDirPackAndCollision(t *testing.T) {
	root := t.TempDir()
	packDir := filepath.Join(root, "drones")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "pack.yaml"), []byte(`
name: drones
version: 0.1.0
entity_types:
  - platform
edge_types:
  - runs
map_layers:
  - id: drone-ops
    name: Drone Ops
    kind: overlay
`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !registry.AllowsEntityType("platform") || len(registry.Packs) != 1 {
		t.Fatalf("unexpected registry %#v", registry)
	}

	badDir := filepath.Join(root, "collision")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "pack.yaml"), []byte(`
name: collision
version: 0.1.0
entity_types:
  - platform
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(root); err == nil {
		t.Fatal("expected type collision")
	}
}
