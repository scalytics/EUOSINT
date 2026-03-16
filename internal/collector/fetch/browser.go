// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// BrowserClient fetches page content by driving a headless Chrome instance
// via chromedp. This is used for sites that block even stealth HTTP clients
// (e.g., government sites with aggressive bot detection).
type BrowserClient struct {
	allocCtx  context.Context
	cancelCtx context.CancelFunc
	timeoutMS int
}

// NewBrowser creates a BrowserClient with a shared headless Chrome allocator.
// Call Close() when done to release browser resources.
func NewBrowser(timeoutMS int) (*BrowserClient, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return &BrowserClient{
		allocCtx:  allocCtx,
		cancelCtx: cancel,
		timeoutMS: timeoutMS,
	}, nil
}

// Text navigates to the URL, waits for the network to become idle, and
// returns the full page HTML as bytes. The followRedirects and accept
// parameters are accepted for interface compatibility but Chrome handles
// redirects natively and always sends its own Accept header.
func (b *BrowserClient) Text(ctx context.Context, url string, followRedirects bool, accept string) ([]byte, error) {
	timeout := time.Duration(b.timeoutMS) * time.Millisecond
	taskCtx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
	defer cancelTimeout()

	var html string
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return nil, fmt.Errorf("browser fetch %s: %w", url, err)
	}
	return []byte(html), nil
}

// Close shuts down the browser allocator and releases Chrome processes.
func (b *BrowserClient) Close() {
	if b.cancelCtx != nil {
		b.cancelCtx()
	}
}
