package kafsiemapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	agentopschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
	"github.com/scalytics/kafSIEM/internal/graph"
	graphschema "github.com/scalytics/kafSIEM/internal/graph/schema"
)

func TestServerRoutes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentops.db")
	seedAPIStore(t, dbPath)

	srv, err := New(Config{
		Listen:   ":0",
		DBPath:   dbPath,
		PacksDir: filepath.Join(t.TempDir(), "packs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	t.Run("healthz", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("healthz code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("readyz", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("readyz code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("entity-profile", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/agent/alice", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("entity profile code=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["first_seen"] == nil {
			t.Fatalf("unexpected payload %#v", payload)
		}
	})

	t.Run("entity-neighborhood", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/agent/alice/neighborhood?depth=2", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("entity neighborhood code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("entity-geometry", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/location/pt-1/geometry", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("entity geometry code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("entity-provenance", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/agent/alice/provenance", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("entity provenance code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("entity-timeline", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/agent/alice/timeline?limit=5", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("entity timeline code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("graph-path", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/graph/path?src=agent:alice&dst=trace:tr-1&max=3", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("graph path code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("map-features", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map/features?bbox=14.40,35.80,14.60,36.00&types=location,area", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("map features code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("map-layers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map/layers", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("map layers code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("flows", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/flows?limit=5", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("flows code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("flow-messages", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/flows/corr-1/messages?limit=5", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("flow messages code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("flow-tasks", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/flows/corr-1/tasks", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("flow tasks code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("flow-traces", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/flows/corr-1/traces", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("flow traces code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("ontology", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ontology/types", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("ontology code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("replays-post", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/replays", nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("replays post code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("ontology-packs", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ontology/packs", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("ontology packs code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("health", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("health code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("topic-health", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/topic-health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("topic health code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("replays", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/replays?limit=5", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("replays code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("search", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=alice", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("search code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("bad-entity-type", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/platform/auv-1", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("bad entity type code=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("bad-bbox", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map/features?bbox=bad", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad bbox code=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestCursorAndBBoxHelpers(t *testing.T) {
	raw := encodeCursor("2026-04-20T10:00:00Z", "msg-1")
	got, err := decodeCursor(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Timestamp != "2026-04-20T10:00:00Z" || got.ID != "msg-1" {
		t.Fatalf("unexpected cursor %#v", got)
	}
	if _, err := decodeCursor("%%%"); err == nil {
		t.Fatal("expected invalid cursor error")
	}
	if _, err := parseBBox("14.1,35.1,14.2,35.2"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseBBox("14.1,35.1"); err == nil {
		t.Fatal("expected invalid bbox")
	}
}

func seedAPIStore(t *testing.T, path string) {
	t.Helper()
	db, err := agentopschema.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := graphschema.Apply(db); err != nil {
		t.Fatal(err)
	}
	takenAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO health_snapshots (taken_at, connected, group_id, accepted_count, rejected_count, mirrored_count, mirror_failed_count, replay_active, replay_last_count, rejected_by_reason_json) VALUES (?, 1, 'group-a', 4, 0, 0, 0, 0, 0, '{}')`, takenAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO topic_stats (topic, message_count, active_agents, first_message_at, last_message_at) VALUES ('group.core.requests', 12, 1, '2026-04-20T09:00:00Z', '2026-04-20T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO replay_sessions (id, group_id, status, started_at, message_count, topics_json) VALUES ('replay-1', 'group-a', 'completed', '2026-04-20T09:00:00Z', 3, '["group.core.requests"]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO flows (id, first_seen, last_seen, message_count, latest_status, latest_preview) VALUES ('corr-1', '2026-04-20T09:00:00Z', '2026-04-20T10:00:00Z', 1, 'completed', 'Investigate outage')`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO flow_participants (flow_id, kind, value) VALUES ('corr-1', 'topic', 'group.core.requests')`,
		`INSERT INTO flow_participants (flow_id, kind, value) VALUES ('corr-1', 'sender', 'alice')`,
		`INSERT INTO flow_participants (flow_id, kind, value) VALUES ('corr-1', 'trace', 'tr-1')`,
		`INSERT INTO flow_participants (flow_id, kind, value) VALUES ('corr-1', 'task', 't-1')`,
		`INSERT INTO traces (id, span_count, latest_title, started_at, ended_at, duration_ms) VALUES ('tr-1', 1, 'trace title', '2026-04-20T09:10:00Z', '2026-04-20T10:00:00Z', 1000)`,
		`INSERT INTO trace_agents (trace_id, agent_id) VALUES ('tr-1', 'alice')`,
		`INSERT INTO trace_span_types (trace_id, span_type) VALUES ('tr-1', 'TOOL')`,
		`INSERT INTO tasks (id, parent_task_id, delegation_depth, requester_id, responder_id, original_requester_id, status, description, last_summary, first_seen, last_seen) VALUES ('t-1', 'root-1', 1, 'alice', 'bob', 'orch-1', 'completed', 'Investigate outage', 'done', '2026-04-20T09:00:00Z', '2026-04-20T10:00:00Z')`,
		`INSERT INTO messages (record_id, topic, topic_family, partition, offset, timestamp, envelope_type, sender_id, correlation_id, trace_id, task_id, status, preview, content, outcome) VALUES ('msg-1', 'group.core.requests', 'requests', 0, 1, '2026-04-20T10:00:00Z', 'request', 'alice', 'corr-1', 'tr-1', 't-1', 'completed', 'Investigate outage', '{"task":"inspect"}', 'accepted')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, entity := range []graph.Entity{
		{ID: "agent:alice", Type: "agent", CanonicalID: "alice", FirstSeen: "2026-04-20T09:00:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "task:t-1", Type: "task", CanonicalID: "t-1", FirstSeen: "2026-04-20T09:00:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "trace:tr-1", Type: "trace", CanonicalID: "tr-1", FirstSeen: "2026-04-20T09:10:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "location:pt-1", Type: graph.CoreTypeLocation, CanonicalID: "pt-1", FirstSeen: "2026-04-20T09:20:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "area:ao-1", Type: graph.CoreTypeArea, CanonicalID: "ao-1", FirstSeen: "2026-04-20T09:20:00Z", LastSeen: "2026-04-20T10:00:00Z"},
	} {
		if err := graph.UpsertEntity(tx, entity); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range []graph.Edge{
		{SrcID: "agent:alice", DstID: "task:t-1", Type: "sent", ValidFrom: "2026-04-20T09:05:00Z", EvidenceMsg: "msg-1"},
		{SrcID: "task:t-1", DstID: "trace:tr-1", Type: "spans", ValidFrom: "2026-04-20T09:10:00Z", EvidenceMsg: "msg-1"},
		{SrcID: "task:t-1", DstID: "location:pt-1", Type: "observed_at", ValidFrom: "2026-04-20T09:20:00Z", EvidenceMsg: "msg-1"},
		{SrcID: "location:pt-1", DstID: "area:ao-1", Type: "in_area", ValidFrom: "2026-04-20T09:25:00Z", EvidenceMsg: "msg-1"},
	} {
		if _, _, err := graph.AppendEdge(tx, edge); err != nil {
			t.Fatal(err)
		}
	}
	if err := graph.AppendProvenance(tx, graph.Provenance{SubjectKind: "entity", SubjectID: "agent:alice", Stage: "graph", Decision: "inserted", Reasons: []string{"seed"}, ProducedAt: "2026-04-20T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := graph.UpsertGeometry(tx, graph.Geometry{
		EntityID:     "location:pt-1",
		GeometryType: "point",
		GeoJSON:      []byte(`{"type":"Point","coordinates":[14.5146,35.8989]}`),
		MinLat:       35.8989,
		MinLon:       14.5146,
		MaxLat:       35.8989,
		MaxLon:       14.5146,
		ObservedAt:   "2026-04-20T09:20:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := graph.UpsertGeometry(tx, graph.Geometry{
		EntityID:     "area:ao-1",
		GeometryType: "polygon",
		GeoJSON:      []byte(`{"type":"Polygon","coordinates":[[[14.4,35.8],[14.6,35.8],[14.6,36.0],[14.4,36.0],[14.4,35.8]]]}`),
		MinLat:       35.8,
		MinLon:       14.4,
		MaxLat:       36.0,
		MaxLon:       14.6,
		ObservedAt:   "2026-04-20T09:20:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
