// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	driverName = "sqlite"
	memoryDSN  = "file:agentops?mode=memory&cache=shared"
)

//go:embed schema.sql
var files embed.FS

func Open(path string) (*sql.DB, error) {
	dsn := strings.TrimSpace(path)
	if dsn == "" {
		dsn = memoryDSN
	} else if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
		return nil, fmt.Errorf("create schema dir: %w", err)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := configure(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := Apply(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Apply(db *sql.DB) error {
	raw, err := files.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema.sql: %w", err)
	}
	if _, err := db.Exec(string(raw)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

func configure(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, stmt := range pragmas {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}
