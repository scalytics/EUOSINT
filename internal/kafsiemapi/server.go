package kafsiemapi

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	agentopsschema "github.com/scalytics/kafSIEM/internal/agentops/schema"
	"github.com/scalytics/kafSIEM/internal/agentops/store"
	"github.com/scalytics/kafSIEM/internal/graph"
	"github.com/scalytics/kafSIEM/internal/packs"
)

type Config struct {
	Listen   string
	DBPath   string
	PacksDir string
}

type Server struct {
	cfg      Config
	db       *sql.DB
	writeDB  *sql.DB
	store    store.Store
	graph    *graph.Reader
	registry packs.Registry
	router   http.Handler
}

type problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func New(cfg Config) (*Server, error) {
	readStore, err := store.OpenReadOnly(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	writeDB, err := agentopsschema.Open(storePathForWrite(cfg.DBPath))
	if err != nil {
		_ = readStore.Close()
		return nil, err
	}
	registry, err := packs.LoadDir(cfg.PacksDir)
	if err != nil {
		_ = readStore.Close()
		_ = writeDB.Close()
		return nil, err
	}
	s := &Server{
		cfg:      cfg,
		db:       readStore.DB(),
		writeDB:  writeDB,
		store:    readStore,
		graph:    graph.NewReader(readStore.DB()),
		registry: registry,
	}
	s.router = s.routes()
	return s, nil
}

func (s *Server) Close() error {
	err := s.store.Close()
	if s.writeDB != nil {
		_ = s.writeDB.Close()
	}
	return err
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.Listen,
		Handler: s.router,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func RunMain(args []string, stdout, stderr *os.File) error {
	cfg := Config{
		Listen:   envString("KAFSIEM_API_LISTEN", ":8081"),
		DBPath:   envString("AGENTOPS_OUTPUT_PATH", "public/agentops-state.json"),
		PacksDir: envString("KAFSIEM_PACKS_DIR", "packs"),
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--listen" && i+1 < len(args) {
			cfg.Listen = args[i+1]
			i++
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv, err := New(cfg)
	if err != nil {
		return err
	}
	defer srv.Close()
	_, _ = fmt.Fprintf(stdout, "kafSIEM API listening on %s\n", cfg.Listen)
	return srv.ListenAndServe(ctx)
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/entities/{type}/{id}", s.handleEntityProfile)
		r.Get("/entities/{type}/{id}/neighborhood", s.handleEntityNeighborhood)
		r.Get("/entities/{type}/{id}/provenance", s.handleEntityProvenance)
		r.Get("/entities/{type}/{id}/geometry", s.handleEntityGeometry)
		r.Get("/entities/{type}/{id}/timeline", s.handleEntityTimeline)
		r.Get("/graph/path", s.handleGraphPath)
		r.Get("/map/features", s.handleMapFeatures)
		r.Get("/map/layers", s.handleMapLayers)
		r.Get("/flows", s.handleFlows)
		r.Get("/flows/{id}", s.handleFlow)
		r.Get("/flows/{id}/messages", s.handleFlowMessages)
		r.Get("/flows/{id}/timeline", s.handleFlowMessages)
		r.Get("/topic-health", s.handleTopicHealth)
		r.Get("/health", s.handleHealth)
		r.Get("/replays", s.handleReplays)
		r.Post("/replays", s.handleReplayRequest)
		r.Get("/search", s.handleSearch)
		r.Get("/ontology/types", s.handleOntologyTypes)
		r.Get("/ontology/packs", s.handleOntologyPacks)
	})
	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "db-unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	var takenAt string
	if err := s.db.QueryRowContext(r.Context(), `SELECT taken_at FROM health_snapshots ORDER BY taken_at DESC LIMIT 1`).Scan(&takenAt); err != nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "not-ready", "no recent health snapshot")
		return
	}
	ts, err := time.Parse(time.RFC3339, takenAt)
	if err != nil || time.Since(ts) > 60*time.Second {
		writeProblem(w, r, http.StatusServiceUnavailable, "stale-health", "latest health snapshot is older than 60s")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleEntityProfile(w http.ResponseWriter, r *http.Request) {
	entityType, entityID, ok := s.entityPath(r)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "unknown-entity-type", "entity type is not enabled")
		return
	}
	profile, err := s.graph.EntityProfile(r.Context(), entityID)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	if profile.Entity.Type != entityType {
		writeProblem(w, r, http.StatusNotFound, "entity-not-found", "entity not found")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleEntityNeighborhood(w http.ResponseWriter, r *http.Request) {
	_, entityID, ok := s.entityPath(r)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "unknown-entity-type", "entity type is not enabled")
		return
	}
	depth := parseIntDefault(r.URL.Query().Get("depth"), 2)
	types := splitCSV(r.URL.Query().Get("types"))
	window := parseWindow(r.URL.Query().Get("window"))
	entities, edges, err := s.graph.Neighborhood(r.Context(), entityID, depth, types, window)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entities": entities, "edges": edges})
}

func (s *Server) handleEntityProvenance(w http.ResponseWriter, r *http.Request) {
	_, entityID, ok := s.entityPath(r)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "unknown-entity-type", "entity type is not enabled")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT subject_kind, subject_id, stage, policy_ver, inputs_json, decision, reasons_json, produced_at
		  FROM provenance
		 WHERE subject_id = ?
		 ORDER BY produced_at DESC, id DESC
	`, entityID)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	defer rows.Close()
	items := []graph.Provenance{}
	for rows.Next() {
		var item graph.Provenance
		var policyVer, inputsJSON, decision, reasonsJSON sql.NullString
		if err := rows.Scan(&item.SubjectKind, &item.SubjectID, &item.Stage, &policyVer, &inputsJSON, &decision, &reasonsJSON, &item.ProducedAt); err != nil {
			s.writeSQLError(w, r, err)
			return
		}
		item.PolicyVer = policyVer.String
		item.Decision = decision.String
		if inputsJSON.Valid && inputsJSON.String != "" {
			_ = json.Unmarshal([]byte(inputsJSON.String), &item.Inputs)
		}
		if reasonsJSON.Valid && reasonsJSON.String != "" {
			_ = json.Unmarshal([]byte(reasonsJSON.String), &item.Reasons)
		}
		items = append(items, item)
	}
	writeList(w, http.StatusOK, items, "")
}

func (s *Server) handleEntityGeometry(w http.ResponseWriter, r *http.Request) {
	_, entityID, ok := s.entityPath(r)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "unknown-entity-type", "entity type is not enabled")
		return
	}
	geometry, err := s.graph.Geometry(r.Context(), entityID)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, geometry)
}

func (s *Server) handleEntityTimeline(w http.ResponseWriter, r *http.Request) {
	_, entityID, ok := s.entityPath(r)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "unknown-entity-type", "entity type is not enabled")
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	after, err := decodeCursor(r.URL.Query().Get("after"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad-cursor", err.Error())
		return
	}
	query := `
		SELECT m.record_id, m.topic, m.topic_family, m.partition, m.offset, m.timestamp, m.envelope_type, m.sender_id,
		       m.correlation_id, m.trace_id, m.task_id, m.parent_task_id, m.status, m.preview, m.content,
		       m.lfs_bucket, m.lfs_key, m.lfs_size, m.lfs_sha256, m.lfs_content_type, m.lfs_created_at, m.lfs_proxy_id
		  FROM messages m
		  JOIN edges e ON e.evidence_msg = m.record_id
		 WHERE (e.src_id = ? OR e.dst_id = ?)
	`
	args := []any{entityID, entityID}
	if after.Timestamp != "" && after.ID != "" {
		query += ` AND (m.timestamp < ? OR (m.timestamp = ? AND m.record_id < ?))`
		args = append(args, after.Timestamp, after.Timestamp, after.ID)
	}
	query += ` ORDER BY m.timestamp DESC, m.record_id DESC LIMIT ?`
	args = append(args, limit+1)
	items, next, err := queryMessages(r.Context(), s.db, query, args...)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeList(w, http.StatusOK, items, next)
}

func (s *Server) handleGraphPath(w http.ResponseWriter, r *http.Request) {
	src := strings.TrimSpace(r.URL.Query().Get("src"))
	dst := strings.TrimSpace(r.URL.Query().Get("dst"))
	if src == "" || dst == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad-request", "src and dst are required")
		return
	}
	maxDepth := parseIntDefault(r.URL.Query().Get("max"), 3)
	window := parseWindow(r.URL.Query().Get("window"))
	path, ok, err := s.graph.Path(r.Context(), src, dst, maxDepth, window)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": ok, "edges": path})
}

func (s *Server) handleMapFeatures(w http.ResponseWriter, r *http.Request) {
	bboxText := strings.TrimSpace(r.URL.Query().Get("bbox"))
	bbox, err := parseBBox(bboxText)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad-bbox", err.Error())
		return
	}
	types := splitCSV(r.URL.Query().Get("types"))
	window := parseWindow(r.URL.Query().Get("window"))
	entities, geometries, err := s.graph.WithinBBox(r.Context(), bbox, types, window)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	type feature struct {
		Type       string          `json:"type"`
		Geometry   json.RawMessage `json:"geometry"`
		Properties map[string]any  `json:"properties"`
	}
	features := make([]feature, 0, len(entities))
	for i := range entities {
		features = append(features, feature{
			Type:     "Feature",
			Geometry: geometries[i].GeoJSON,
			Properties: map[string]any{
				"id":           entities[i].ID,
				"type":         entities[i].Type,
				"canonical_id": entities[i].CanonicalID,
				"display_name": entities[i].DisplayName,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": "FeatureCollection", "features": features})
}

func (s *Server) handleMapLayers(w http.ResponseWriter, r *http.Request) {
	writeList(w, http.StatusOK, s.registry.MapLayers, "")
}

func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	items, next, err := s.store.ListFlows(r.Context(), store.FlowFilter{
		Topic:  strings.TrimSpace(r.URL.Query().Get("topic")),
		Sender: strings.TrimSpace(r.URL.Query().Get("sender")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Text:   strings.TrimSpace(r.URL.Query().Get("q")),
	}, store.Pagination{
		Limit: parseIntDefault(r.URL.Query().Get("limit"), 50),
		After: store.Cursor(strings.TrimSpace(r.URL.Query().Get("after"))),
	})
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad-request", err.Error())
		return
	}
	writeList(w, http.StatusOK, items, string(next))
}

func (s *Server) handleFlow(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetFlow(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleFlowMessages(w http.ResponseWriter, r *http.Request) {
	items, next, err := s.store.ListMessagesForFlow(r.Context(), chi.URLParam(r, "id"), store.Pagination{
		Limit: parseIntDefault(r.URL.Query().Get("limit"), 50),
		After: store.Cursor(strings.TrimSpace(r.URL.Query().Get("after"))),
	})
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad-request", err.Error())
		return
	}
	writeList(w, http.StatusOK, items, string(next))
}

func (s *Server) handleTopicHealth(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.TopicHealth(r.Context())
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeList(w, http.StatusOK, items, "")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.LatestHealth(r.Context())
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleReplays(w http.ResponseWriter, r *http.Request) {
	limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
	items, err := s.store.RecentReplays(r.Context(), limit)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeList(w, http.StatusOK, items, "")
}

func (s *Server) handleReplayRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Topics []string `json:"topics"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	topicsJSON, _ := json.Marshal(body.Topics)
	id := time.Now().UTC().Format("20060102T150405.000000000")
	if _, err := s.writeDB.ExecContext(r.Context(), `
		INSERT INTO replay_requests (id, requested_at, status, topics_json)
		VALUES (?, ?, 'pending', ?)
	`, id, time.Now().UTC().Format(time.RFC3339), string(topicsJSON)); err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":           id,
		"status":       "pending",
		"requested_at": time.Now().UTC().Format(time.RFC3339),
		"topics":       body.Topics,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad-request", "q is required")
		return
	}
	like := "%" + q + "%"
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, type, canonical_id, display_name, last_seen
		  FROM entities
		 WHERE id LIKE ? OR canonical_id LIKE ? OR COALESCE(display_name, '') LIKE ?
		 ORDER BY last_seen DESC, id DESC
		 LIMIT 20
	`, like, like, like)
	if err != nil {
		s.writeSQLError(w, r, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, entityType, canonicalID, lastSeen string
		var displayName sql.NullString
		if err := rows.Scan(&id, &entityType, &canonicalID, &displayName, &lastSeen); err != nil {
			s.writeSQLError(w, r, err)
			return
		}
		items = append(items, map[string]any{
			"kind":         "entity",
			"id":           id,
			"type":         entityType,
			"canonical_id": canonicalID,
			"display_name": displayName.String,
			"last_seen":    lastSeen,
		})
	}
	writeList(w, http.StatusOK, items, "")
}

func (s *Server) handleOntologyTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_types": s.registry.EntityTypes,
		"edge_types":   s.registry.EdgeTypes,
	})
}

func (s *Server) handleOntologyPacks(w http.ResponseWriter, r *http.Request) {
	writeList(w, http.StatusOK, s.registry.Packs, "")
}

func (s *Server) entityPath(r *http.Request) (string, string, bool) {
	entityType := strings.TrimSpace(chi.URLParam(r, "type"))
	if !s.registry.AllowsEntityType(entityType) {
		return "", "", false
	}
	entityID := graph.EntityID(entityType, chi.URLParam(r, "id"))
	if entityID == "" {
		return "", "", false
	}
	return entityType, entityID, true
}

func (s *Server) writeSQLError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeProblem(w, r, http.StatusNotFound, "not-found", "resource not found")
		return
	}
	writeProblem(w, r, http.StatusInternalServerError, "internal-error", err.Error())
}

type cursor struct {
	Timestamp string `json:"timestamp"`
	ID        string `json:"id"`
}

func encodeCursor(ts, id string) string {
	raw, _ := json.Marshal(cursor{Timestamp: ts, ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(raw string) (cursor, error) {
	if strings.TrimSpace(raw) == "" {
		return cursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor{}, err
	}
	var out cursor
	if err := json.Unmarshal(body, &out); err != nil {
		return cursor{}, err
	}
	return out, nil
}

func queryMessages(ctx context.Context, db *sql.DB, query string, args ...any) ([]store.Message, string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []store.Message{}
	limit := args[len(args)-1].(int) - 1
	for rows.Next() {
		var item store.Message
		var envelopeType, senderID, correlationID, traceID, taskID, parentTaskID, status, preview, content sql.NullString
		var lfsBucket, lfsKey, lfsSHA, lfsContentType, lfsCreatedAt, lfsProxyID sql.NullString
		var lfsSize sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Topic, &item.TopicFamily, &item.Partition, &item.Offset, &item.Timestamp, &envelopeType, &senderID, &correlationID, &traceID, &taskID, &parentTaskID, &status, &preview, &content, &lfsBucket, &lfsKey, &lfsSize, &lfsSHA, &lfsContentType, &lfsCreatedAt, &lfsProxyID); err != nil {
			return nil, "", err
		}
		item.EnvelopeType = envelopeType.String
		item.SenderID = senderID.String
		item.CorrelationID = correlationID.String
		item.TraceID = traceID.String
		item.TaskID = taskID.String
		item.ParentTaskID = parentTaskID.String
		item.Status = status.String
		item.Preview = preview.String
		item.Content = content.String
		if lfsBucket.Valid || lfsKey.Valid {
			item.LFS = &store.LFSPointer{
				Bucket:      lfsBucket.String,
				Key:         lfsKey.String,
				Size:        lfsSize.Int64,
				SHA256:      lfsSHA.String,
				ContentType: lfsContentType.String,
				CreatedAt:   lfsCreatedAt.String,
				ProxyID:     lfsProxyID.String,
				Path:        "s3://" + lfsBucket.String + "/" + lfsKey.String,
			}
		}
		items = append(items, item)
	}
	next := ""
	if len(items) > limit {
		last := items[limit-1]
		next = encodeCursor(last.Timestamp, last.ID)
		items = items[:limit]
	}
	return items, next, rows.Err()
}

func parseIntDefault(raw string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && value > 0 {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseWindow(raw string) graph.TimeRange {
	window := strings.TrimSpace(raw)
	if window == "" {
		return graph.TimeRange{}
	}
	if d, err := time.ParseDuration(window); err == nil {
		return graph.TimeRange{Start: time.Now().UTC().Add(-d).Format(time.RFC3339)}
	}
	return graph.TimeRange{}
}

func parseBBox(raw string) ([4]float64, error) {
	parts := splitCSV(raw)
	if len(parts) != 4 {
		return [4]float64{}, fmt.Errorf("bbox must be minLon,minLat,maxLon,maxLat")
	}
	out := [4]float64{}
	for i, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return [4]float64{}, err
		}
		out[i] = value
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeList(w http.ResponseWriter, status int, items any, next string) {
	payload := map[string]any{"items": items, "next": nil}
	if next != "" {
		payload["next"] = next
	}
	writeJSON(w, status, payload)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, slug string, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{
		Type:     "/docs/agentops-api-errors.md#" + slug,
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	})
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func storePathForWrite(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		return strings.TrimSuffix(path, ".json") + ".db"
	}
	return path
}
