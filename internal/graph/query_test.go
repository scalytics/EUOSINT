package graph

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	agentopschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
	"github.com/scalytics/kafSIEM/internal/graph/schema"
)

func TestReaderNeighborhoodAndPath(t *testing.T) {
	db := seededGraphDB(t)
	reader := NewReader(db)

	entities, edges, err := reader.Neighborhood(context.Background(), "agent:alice", 2, nil, TimeRange{})
	if err != nil {
		t.Fatalf("Neighborhood() error = %v", err)
	}
	if len(entities) < 4 || len(edges) < 3 {
		t.Fatalf("unexpected neighborhood entities=%d edges=%d", len(entities), len(edges))
	}

	path, ok, err := reader.Path(context.Background(), "agent:alice", "trace:tr-1", 3, TimeRange{})
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if !ok || len(path) != 2 || path[0].DstID != "task:t-1" || path[1].DstID != "trace:tr-1" {
		t.Fatalf("unexpected path %#v ok=%v", path, ok)
	}
}

func TestReaderProfileAndGeometryQueries(t *testing.T) {
	db := seededGraphDB(t)
	reader := NewReader(db)

	profile, err := reader.EntityProfile(context.Background(), "task:t-1")
	if err != nil {
		t.Fatalf("EntityProfile() error = %v", err)
	}
	if profile.EdgeCounts["sent"] != 1 || profile.EdgeCounts["spans"] != 1 || len(profile.TopNeighbors) == 0 {
		t.Fatalf("unexpected profile %#v", profile)
	}

	geometry, err := reader.Geometry(context.Background(), "location:pt-1")
	if err != nil {
		t.Fatalf("Geometry() error = %v", err)
	}
	if geometry.GeometryType != "point" || geometry.MinLat != 35.8989 {
		t.Fatalf("unexpected geometry %#v", geometry)
	}

	entities, geometries, err := reader.WithinBBox(context.Background(), [4]float64{14.40, 35.80, 14.60, 36.00}, []string{CoreTypeLocation, CoreTypeArea}, TimeRange{})
	if err != nil {
		t.Fatalf("WithinBBox() error = %v", err)
	}
	if len(entities) != 2 || len(geometries) != 2 {
		t.Fatalf("unexpected bbox result entities=%d geometries=%d", len(entities), len(geometries))
	}

	nearbyEntities, _, err := reader.Nearby(context.Background(), [2]float64{14.5146, 35.8989}, 50, []string{CoreTypeLocation}, TimeRange{})
	if err != nil {
		t.Fatalf("Nearby() error = %v", err)
	}
	if len(nearbyEntities) != 1 || nearbyEntities[0].ID != "location:pt-1" {
		t.Fatalf("unexpected nearby entities %#v", nearbyEntities)
	}

	intersects, err := reader.Intersects(context.Background(), "location:pt-1", "area:ao-1", TimeRange{})
	if err != nil {
		t.Fatalf("Intersects() error = %v", err)
	}
	if !intersects {
		t.Fatal("expected point geometry to intersect containing area")
	}
}

func seededGraphDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := agentopschema.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Apply(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages (record_id, topic, topic_family, partition, offset, timestamp, outcome) VALUES ('msg-1', 'group.core.requests', 'requests', 0, 1, '2026-04-20T10:00:00Z', 'accepted')`); err != nil {
		t.Fatal(err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range []Entity{
		{ID: "agent:alice", Type: "agent", CanonicalID: "alice", FirstSeen: "2026-04-20T09:00:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "task:t-1", Type: "task", CanonicalID: "t-1", FirstSeen: "2026-04-20T09:05:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "trace:tr-1", Type: "trace", CanonicalID: "tr-1", FirstSeen: "2026-04-20T09:10:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "location:pt-1", Type: CoreTypeLocation, CanonicalID: "pt-1", FirstSeen: "2026-04-20T09:00:00Z", LastSeen: "2026-04-20T10:00:00Z"},
		{ID: "area:ao-1", Type: CoreTypeArea, CanonicalID: "ao-1", FirstSeen: "2026-04-20T09:00:00Z", LastSeen: "2026-04-20T10:00:00Z"},
	} {
		if err := UpsertEntity(tx, entity); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range []Edge{
		{SrcID: "agent:alice", DstID: "task:t-1", Type: "sent", ValidFrom: "2026-04-20T09:05:00Z", EvidenceMsg: "msg-1"},
		{SrcID: "task:t-1", DstID: "trace:tr-1", Type: "spans", ValidFrom: "2026-04-20T09:10:00Z", EvidenceMsg: "msg-1"},
		{SrcID: "task:t-1", DstID: "location:pt-1", Type: "observed_at", ValidFrom: "2026-04-20T09:20:00Z", EvidenceMsg: "msg-1"},
	} {
		if _, _, err := AppendEdge(tx, edge); err != nil {
			t.Fatal(err)
		}
	}
	if err := UpsertGeometry(tx, Geometry{
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
	if err := UpsertGeometry(tx, Geometry{
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
	return db
}
