// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/trends"
	"github.com/scalytics/euosint/internal/collector/vet"
	"github.com/scalytics/euosint/internal/sourcedb"
)

// Server serves the search API backed by SQLite FTS5.
type Server struct {
	db     *sourcedb.DB
	addr   string
	srv    *http.Server
	stderr io.Writer
	llmCfg ZoneBriefLLMConfig
}

type ZoneBriefLLMConfig struct {
	RuntimeDir         string
	VettingTimeoutMS   int
	VettingBaseURL     string
	VettingAPIKey      string
	VettingProvider    string
	VettingModel       string
	VettingTemperature float64
}

func New(db *sourcedb.DB, addr string, stderr io.Writer) *Server {
	s := &Server{db: db, addr: addr, stderr: stderr}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/digest", s.handleDigest)
	mux.HandleFunc("GET /api/noise-feedback/stats", s.handleNoiseFeedbackStats)
	mux.HandleFunc("POST /api/noise-feedback", s.handleNoiseFeedbackCreate)
	mux.HandleFunc("POST /api/zone-brief-llm", s.handleZoneBriefLLM)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	rl := newRateLimiter(30, 5, 10*time.Minute) // 30 requests burst, 5/sec refill
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      cors(rateLimit(rl, mux)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 120 * time.Second,
	}
	return s
}

func (s *Server) ConfigureZoneBriefLLM(cfg ZoneBriefLLMConfig) {
	s.llmCfg = cfg
}

// Start begins listening in a goroutine. Returns once the listener is bound.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("api listen %s: %w", s.addr, err)
	}
	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(s.stderr, "API server error: %v\n", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	region := strings.TrimSpace(r.URL.Query().Get("region"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	lane := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lane")))
	includeFiltered := parseBoolQuery(r, "include_filtered")
	includeRemoved := parseBoolQuery(r, "include_removed")
	includeLocal := parseBoolQuery(r, "include_local")
	limitStr := strings.TrimSpace(r.URL.Query().Get("limit"))

	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	if q == "" && category == "" && region == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q, category, or region parameter required"})
		return
	}
	if status == "" && !includeFiltered && !includeRemoved {
		// Intelligence-default view: active/current only.
		status = "active"
	}

	// Sanitize FTS query: if user passes bare text without operators, wrap words for prefix matching.
	ftsQuery := q
	if q != "" && !strings.ContainsAny(q, `"*()`) && !strings.Contains(q, " OR ") && !strings.Contains(q, " AND ") && !strings.Contains(q, " NOT ") {
		words := strings.Fields(q)
		parts := make([]string, len(words))
		for i, w := range words {
			parts[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"` + "*"
		}
		ftsQuery = strings.Join(parts, " ")
	}

	results, err := s.db.SearchAlerts(r.Context(), ftsQuery, category, region, status, limit)
	if err != nil {
		// FTS parse error — try as plain phrase.
		if strings.Contains(err.Error(), "fts5") || strings.Contains(err.Error(), "syntax") {
			phrase := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
			results, err = s.db.SearchAlerts(r.Context(), phrase, category, region, status, limit)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	results = filterStatuses(results, status, includeFiltered, includeRemoved)
	results = filterLane(results, lane)
	// Global/default search suppresses local law-enforcement noise.
	// Country-scoped search (region filter) keeps local sources visible.
	results = filterLocalLawEnforcement(results, includeLocal || region != "")
	writeJSON(w, http.StatusOK, map[string]any{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func parseBoolQuery(r *http.Request, key string) bool {
	v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	return v == "1" || v == "true" || v == "yes"
}

func filterStatuses(items []model.Alert, explicitStatus string, includeFiltered bool, includeRemoved bool) []model.Alert {
	if explicitStatus != "" {
		return items
	}
	out := make([]model.Alert, 0, len(items))
	for _, a := range items {
		switch strings.ToLower(strings.TrimSpace(a.Status)) {
		case "filtered":
			if includeFiltered {
				out = append(out, a)
			}
		case "removed":
			if includeRemoved {
				out = append(out, a)
			}
		default:
			out = append(out, a)
		}
	}
	return out
}

func filterLane(items []model.Alert, lane string) []model.Alert {
	lane = strings.ToLower(strings.TrimSpace(lane))
	if lane == "" || lane == "all" {
		return items
	}
	out := make([]model.Alert, 0, len(items))
	for _, a := range items {
		switch lane {
		case "alarm", "intel", "info":
			if string(a.SignalLane) == lane {
				out = append(out, a)
			}
		default:
			return items
		}
	}
	return out
}

func filterLocalLawEnforcement(items []model.Alert, includeLocal bool) []model.Alert {
	if includeLocal {
		return items
	}
	out := make([]model.Alert, 0, len(items))
	for _, a := range items {
		if isLocalLawEnforcement(a) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func isLocalLawEnforcement(a model.Alert) bool {
	authorityType := strings.ToLower(strings.TrimSpace(a.Source.AuthorityType))
	switch authorityType {
	case "police", "law_enforcement", "gendarmerie", "sheriff", "border_guard":
	default:
		return false
	}
	level := strings.ToLower(strings.TrimSpace(a.Source.Level))
	scope := strings.ToLower(strings.TrimSpace(a.Source.Scope))
	if level == "local" || scope == "local" {
		return true
	}
	if level == "regional" || scope == "regional" || scope == "state" || scope == "provincial" {
		return true
	}
	return strings.TrimSpace(a.Source.JurisdictionName) != ""
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleNoiseFeedbackCreate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AlertID  string `json:"alert_id"`
		SourceID string `json:"source_id,omitempty"`
		Verdict  string `json:"verdict"`
		Analyst  string `json:"analyst,omitempty"`
		Notes    string `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}
	if err := s.db.SaveNoiseFeedback(r.Context(), sourcedb.NoiseFeedbackInput{
		AlertID:  payload.AlertID,
		SourceID: payload.SourceID,
		Verdict:  payload.Verdict,
		Analyst:  payload.Analyst,
		Notes:    payload.Notes,
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "stored",
	})
}

func (s *Server) handleNoiseFeedbackStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.NoiseFeedbackStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleDigest returns the country intelligence digest — top trending
// terms per country, ranked by trust-weighted frequency.
//
//	GET /api/digest?cc=PT&days=7&limit=10  → single country
//	GET /api/digest?days=7&limit=10        → all countries
func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	cc := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("cc")))
	daysStr := strings.TrimSpace(r.URL.Query().Get("days"))
	limitStr := strings.TrimSpace(r.URL.Query().Get("limit"))

	days := 7
	if daysStr != "" {
		if n, err := strconv.Atoi(daysStr); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	limit := 10
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	detector := trends.New(s.db.RawDB())
	if err := detector.Init(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	if cc != "" {
		terms, err := detector.CountryDigestQuery(r.Context(), cc, now, days, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if terms == nil {
			terms = []trends.DigestTerm{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"country_code": cc,
			"days":         days,
			"terms":        terms,
		})
		return
	}

	digests, err := detector.AllCountryDigests(r.Context(), now, days, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if digests == nil {
		digests = []trends.CountryDigest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":    days,
		"count":   len(digests),
		"digests": digests,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleZoneBriefLLM(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.llmCfg.VettingAPIKey) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "source vetting API key missing"})
		return
	}

	var payload struct {
		ConflictID string `json:"conflict_id"`
		CountryID  string `json:"country_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}
	payload.ConflictID = strings.TrimSpace(payload.ConflictID)
	payload.CountryID = strings.TrimSpace(payload.CountryID)
	if payload.ConflictID == "" && payload.CountryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conflict_id or country_id is required"})
		return
	}

	statsPath := filepath.Join(strings.TrimSpace(s.llmCfg.RuntimeDir), "ucdp-conflict-stats.json")
	stats, err := readConflictStatsArtifact(statsPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	row, idx := findConflictStat(stats, payload.ConflictID, payload.CountryID)
	if idx < 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conflict not found in runtime stats"})
		return
	}

	narrative, refreshedHistorical, refreshedAnalysis, err := s.ensureZoneBriefLLM(r.Context(), row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	stats[idx].HistoricalSummary = strings.TrimSpace(narrative.HistoricalSummary)
	stats[idx].CurrentAnalysis = strings.TrimSpace(narrative.CurrentAnalysis)
	stats[idx].AnalysisUpdatedAt = strings.TrimSpace(narrative.AnalysisUpdatedAt)
	if err := writeConflictStatsArtifact(statsPath, stats); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"country_id":           narrative.CountryID,
		"title":                narrative.Title,
		"historical_summary":   narrative.HistoricalSummary,
		"current_analysis":     narrative.CurrentAnalysis,
		"analysis_updated_at":  narrative.AnalysisUpdatedAt,
		"refreshed_historical": refreshedHistorical,
		"refreshed_analysis":   refreshedAnalysis,
	})
}

type conflictStatRow struct {
	ConflictID        string                `json:"conflict_id"`
	CountryID         string                `json:"country_id"`
	Title             string                `json:"title"`
	Year              int                   `json:"year"`
	StartDate         string                `json:"start_date"`
	TypeOfConflict    string                `json:"type_of_conflict"`
	SideA             string                `json:"side_a"`
	SideB             string                `json:"side_b"`
	FatalitiesTotal   int                   `json:"fatalities_total"`
	FatalitiesLatest  int                   `json:"fatalities_latest_year"`
	FatalitiesYear    int                   `json:"fatalities_latest_year_year"`
	RecentEvents      []conflictRecentEvent `json:"recent_events"`
	HistoricalSummary string                `json:"historical_summary"`
	CurrentAnalysis   string                `json:"current_analysis"`
	AnalysisUpdatedAt string                `json:"analysis_updated_at"`
}

type conflictRecentEvent struct {
	Date       string `json:"date"`
	Title      string `json:"title"`
	Location   string `json:"location"`
	Fatalities int    `json:"fatalities"`
}

func readConflictStatsArtifact(path string) ([]conflictStatRow, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("runtime stats path missing")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read conflict stats artifact: %w", err)
	}
	var out []conflictStatRow
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode conflict stats artifact: %w", err)
	}
	return out, nil
}

func writeConflictStatsArtifact(path string, rows []conflictStatRow) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("runtime stats path missing")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir runtime stats dir: %w", err)
	}
	payload, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal conflict stats artifact: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write conflict stats temp artifact: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace conflict stats artifact: %w", err)
	}
	return nil
}

func findConflictStat(rows []conflictStatRow, conflictID, countryID string) (conflictStatRow, int) {
	conflictID = strings.TrimSpace(conflictID)
	countryID = strings.TrimSpace(countryID)
	if conflictID != "" {
		for i, row := range rows {
			if strings.TrimSpace(row.ConflictID) == conflictID {
				return row, i
			}
		}
	}
	if countryID != "" {
		for i, row := range rows {
			if strings.TrimSpace(row.CountryID) == countryID {
				return row, i
			}
		}
	}
	return conflictStatRow{}, -1
}

func (s *Server) ensureZoneBriefLLM(ctx context.Context, row conflictStatRow) (sourcedb.ZoneBriefLLM, bool, bool, error) {
	countryID := strings.TrimSpace(row.CountryID)
	if countryID == "" {
		return sourcedb.ZoneBriefLLM{}, false, false, fmt.Errorf("country_id missing for selected conflict")
	}
	existing, ok, err := s.db.GetZoneBriefLLM(ctx, countryID)
	if err != nil {
		return sourcedb.ZoneBriefLLM{}, false, false, err
	}
	if !ok {
		existing = sourcedb.ZoneBriefLLM{CountryID: countryID, Title: strings.TrimSpace(row.Title)}
	}
	needsHistorical := strings.TrimSpace(existing.HistoricalSummary) == ""
	needsAnalysis := strings.TrimSpace(existing.CurrentAnalysis) == ""
	if !needsAnalysis {
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(existing.AnalysisUpdatedAt))
		if err != nil || time.Since(ts) >= 7*24*time.Hour {
			needsAnalysis = true
		}
	}
	if !needsHistorical && !needsAnalysis {
		return existing, false, false, nil
	}

	llm := vet.NewClient(config.Config{
		VettingTimeoutMS:   s.llmCfg.VettingTimeoutMS,
		VettingBaseURL:     s.llmCfg.VettingBaseURL,
		VettingAPIKey:      s.llmCfg.VettingAPIKey,
		VettingProvider:    s.llmCfg.VettingProvider,
		VettingModel:       s.llmCfg.VettingModel,
		VettingTemperature: s.llmCfg.VettingTemperature,
	})
	baseContext := fmt.Sprintf(
		"Zone: %s\nCountry ID: %s\nSides: %s | %s\nConflict start: %s\nConflict type: %s\nTotal deaths (all years): %d\nDeaths latest year (%d): %d",
		strings.TrimSpace(row.Title),
		countryID,
		strings.TrimSpace(row.SideA),
		strings.TrimSpace(row.SideB),
		strings.TrimSpace(row.StartDate),
		strings.TrimSpace(row.TypeOfConflict),
		row.FatalitiesTotal,
		row.FatalitiesYear,
		row.FatalitiesLatest,
	)
	zoneLabel := strings.TrimSpace(row.Title)
	if strings.TrimSpace(row.SideA) != "" && strings.TrimSpace(row.SideB) != "" {
		zoneLabel = strings.TrimSpace(row.SideA) + " vs " + strings.TrimSpace(row.SideB)
	}
	if len(row.RecentEvents) > 0 {
		eventLines := make([]string, 0, 3)
		for i, event := range row.RecentEvents {
			if i >= 3 {
				break
			}
			line := strings.TrimSpace(event.Date) + " | " + strings.TrimSpace(event.Location) + " | " + strings.TrimSpace(event.Title)
			if event.Fatalities > 0 {
				line += fmt.Sprintf(" | fatalities=%d", event.Fatalities)
			}
			eventLines = append(eventLines, line)
		}
		if len(eventLines) > 0 {
			baseContext += "\nRecent events:\n- " + strings.Join(eventLines, "\n- ")
		}
	}

	refreshedHistorical := false
	refreshedAnalysis := false
	now := time.Now().UTC().Format(time.RFC3339)

	if needsHistorical {
		resp, err := llm.Complete(ctx, []vet.Message{
			{Role: "system", Content: "You are an OSINT analyst. Return plain text only. Facts only. Neutral tone. No fluff. No speculation."},
			{Role: "user", Content: "Short historic summary about conflict zone " + zoneLabel + " in max 80 words and current analysis in max 60 words.\nNow return only the historic summary block.\nConstraints: factual only, no bullets, no disclaimers, no filler.\n" + baseContext},
		})
		if err != nil {
			return existing, false, false, fmt.Errorf("generate historic summary: %w", err)
		}
		text := strings.TrimSpace(resp)
		if text != "" {
			existing.HistoricalSummary = limitWords(text, 80)
			existing.HistoricalUpdatedAt = now
			refreshedHistorical = true
		}
	}
	if needsAnalysis {
		resp, err := llm.Complete(ctx, []vet.Message{
			{Role: "system", Content: "You are an OSINT analyst. Return plain text only. Facts only. Neutral tone. No fluff. No speculation."},
			{Role: "user", Content: "Short historic summary about conflict zone " + zoneLabel + " in max 80 words and current analysis in max 60 words.\nNow return only the current analysis block.\nConstraints: factual only, no bullets, no disclaimers, no filler.\nFocus only on current dynamics (roughly last 6-12 months): momentum, intensity direction, territorial/control shifts, and near-term operational outlook.\nDo NOT repeat conflict start date or cumulative death totals from historical summary.\nIf recent evidence is weak, give a cautious best-available assessment from the provided context.\nAs-of date: " + time.Now().UTC().Format("2006-01-02") + "\n" + baseContext},
		})
		if err != nil {
			return existing, refreshedHistorical, false, fmt.Errorf("generate current analysis: %w", err)
		}
		text := strings.TrimSpace(resp)
		if text != "" {
			existing.CurrentAnalysis = limitWords(text, 60)
			existing.AnalysisUpdatedAt = now
			refreshedAnalysis = true
		}
	}
	if err := s.db.UpsertZoneBriefLLM(ctx, existing); err != nil {
		return existing, refreshedHistorical, refreshedAnalysis, err
	}
	return existing, refreshedHistorical, refreshedAnalysis, nil
}

func limitWords(text string, maxWords int) string {
	if maxWords <= 0 {
		return ""
	}
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return strings.TrimSpace(text)
	}
	return strings.Join(words[:maxWords], " ")
}

// ---------- per-IP token bucket rate limiter ----------

type ipBucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiterState struct {
	mu       sync.Mutex
	buckets  map[string]*ipBucket
	burst    float64
	rate     float64 // tokens per second
	staleAge time.Duration
}

func newRateLimiter(burst int, perSecond float64, staleAge time.Duration) *rateLimiterState {
	rl := &rateLimiterState{
		buckets:  make(map[string]*ipBucket),
		burst:    float64(burst),
		rate:     perSecond,
		staleAge: staleAge,
	}
	go rl.evictLoop()
	return rl
}

func (rl *rateLimiterState) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{tokens: rl.burst, lastSeen: now}
		rl.buckets[ip] = b
	}
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *rateLimiterState) evictLoop() {
	ticker := time.NewTicker(rl.staleAge)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.staleAge)
		for ip, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func rateLimit(rl *rateLimiterState, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip rate limiting for health checks.
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	// Trust X-Forwarded-For from Caddy reverse proxy.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
