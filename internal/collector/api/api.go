// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
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
	"time"

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
	mux.HandleFunc("GET /api/health", s.handleHealth)
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      cors(mux),
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
	writeJSON(w, http.StatusOK, map[string]any{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
