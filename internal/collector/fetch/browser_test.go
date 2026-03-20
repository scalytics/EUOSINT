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
	oldLocal := localAllocatorFn
	t.Cleanup(func() {
		initRemoteAllocatorFn = oldRemote
		localAllocatorFn = oldLocal
	})

	var (
		gotWSURL     string
		gotTimeout   int
		gotRetries   int
		gotRetryWait time.Duration
		localCalled  bool
	)
	initRemoteAllocatorFn = func(wsURL string, timeoutMS int, connectRetries int, retryDelay time.Duration) (context.Context, context.CancelFunc, error) {
		gotWSURL = wsURL
		gotTimeout = timeoutMS
		gotRetries = connectRetries
		gotRetryWait = retryDelay
		return context.Background(), func() {}, nil
	}
	localAllocatorFn = func() (context.Context, context.CancelFunc) {
		localCalled = true
		return context.Background(), func() {}
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
	if localCalled {
		t.Fatal("local allocator should not be called on remote success")
	}
	if b.Warning() != "" {
		t.Fatalf("warning = %q, want empty", b.Warning())
	}
	if cap(b.sem) != 1 {
		t.Fatalf("default semaphore cap = %d, want 1", cap(b.sem))
	}
}

func TestNewBrowser_FallsBackToLocalOnRemoteFailure(t *testing.T) {
	oldRemote := initRemoteAllocatorFn
	oldLocal := localAllocatorFn
	t.Cleanup(func() {
		initRemoteAllocatorFn = oldRemote
		localAllocatorFn = oldLocal
	})

	initRemoteAllocatorFn = func(wsURL string, timeoutMS int, connectRetries int, retryDelay time.Duration) (context.Context, context.CancelFunc, error) {
		return nil, nil, errors.New("remote down")
	}
	localCalled := false
	localAllocatorFn = func() (context.Context, context.CancelFunc) {
		localCalled = true
		return context.Background(), func() {}
	}

	b, err := NewBrowser(BrowserOptions{
		WSURL:          "ws://browser:3000",
		MaxConcurrency: 4,
	})
	if err != nil {
		t.Fatalf("NewBrowser() error = %v", err)
	}
	defer b.Close()

	if !localCalled {
		t.Fatal("expected local allocator fallback to be called")
	}
	if cap(b.sem) != 4 {
		t.Fatalf("semaphore cap = %d, want 4", cap(b.sem))
	}
	if b.Warning() == "" {
		t.Fatal("expected fallback warning, got empty")
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

func TestWarning_TrimsWhitespace(t *testing.T) {
	b := &BrowserClient{warning: "  fallback warning  "}
	if got := b.Warning(); got != "fallback warning" {
		t.Fatalf("Warning() = %q, want %q", got, "fallback warning")
	}
}
