// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package store

// Store is the current persistence contract used by the AgentOps runtime.
// It intentionally stays narrow while the runtime still mutates a whole
// snapshot document. W1 will replace the backing implementation; later
// workstreams can split this into read/write surfaces and typed queries.
type Store interface {
	Snapshot() Snapshot
	Update(func(*Snapshot)) error
}

var _ Store = (*FileStore)(nil)
