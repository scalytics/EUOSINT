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

func TestLoadDirValidPack(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "drones", "pack.yaml"), `
name: drones
version: 0.1.0
schema_version: v1
description: Unmanned systems pack
owner: scalytics
requires:
  core_min_version: "0.5.0"
entity_types:
  - id: platform
    display: Platform
    canonical_id_format: "<serial>"
  - id: sortie
    display: Sortie
edge_types:
  - id: assigned_to
    src_types: [sortie]
    dst_types: [platform]
map_layers:
  - id: live-platforms
    name: Live Platforms
    kind: overlay
    geometry_source: entity_geometry
    entity_types: [platform]
    render: point
`)
	writePackFile(t, filepath.Join(root, "drones", "detectors", "cohort-failure.yaml"), `
id: cohort-failure
severity: high
window: 72h
match:
  pattern: |
    SELECT platform_id, COUNT(*) AS n
      FROM view_platform_faults
     GROUP BY platform_id
explanation_template: >
  Cohort failure detected.
suggested_actions:
  - inspect affected platforms
`)
	writePackFile(t, filepath.Join(root, "drones", "views", "platform.yaml"), `
entity_type: platform
title: Platform
fields:
  - id: serial
    label: Serial
`)
	writePackFile(t, filepath.Join(root, "drones", "queries", "same-failure-mode.yaml"), `
id: same-failure-mode
title: Same failure mode across fleet
sql: |
  SELECT fault_mode, COUNT(*) AS n
    FROM view_faults_by_platform
   GROUP BY fault_mode
`)
	writePackFile(t, filepath.Join(root, "drones", "reports", "validation-report.md.tmpl"), `# Validation`)

	registry, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Packs) != 1 {
		t.Fatalf("expected one pack, got %#v", registry.Packs)
	}
	pack := registry.Packs[0]
	if pack.Name != "drones" || pack.SchemaVersion != "v1" {
		t.Fatalf("unexpected pack %#v", pack)
	}
	if !registry.AllowsEntityType("platform") {
		t.Fatalf("expected pack entity type in registry %#v", registry.EntityTypes)
	}
	if len(pack.Detectors) != 1 || pack.Detectors[0].Source != "pack/drones" {
		t.Fatalf("unexpected detectors %#v", pack.Detectors)
	}
	if len(pack.Views) != 1 || pack.Views[0].EntityType != "platform" {
		t.Fatalf("unexpected views %#v", pack.Views)
	}
	if len(pack.Queries) != 1 || pack.Queries[0].ID != "same-failure-mode" {
		t.Fatalf("unexpected queries %#v", pack.Queries)
	}
	if len(pack.ReportTemplates) != 1 || pack.ReportTemplates[0] != "validation-report.md.tmpl" {
		t.Fatalf("unexpected reports %#v", pack.ReportTemplates)
	}
	if len(registry.MapLayers) != 2 || registry.MapLayers[1].ID != "live-platforms" {
		t.Fatalf("unexpected map layers %#v", registry.MapLayers)
	}
}

func TestLoadDirMalformedYAML(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "broken", "pack.yaml"), `name: broken: nope`)

	if _, err := LoadDir(root); err == nil {
		t.Fatal("expected malformed YAML error")
	}
}

func TestLoadDirTypeCollision(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "drones", "pack.yaml"), `
name: drones
version: 0.1.0
entity_types:
  - id: platform
edge_types:
  - id: assigned_to
    src_types: [platform]
    dst_types: [agent]
`)
	writePackFile(t, filepath.Join(root, "scada", "pack.yaml"), `
name: scada
version: 0.1.0
entity_types:
  - id: platform
`)

	if _, err := LoadDir(root); err == nil {
		t.Fatal("expected type collision")
	}
}

func TestLoadDirMissingRequiredField(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "drones", "pack.yaml"), `
name: drones
entity_types:
  - id: platform
`)

	if _, err := LoadDir(root); err == nil {
		t.Fatal("expected missing version error")
	}
}

func TestLoadDirRejectsMutableDetectorSQL(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "drones", "pack.yaml"), `
name: drones
version: 0.1.0
entity_types:
  - id: platform
`)
	writePackFile(t, filepath.Join(root, "drones", "detectors", "bad.yaml"), `
id: bad
severity: low
match:
  pattern: DELETE FROM entities
`)

	if _, err := LoadDir(root); err == nil {
		t.Fatal("expected mutable detector SQL to fail")
	}
}

func TestBundledPacksLoad(t *testing.T) {
	root := filepath.Join("..", "..", "packs")
	registry, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Packs) != 2 {
		t.Fatalf("expected bundled drones and scada packs, got %#v", registry.Packs)
	}
	if !registry.AllowsEntityType("platform") || !registry.AllowsEntityType("plant") {
		t.Fatalf("expected bundled pack entity types in registry %#v", registry.EntityTypes)
	}
	if len(registry.Detectors) < 2 || len(registry.Queries) < 2 || len(registry.Views) < 2 {
		t.Fatalf("expected bundled pack assets in registry detectors=%d queries=%d views=%d", len(registry.Detectors), len(registry.Queries), len(registry.Views))
	}
}

func writePackFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
