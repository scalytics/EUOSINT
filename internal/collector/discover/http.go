// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/config"
	"github.com/scalytics/kafSIEM/internal/collector/fetch"
)

func fetchTextWithRetry(ctx context.Context, client *fetch.Client, url string, accept string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		attemptCtx := ctx
		cancel := func() {}
		if _, ok := ctx.Deadline(); !ok {
			attemptCtx, cancel = context.WithTimeout(ctx, 45*time.Second)
		}
		body, err := client.Text(attemptCtx, url, true, accept)
		cancel()
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !looksTransient(err) || attempt == 1 {
			break
		}
	}
	return nil, lastErr
}

func looksTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "request canceled") ||
		strings.Contains(msg, " eof") ||
		strings.Contains(msg, ": eof")
}

func fetchWikidataTextWithCache(ctx context.Context, cfg config.Config, client *fetch.Client, url string, accept string) ([]byte, error) {
	if body, ok := readWikidataCache(cfg, url); ok {
		return body, nil
	}
	// Use Wikimedia-specific headers only for Wikidata/Wikimedia URLs.
	var headers map[string]string
	if strings.Contains(url, "wikidata.org") || strings.Contains(url, "wikimedia.org") {
		headers = map[string]string{
			"User-Agent":     cfg.WikimediaUserAgent,
			"Api-User-Agent": cfg.WikimediaUserAgent,
		}
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		attemptCtx := ctx
		cancel := func() {}
		if _, ok := ctx.Deadline(); !ok {
			attemptCtx, cancel = context.WithTimeout(ctx, 45*time.Second)
		}
		var body []byte
		var err error
		if headers != nil {
			body, err = client.TextWithHeaders(attemptCtx, url, true, accept, headers)
		} else {
			body, err = client.Text(attemptCtx, url, true, accept)
		}
		cancel()
		if err == nil {
			writeWikidataCache(cfg, url, body)
			return body, nil
		}
		lastErr = err
		if !looksTransient(err) || attempt == 1 {
			break
		}
	}
	return nil, lastErr
}

func readWikidataCache(cfg config.Config, url string) ([]byte, bool) {
	path := wikidataCacheFile(cfg, url)
	if path == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	ttl := time.Duration(cfg.WikidataCacheTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if time.Since(info.ModTime()) > ttl {
		return nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return nil, false
	}
	return body, true
}

func writeWikidataCache(cfg config.Config, url string, body []byte) {
	path := wikidataCacheFile(cfg, url)
	if path == "" || len(body) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o644)
}

func wikidataCacheFile(cfg config.Config, url string) string {
	dir := strings.TrimSpace(cfg.WikidataCachePath)
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
}
