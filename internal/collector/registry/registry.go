// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/sourcedb"
)

func Load(path string) ([]model.RegistrySource, error) {
	if isSQLitePath(path) {
		db, err := sourcedb.Open(path)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		raw, err := db.LoadActiveSources(context.Background())
		if err != nil {
			return nil, fmt.Errorf("load registry from source DB %s: %w", path, err)
		}
		return normalizeAll(raw), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", path, err)
	}

	var raw []model.RegistrySource
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode registry %s: %w", path, err)
	}

	return normalizeAll(raw), nil
}

func normalize(entry model.RegistrySource) (model.RegistrySource, bool) {
	entry.Type = strings.TrimSpace(entry.Type)
	entry.Category = strings.TrimSpace(entry.Category)
	entry.RegionTag = strings.TrimSpace(entry.RegionTag)
	entry.FeedURL = strings.TrimSpace(entry.FeedURL)
	entry.Source.SourceID = strings.TrimSpace(entry.Source.SourceID)
	entry.Source.AuthorityName = strings.TrimSpace(entry.Source.AuthorityName)
	entry.Source.Country = fallback(entry.Source.Country, "Unknown")
	entry.Source.CountryCode = fallback(strings.ToUpper(entry.Source.CountryCode), "XX")
	entry.Source.Region = fallback(entry.Source.Region, "International")
	entry.Source.AuthorityType = fallback(entry.Source.AuthorityType, "public_safety_program")
	entry.Source.BaseURL = fallback(entry.Source.BaseURL, entry.FeedURL)
	if entry.Type == "" || entry.Category == "" || entry.Source.SourceID == "" || entry.Source.AuthorityName == "" {
		return model.RegistrySource{}, false
	}
	if entry.FeedURL == "" && len(entry.FeedURLs) == 0 {
		return model.RegistrySource{}, false
	}
	if entry.MaxItems <= 0 {
		entry.MaxItems = 20
	}
	return entry, true
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func normalizeAll(raw []model.RegistrySource) []model.RegistrySource {
	seen := make(map[string]struct{}, len(raw))
	out := make([]model.RegistrySource, 0, len(raw))
	for _, entry := range raw {
		normalized, ok := normalize(entry)
		if !ok {
			continue
		}
		if _, exists := seen[normalized.Source.SourceID]; exists {
			continue
		}
		seen[normalized.Source.SourceID] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Source.SourceID < out[j].Source.SourceID
	})
	return out
}

func isSQLitePath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	default:
		return false
	}
}
