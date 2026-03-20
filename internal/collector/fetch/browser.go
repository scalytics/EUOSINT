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

const defaultBrowserWSURL = "ws://browser:3000"

var (
	initRemoteAllocatorFn = initRemoteAllocator
	localAllocatorFn      = localAllocator
)

type BrowserOptions struct {
	TimeoutMS           int
	WSURL               string
	MaxConcurrency      int
	ConnectRetries      int
	ConnectRetryDelayMS int
}

// BrowserClient fetches page content by driving a headless Chrome instance
// via chromedp. This is used for sites that block even stealth HTTP clients
// (e.g., government sites with aggressive bot detection).
type BrowserClient struct {
	allocCtx  context.Context
	cancelCtx context.CancelFunc
	timeoutMS int
	sem       chan struct{}
	warning   string
}

// NewBrowser creates a BrowserClient with a shared headless Chrome allocator.
// Call Close() when done to release browser resources.
func NewBrowser(opts BrowserOptions) (*BrowserClient, error) {
	timeoutMS := opts.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}

	wsURL := strings.TrimSpace(opts.WSURL)
	if wsURL == "" {
		wsURL = defaultBrowserWSURL
	}
	connectRetries := opts.ConnectRetries
	if connectRetries <= 0 {
		connectRetries = 3
	}
	retryDelay := time.Duration(opts.ConnectRetryDelayMS) * time.Millisecond
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	maxConcurrency := opts.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	allocCtx, cancel, remoteErr := initRemoteAllocatorFn(wsURL, timeoutMS, connectRetries, retryDelay)
	warning := ""
	if remoteErr != nil {
		warning = fmt.Sprintf("remote browser unreachable (%v); falling back to local chromium", remoteErr)
		allocCtx, cancel = localAllocatorFn()
	}

	return &BrowserClient{
		allocCtx:  allocCtx,
		cancelCtx: cancel,
		timeoutMS: timeoutMS,
		sem:       make(chan struct{}, maxConcurrency),
		warning:   warning,
	}, nil
}

func initRemoteAllocator(wsURL string, timeoutMS int, connectRetries int, retryDelay time.Duration) (context.Context, context.CancelFunc, error) {
	var lastErr error
	for i := 1; i <= connectRetries; i++ {
		allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
		taskCtx, taskCancel := chromedp.NewContext(allocCtx)
		probeTimeout := time.Duration(timeoutMS) * time.Millisecond
		if probeTimeout > 10*time.Second {
			probeTimeout = 10 * time.Second
		}
		if probeTimeout <= 0 {
			probeTimeout = 5 * time.Second
		}
		probeCtx, probeCancel := context.WithTimeout(taskCtx, probeTimeout)
		err := chromedp.Run(probeCtx, chromedp.Navigate("about:blank"))
		probeCancel()
		taskCancel()
		if err == nil {
			return allocCtx, cancel, nil
		}
		cancel()
		lastErr = err
		if i < connectRetries {
			time.Sleep(retryDelay)
		}
	}
	return nil, nil, lastErr
}

func localAllocator() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	return chromedp.NewExecAllocator(context.Background(), opts...)
}

func (b *BrowserClient) Warning() string {
	return strings.TrimSpace(b.warning)
}

func (b *BrowserClient) acquire(ctx context.Context) error {
	if b.sem == nil {
		return nil
	}
	select {
	case b.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *BrowserClient) release() {
	if b.sem == nil {
		return
	}
	select {
	case <-b.sem:
	default:
	}
}

func (b *BrowserClient) newTaskContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, context.CancelFunc, error) {
	if err := b.acquire(ctx); err != nil {
		return nil, nil, nil, err
	}
	taskCtx, cancelTask := chromedp.NewContext(b.allocCtx)
	runCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
	cleanup := func() {
		_ = chromedp.Cancel(taskCtx)
		cancelTimeout()
		cancelTask()
		b.release()
	}
	return runCtx, cancelTimeout, cleanup, nil
}

// Text navigates to the URL, waits for the network to become idle, and
// returns the full page HTML as bytes. The followRedirects and accept
// parameters are accepted for interface compatibility but Chrome handles
// redirects natively and always sends its own Accept header.
func (b *BrowserClient) Text(ctx context.Context, url string, followRedirects bool, accept string) ([]byte, error) {
	timeout := time.Duration(b.timeoutMS) * time.Millisecond
	taskCtx, cancelTimeout, cleanup, err := b.newTaskContext(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("browser fetch %s: %w", url, err)
	}
	defer cancelTimeout()
	defer cleanup()

	var html string
	err = chromedp.Run(taskCtx,
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
	taskCtx, cancelTimeout, cleanup, err := b.newTaskContext(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("browser capture %s: %w", pageURL, err)
	}
	defer cancelTimeout()
	defer cleanup()

	var (
		mu         sync.Mutex
		seen       = map[network.RequestID]string{}
		bodies     [][]byte
		captureErr error
		wg         sync.WaitGroup
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
			wg.Add(1)
			go func(requestID network.RequestID) {
				defer wg.Done()
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
	wg.Wait()

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
