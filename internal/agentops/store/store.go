// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
)

type Store interface {
	ListFlows(context.Context, FlowFilter, Pagination) ([]Flow, Cursor, error)
	GetFlow(context.Context, string) (Flow, error)
	ListMessagesForFlow(context.Context, string, Pagination) ([]Message, Cursor, error)
	ListTracesForFlow(context.Context, string) ([]Trace, error)
	ListTasksForFlow(context.Context, string) ([]Task, error)
	TopicHealth(context.Context) ([]TopicHealth, error)
	RecentReplays(context.Context, int) ([]ReplaySession, error)
	LatestHealth(context.Context) (Health, error)
	Close() error
}

type WriteStore interface {
	Store
	Snapshot() Snapshot
	Update(func(*Snapshot)) error
	Apply(func(*sql.Tx) error) error
}

var _ WriteStore = (*SqliteStore)(nil)
