package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ReadStore struct {
	db *sql.DB
}

func NewReadStore(db *sql.DB) *ReadStore {
	return &ReadStore{db: db}
}

func OpenReadOnly(path string) (*ReadStore, error) {
	dbPath := sqlitePath(path)
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("open read-only store: empty path")
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000&_journal_mode=WAL", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &ReadStore{db: db}, nil
}

func (s *ReadStore) Close() error {
	return s.db.Close()
}

func (s *ReadStore) DB() *sql.DB {
	return s.db
}

func (s *ReadStore) ListFlows(ctx context.Context, filter FlowFilter, page Pagination) ([]Flow, Cursor, error) {
	return (&SqliteStore{db: s.db}).ListFlows(ctx, filter, page)
}

func (s *ReadStore) GetFlow(ctx context.Context, id string) (Flow, error) {
	return (&SqliteStore{db: s.db}).GetFlow(ctx, id)
}

func (s *ReadStore) ListMessagesForFlow(ctx context.Context, id string, page Pagination) ([]Message, Cursor, error) {
	return (&SqliteStore{db: s.db}).ListMessagesForFlow(ctx, id, page)
}

func (s *ReadStore) ListTracesForFlow(ctx context.Context, id string) ([]Trace, error) {
	return (&SqliteStore{db: s.db}).ListTracesForFlow(ctx, id)
}

func (s *ReadStore) ListTasksForFlow(ctx context.Context, id string) ([]Task, error) {
	return (&SqliteStore{db: s.db}).ListTasksForFlow(ctx, id)
}

func (s *ReadStore) TopicHealth(ctx context.Context) ([]TopicHealth, error) {
	return (&SqliteStore{db: s.db}).TopicHealth(ctx)
}

func (s *ReadStore) RecentReplays(ctx context.Context, limit int) ([]ReplaySession, error) {
	return (&SqliteStore{db: s.db}).RecentReplays(ctx, limit)
}

func (s *ReadStore) LatestHealth(ctx context.Context) (Health, error) {
	return (&SqliteStore{db: s.db}).LatestHealth(ctx)
}

var _ Store = (*ReadStore)(nil)
