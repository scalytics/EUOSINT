package graph

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	agentopschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
	"github.com/scalytics/kafSIEM/internal/graph/schema"
)

type benchmarkFixture struct {
	reader      *Reader
	taskID      string
	agentID     string
	traceID     string
	locationID  string
	areaID      string
	searchPoint [2]float64
	searchBBox  [4]float64
}

var (
	benchmarkOnce         sync.Once
	benchmarkFixtureState benchmarkFixture
	benchmarkFixtureErr   error
)

func BenchmarkReaderNeighborhood(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := fixture.reader.Neighborhood(ctx, fixture.agentID, 2, nil, TimeRange{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReaderPath(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok, err := fixture.reader.Path(ctx, fixture.agentID, fixture.traceID, 3, TimeRange{}); err != nil {
			b.Fatal(err)
		} else if !ok {
			b.Fatal("expected path")
		}
	}
}

func BenchmarkReaderEntityProfile(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := fixture.reader.EntityProfile(ctx, fixture.taskID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReaderGeometry(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := fixture.reader.Geometry(ctx, fixture.locationID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReaderWithinBBox(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := fixture.reader.WithinBBox(ctx, fixture.searchBBox, []string{CoreTypeLocation}, TimeRange{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReaderIntersects(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := fixture.reader.Intersects(ctx, fixture.locationID, fixture.areaID, TimeRange{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReaderNearby(b *testing.B) {
	fixture := loadBenchmarkFixture(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := fixture.reader.Nearby(ctx, fixture.searchPoint, 250, []string{CoreTypeLocation}, TimeRange{}); err != nil {
			b.Fatal(err)
		}
	}
}

func loadBenchmarkFixture(tb testing.TB) benchmarkFixture {
	tb.Helper()
	benchmarkOnce.Do(func() {
		benchmarkFixtureState, benchmarkFixtureErr = buildBenchmarkFixture(tb)
	})
	if benchmarkFixtureErr != nil {
		tb.Fatal(benchmarkFixtureErr)
	}
	return benchmarkFixtureState
}

func buildBenchmarkFixture(tb testing.TB) (benchmarkFixture, error) {
	tb.Helper()
	db, err := agentopschema.Open(filepath.Join(tb.TempDir(), "graph-bench.db"))
	if err != nil {
		return benchmarkFixture{}, err
	}
	if err := schema.Apply(db); err != nil {
		return benchmarkFixture{}, err
	}
	if err := seedBenchmarkGraph(db); err != nil {
		return benchmarkFixture{}, err
	}
	return benchmarkFixture{
		reader:      NewReader(db),
		agentID:     "agent:agent-0001",
		taskID:      "task:task-000001",
		traceID:     "trace:trace-000001",
		locationID:  "location:loc-000001",
		areaID:      "area:sector-west",
		searchPoint: [2]float64{14.5146, 35.8989},
		searchBBox:  [4]float64{14.40, 35.80, 14.65, 36.05},
	}, nil
}

func seedBenchmarkGraph(db *sql.DB) error {
	if _, err := db.Exec(`INSERT INTO messages (record_id, topic, topic_family, partition, offset, timestamp, outcome) VALUES ('msg-bench', 'group.core.requests', 'requests', 0, 1, '2026-04-20T10:00:00Z', 'accepted')`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	entityStmt, err := tx.Prepare(`
		INSERT INTO entities (id, type, canonical_id, first_seen, last_seen, attrs_json)
		VALUES (?, ?, ?, ?, ?, '{}')
	`)
	if err != nil {
		return err
	}
	defer entityStmt.Close()
	edgeStmt, err := tx.Prepare(`
		INSERT INTO edges (src_id, dst_id, type, valid_from, evidence_msg, weight, attrs_json)
		VALUES (?, ?, ?, ?, 'msg-bench', 1.0, '{}')
	`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()
	geomStmt, err := tx.Prepare(`
		INSERT INTO entity_geometry (entity_id, geometry_type, geojson, srid, min_lat, min_lon, max_lat, max_lon, observed_at)
		VALUES (?, ?, ?, 4326, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer geomStmt.Close()

	if _, err := entityStmt.Exec("area:sector-west", CoreTypeArea, "sector-west", "2026-04-20T09:00:00Z", "2026-04-20T10:00:00Z"); err != nil {
		return err
	}
	areaGeoJSON := `{"type":"Polygon","coordinates":[[[14.40,35.80],[14.65,35.80],[14.65,36.05],[14.40,36.05],[14.40,35.80]]]}`
	if _, err := geomStmt.Exec("area:sector-west", "polygon", areaGeoJSON, 35.80, 14.40, 36.05, 14.65, "2026-04-20T09:00:00Z"); err != nil {
		return err
	}

	const totalTasks = 25000
	const totalAgents = 500
	for i := 1; i <= totalAgents; i++ {
		agentCanonical := fmt.Sprintf("agent-%04d", i)
		agentID := EntityID("agent", agentCanonical)
		if _, err := entityStmt.Exec(agentID, "agent", agentCanonical, "2026-04-20T09:00:00Z", "2026-04-20T10:00:00Z"); err != nil {
			return err
		}
	}
	for i := 1; i <= totalTasks; i++ {
		taskCanonical := fmt.Sprintf("task-%06d", i)
		traceCanonical := fmt.Sprintf("trace-%06d", i)
		locationCanonical := fmt.Sprintf("loc-%06d", i)
		taskID := EntityID("task", taskCanonical)
		traceID := EntityID("trace", traceCanonical)
		locationID := EntityID(CoreTypeLocation, locationCanonical)
		agentID := EntityID("agent", fmt.Sprintf("agent-%04d", ((i-1)%totalAgents)+1))

		if _, err := entityStmt.Exec(taskID, "task", taskCanonical, "2026-04-20T09:00:00Z", "2026-04-20T10:00:00Z"); err != nil {
			return err
		}
		if _, err := entityStmt.Exec(traceID, "trace", traceCanonical, "2026-04-20T09:00:00Z", "2026-04-20T10:00:00Z"); err != nil {
			return err
		}
		if _, err := entityStmt.Exec(locationID, CoreTypeLocation, locationCanonical, "2026-04-20T09:00:00Z", "2026-04-20T10:00:00Z"); err != nil {
			return err
		}

		if _, err := edgeStmt.Exec(agentID, taskID, "sent", "2026-04-20T09:05:00Z"); err != nil {
			return err
		}
		if _, err := edgeStmt.Exec(taskID, traceID, "spans", "2026-04-20T09:10:00Z"); err != nil {
			return err
		}
		if _, err := edgeStmt.Exec(taskID, locationID, "observed_at", "2026-04-20T09:15:00Z"); err != nil {
			return err
		}
		if _, err := edgeStmt.Exec(locationID, "area:sector-west", "in_area", "2026-04-20T09:20:00Z"); err != nil {
			return err
		}

		lat := 35.80 + float64(i%250)*0.001
		lon := 14.40 + float64(i%250)*0.001
		geoJSON := fmt.Sprintf(`{"type":"Point","coordinates":[%.4f,%.4f]}`, lon, lat)
		if _, err := geomStmt.Exec(locationID, "point", geoJSON, lat, lon, lat, lon, "2026-04-20T09:15:00Z"); err != nil {
			return err
		}
	}
	return tx.Commit()
}
