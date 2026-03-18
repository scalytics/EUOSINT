// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import "context"

// Fetcher is the common interface for fetching page content as text.
// Both the stealth HTTP Client and the headless BrowserClient satisfy it.
type Fetcher interface {
	Text(ctx context.Context, url string, followRedirects bool, accept string) ([]byte, error)
}

// FetcherFor returns the appropriate Fetcher for the given fetch mode.
// When mode is "browser" and a BrowserClient is available, the browser
// fetcher is returned. Otherwise the stealth HTTP client is used.
func FetcherFor(mode string, client *Client, browser *BrowserClient) Fetcher {
	if mode == "browser" && browser != nil {
		return browser
	}
	return client
}

// PrefetchedFetcher wraps a Fetcher and returns a pre-fetched body for
// the first Text() call that matches the cached URL. Subsequent calls or
// calls for different URLs pass through to the inner Fetcher.
// This avoids double-fetching when a conditional GET already retrieved
// the full response body.
type PrefetchedFetcher struct {
	Inner    Fetcher
	URL      string
	Body     []byte
	consumed bool
}

func (f *PrefetchedFetcher) Text(ctx context.Context, url string, followRedirects bool, accept string) ([]byte, error) {
	if !f.consumed && f.Body != nil && url == f.URL {
		f.consumed = true
		return f.Body, nil
	}
	return f.Inner.Text(ctx, url, followRedirects, accept)
}
