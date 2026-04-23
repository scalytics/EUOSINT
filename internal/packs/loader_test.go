package packs

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	agentopsschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
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

func TestBundledDronesDetectorsAndQueriesExecuteAgainstFixtureDB(t *testing.T) {
	root := filepath.Join("..", "..", "packs")
	registry, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var drones *Pack
	for i := range registry.Packs {
		if registry.Packs[i].Name == "drones" {
			drones = &registry.Packs[i]
			break
		}
	}
	if drones == nil {
		t.Fatal("expected bundled drones pack")
	}

	db, err := agentopsschema.Open(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE VIEW view_drone_faults_by_variant AS SELECT 'variant-a' AS variant_id, 'fault-mode-1' AS fault_mode_id, 'platform-1' AS platform_id, '2026-04-22T10:00:00Z' AS observed_at UNION ALL SELECT 'variant-a', 'fault-mode-1', 'platform-2', '2026-04-22T10:10:00Z' UNION ALL SELECT 'variant-a', 'fault-mode-1', 'platform-3', '2026-04-22T10:20:00Z'`,
		`CREATE VIEW view_drone_roe_drift AS SELECT 'sortie-1' AS sortie_id, 'area-roe' AS area_id, '2026-04-22T10:00:00Z' AS crossed_at`,
		`CREATE VIEW view_drone_silent_subsystems AS SELECT 'sortie-1' AS sortie_id, 'platform-1' AS platform_id, 'nav' AS subsystem_id, 90 AS heartbeat_gap_seconds, 20 AS expected_gap_seconds, '2026-04-22T10:00:00Z' AS observed_at`,
		`CREATE VIEW view_drone_autonomy_regressions AS SELECT 'autonomy-7.4.2' AS software_version, 'variant-a' AS variant_id, 0.42 AS fault_rate_delta, '2026-04-22T10:00:00Z' AS deployed_at`,
		`CREATE VIEW view_drone_component_lot_anomalies AS SELECT 'lot-55' AS component_lot, 'fault-mode-1' AS fault_mode_id, 3.7 AS z_score, '2026-04-22T10:00:00Z' AS observed_at`,
		`CREATE VIEW view_drone_pre_mission_gaps AS SELECT 'mission-1' AS mission_id, 'platform-1' AS platform_id, 1 AS open_faults, 0 AS open_signoffs, '2026-04-22T10:00:00Z' AS scheduled_at`,
		`CREATE VIEW view_drone_changes_since_signoff AS SELECT 'platform-1' AS platform_id, '2026-04-22T10:00:00Z' AS changed_at, 'software' AS change_type, 'Autonomy package upgraded' AS summary`,
		`CREATE VIEW view_drone_fault_mode_spread AS SELECT 'fault-mode-1' AS fault_mode_id, 'platform-1' AS platform_id, 'variant-a' AS variant_id, '2026-04-22T10:00:00Z' AS last_seen`,
		`CREATE VIEW view_drone_platform_software AS SELECT 'platform-1' AS platform_id, 'autonomy-7.4.2' AS software_version, '2026-04-22T10:00:00Z' AS observed_at`,
		`CREATE VIEW view_drone_degraded_comms AS SELECT 'sortie-1' AS sortie_id, 'platform-1' AS platform_id, 'area-roe' AS area_id, '2026-04-22T10:00:00Z' AS degraded_at`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	for _, detector := range drones.Detectors {
		if err := executeReadOnlyStatement(db, detector.Query,
			sql.Named("window_start", "2026-04-20T00:00:00Z"),
			sql.Named("area_id", "area-roe"),
			sql.Named("platform_id", "platform-1"),
			sql.Named("fault_mode_id", "fault-mode-1"),
			sql.Named("software_version", "autonomy-7.4.2"),
			sql.Named("cve", "CVE-2026-12345"),
		); err != nil {
			t.Fatalf("detector %s failed: %v", detector.ID, err)
		}
	}
	for _, query := range drones.Queries {
		if err := executeReadOnlyStatement(db, query.SQL,
			sql.Named("window_start", "2026-04-20T00:00:00Z"),
			sql.Named("area_id", "area-roe"),
			sql.Named("platform_id", "platform-1"),
			sql.Named("fault_mode_id", "fault-mode-1"),
			sql.Named("software_version", "autonomy-7.4.2"),
		); err != nil {
			t.Fatalf("query %s failed: %v", query.ID, err)
		}
	}
}

func TestBundledScadaDetectorsAndQueriesExecuteAgainstFixtureDB(t *testing.T) {
	root := filepath.Join("..", "..", "packs")
	registry, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var scada *Pack
	for i := range registry.Packs {
		if registry.Packs[i].Name == "scada" {
			scada = &registry.Packs[i]
			break
		}
	}
	if scada == nil {
		t.Fatal("expected bundled scada pack")
	}

	db, err := agentopsschema.Open(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE VIEW view_scada_purdue_violations AS SELECT 'plc-l1-1' AS src_device_id, 'historian-l4-1' AS dst_device_id, '2026-04-22T10:00:00Z' AS observed_at`,
		`CREATE VIEW view_scada_changes_without_work_order AS SELECT 'chg-1' AS change_id, 'safety-plc-1' AS device_id, 'eng-1' AS engineer_id, '2026-04-22T10:00:00Z' AS applied_at`,
		`CREATE VIEW view_scada_firmware_drift AS SELECT 'safety-plc-1' AS device_id, 'fw-expected-1' AS expected_firmware, 'fw-live-2' AS observed_firmware, 'critical' AS criticality, '2026-04-22T10:00:00Z' AS observed_at`,
		`CREATE VIEW view_scada_tradecraft_matches AS SELECT 'triton' AS pattern_id, 'safety-plc-1' AS device_id, '2026-04-22T10:00:00Z' AS matched_at`,
		`CREATE VIEW view_scada_stale_sessions AS SELECT 'sess-1' AS session_id, 'eng-1' AS engineer_id, 'safety-plc-1' AS device_id, 36 AS open_hours, '2026-04-22T10:00:00Z' AS observed_at`,
		`CREATE VIEW view_scada_alarm_flood_after_change AS SELECT 'chg-1' AS change_id, 'tag-1' AS tag_id, 57 AS alarm_count, '2026-04-22T10:00:00Z' AS flood_started_at`,
		`CREATE VIEW view_scada_device_vulnerabilities AS SELECT 'safety-plc-1' AS device_id, 'plant-a' AS plant_id, 'critical' AS criticality, 'fw-live-2' AS firmware_version, 'CVE-2026-12345' AS cve`,
		`CREATE VIEW view_scada_tag_changes AS SELECT 'chg-1' AS change_id, 'safety-plc-1' AS device_id, 'tag-1' AS tag_id, '2026-04-22T10:00:00Z' AS applied_at`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	for _, detector := range scada.Detectors {
		if err := executeReadOnlyStatement(db, detector.Query,
			sql.Named("window_start", "2026-04-20T00:00:00Z"),
			sql.Named("cve", "CVE-2026-12345"),
			sql.Named("tag_id", "tag-1"),
			sql.Named("pattern_id", "triton"),
		); err != nil {
			t.Fatalf("detector %s failed: %v", detector.ID, err)
		}
	}
	for _, query := range scada.Queries {
		if err := executeReadOnlyStatement(db, query.SQL,
			sql.Named("window_start", "2026-04-20T00:00:00Z"),
			sql.Named("cve", "CVE-2026-12345"),
			sql.Named("tag_id", "tag-1"),
			sql.Named("pattern_id", "triton"),
		); err != nil {
			t.Fatalf("query %s failed: %v", query.ID, err)
		}
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

func executeReadOnlyStatement(db *sql.DB, query string, args ...any) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	_, err = rows.Columns()
	return err
}
