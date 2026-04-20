// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package store

// SqliteStore is the SQLite-backed AgentOps persistence layer.
// FileStore remains as a compatibility alias while the runtime and tests
// transition away from the old JSON-store naming.
type SqliteStore = FileStore

func NewSqliteStore(path string, initial Snapshot) (*SqliteStore, error) {
	return NewFileStore(path, initial)
}
