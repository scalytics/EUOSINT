// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"fmt"
	"strings"
)

func EntityID(typ, canonicalID string) string {
	typ = strings.TrimSpace(strings.ToLower(typ))
	canonicalID = strings.TrimSpace(canonicalID)
	if typ == "" || canonicalID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", typ, canonicalID)
}
