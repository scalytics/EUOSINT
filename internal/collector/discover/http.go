// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"context"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/fetch"
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
