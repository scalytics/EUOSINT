// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
)

type Client struct {
	httpClient   *http.Client
	userAgent    string
	maxBodyBytes int64
}

func New(cfg config.Config) *Client {
	timeout := time.Duration(cfg.HTTPTimeoutMS) * time.Millisecond

	return NewWithHTTPClient(cfg, &http.Client{
		Timeout: timeout,
		Transport: newStealthTransport(&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	})
}

func NewWithHTTPClient(cfg config.Config, httpClient *http.Client) *Client {
	return &Client{
		httpClient:   httpClient,
		userAgent:    cfg.UserAgent,
		maxBodyBytes: cfg.MaxResponseBodyBytes,
	}
}

func (c *Client) Text(ctx context.Context, url string, followRedirects bool, accept string) ([]byte, error) {
	return c.TextWithHeaders(ctx, url, followRedirects, accept, nil)
}

func (c *Client) TextWithHeaders(ctx context.Context, url string, followRedirects bool, accept string, extraHeaders map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if strings.TrimSpace(accept) != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	for key, value := range extraHeaders {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	client := c.httpClient
	if !followRedirects {
		copyClient := *c.httpClient
		copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &copyClient
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: status %d", url, res.StatusCode)
	}

	body, err := readBody(res, c.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if int64(len(body)) > c.maxBodyBytes {
		return nil, fmt.Errorf("response too large for %s", url)
	}

	return body, nil
}

// readBody reads the response body, handling gzip/br/deflate transparently.
// The stealth transport configures decompression, but if a test transport is
// injected the body may already be plain text.
func readBody(res *http.Response, limit int64) ([]byte, error) {
	reader := io.LimitReader(res.Body, limit+1)
	return io.ReadAll(reader)
}
