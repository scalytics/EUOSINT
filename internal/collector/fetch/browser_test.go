// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewBrowser_UsesDefaultsAndRemoteAllocator(t *testing.T) {
	oldRemote := initRemoteAllocatorFn
	t.Cleanup(func() { initRemoteAllocatorFn = oldRemote })

	var (
		gotWSURL     string
		gotTimeout   int
		gotRetries   int
		gotRetryWait time.Duration
	)
	initRemoteAllocatorFn = func(wsURL string, timeoutMS int, connectRetries int, retryDelay time.Duration) (context.Context, context.CancelFunc, error) {
		gotWSURL = wsURL
		gotTimeout = timeoutMS
		gotRetries = connectRetries
		gotRetryWait = retryDelay
		return context.Background(), func() {}, nil
	}

	b, err := NewBrowser(BrowserOptions{})
	if err != nil {
		t.Fatalf("NewBrowser() error = %v", err)
	}
	defer b.Close()

	if gotWSURL != defaultBrowserWSURL {
		t.Fatalf("wsURL = %q, want %q", gotWSURL, defaultBrowserWSURL)
	}
	if gotTimeout != 30000 {
		t.Fatalf("timeoutMS = %d, want 30000", gotTimeout)
	}
	if gotRetries != 3 {
		t.Fatalf("connectRetries = %d, want 3", gotRetries)
	}
	if gotRetryWait != time.Second {
		t.Fatalf("retryDelay = %v, want %v", gotRetryWait, time.Second)
	}
	if cap(b.sem) != 1 {
		t.Fatalf("default semaphore cap = %d, want 1", cap(b.sem))
	}
}

func TestNewBrowser_ReturnsErrorOnRemoteFailure(t *testing.T) {
	oldRemote := initRemoteAllocatorFn
	t.Cleanup(func() { initRemoteAllocatorFn = oldRemote })

	initRemoteAllocatorFn = func(wsURL string, timeoutMS int, connectRetries int, retryDelay time.Duration) (context.Context, context.CancelFunc, error) {
		return nil, nil, errors.New("remote down")
	}

	b, err := NewBrowser(BrowserOptions{
		WSURL:          "ws://browser:3000",
		MaxConcurrency: 4,
	})
	if err == nil {
		b.Close()
		t.Fatal("expected error when remote browser is unreachable")
	}
	if b != nil {
		t.Fatal("expected nil client on error")
	}
}

func TestAcquire_RespectsCanceledContext(t *testing.T) {
	b := &BrowserClient{sem: make(chan struct{}, 1)}
	if err := b.acquire(context.Background()); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.acquire(ctx); err == nil {
		t.Fatal("expected canceled context error on second acquire")
	}
}
