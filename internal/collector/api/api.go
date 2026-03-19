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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/trends"
	"github.com/scalytics/euosint/internal/sourcedb"
)

// Server serves the search API backed by SQLite FTS5.
type Server struct {
	db     *sourcedb.DB
	addr   string
	srv    *http.Server
	stderr io.Writer
}

func New(db *sourcedb.DB, addr string, stderr io.Writer) *Server {
	s := &Server{db: db, addr: addr, stderr: stderr}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/digest", s.handleDigest)
	mux.HandleFunc("GET /api/noise-feedback/stats", s.handleNoiseFeedbackStats)
	mux.HandleFunc("POST /api/noise-feedback", s.handleNoiseFeedbackCreate)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	rl := newRateLimiter(30, 5, 10*time.Minute) // 30 requests burst, 5/sec refill
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      cors(rateLimit(rl, mux)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return s
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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
