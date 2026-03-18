// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
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

// CaptureJSONResponses opens a page in headless Chrome and collects JSON
// XHR/fetch responses whose URL contains the given substring.
func (b *BrowserClient) CaptureJSONResponses(ctx context.Context, pageURL string, urlContains string) ([][]byte, error) {
	timeout := time.Duration(b.timeoutMS) * time.Millisecond
	taskCtx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
	defer cancelTimeout()

	var (
		mu         sync.Mutex
		seen       = map[network.RequestID]string{}
		bodies     [][]byte
		captureErr error
	)

	chromedp.ListenTarget(taskCtx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			if !strings.Contains(e.Response.URL, urlContains) {
				return
			}
			if e.Type != network.ResourceTypeXHR && e.Type != network.ResourceTypeFetch {
				return
			}
			mu.Lock()
			seen[e.RequestID] = e.Response.URL
			mu.Unlock()
		case *network.EventLoadingFinished:
			mu.Lock()
			_, ok := seen[e.RequestID]
			mu.Unlock()
			if !ok {
				return
			}
			go func(requestID network.RequestID) {
				var body []byte
				err := chromedp.Run(taskCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					data, err := network.GetResponseBody(requestID).Do(ctx)
					if err != nil {
						return err
					}
					body = data
					return nil
				}))
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if captureErr == nil {
						captureErr = err
					}
					return
				}
				if len(body) > 0 {
					bodies = append(bodies, body)
				}
			}(e.RequestID)
		}
	})

	if err := chromedp.Run(taskCtx,
		network.Enable(),
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return nil, fmt.Errorf("browser capture %s: %w", pageURL, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) == 0 && captureErr != nil {
		return nil, fmt.Errorf("browser capture %s: %w", pageURL, captureErr)
	}
	if len(bodies) == 0 {
		return nil, fmt.Errorf("browser capture %s: no matching JSON responses", pageURL)
	}
	return bodies, nil
}

// Close shuts down the browser allocator and releases Chrome processes.
func (b *BrowserClient) Close() {
	if b.cancelCtx != nil {
		b.cancelCtx()
	}
}
