// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"database/sql"
	_ "embed"
)

//go:embed schema.sql
var ddl string

func Apply(db *sql.DB) error {
	_, err := db.Exec(ddl)
	return err
}
