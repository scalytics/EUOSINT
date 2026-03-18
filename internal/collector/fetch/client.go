// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"golang.org/x/net/idna"
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

// NewFast creates a client with the fast timeout (FetchTimeoutFastMS)
// for RSS/JSON feeds that should respond quickly.
func NewFast(cfg config.Config) *Client {
	timeout := time.Duration(cfg.FetchTimeoutFastMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
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

// FetchResult contains the response body and cache-related headers
// returned by conditional GET requests.
type FetchResult struct {
	Body         []byte
	ETag         string
	LastModified string
	NotModified  bool // true when server returned 304
}

// ErrNotModified is returned when a conditional GET receives 304.
var ErrNotModified = errors.New("not modified")

func (c *Client) Text(ctx context.Context, url string, followRedirects bool, accept string) ([]byte, error) {
	return c.TextWithHeaders(ctx, url, followRedirects, accept, nil)
}

// TextConditional performs a GET with If-None-Match / If-Modified-Since
// headers when etag or lastModified are non-empty. Returns a FetchResult
// with NotModified=true on 304.
func (c *Client) TextConditional(ctx context.Context, url string, followRedirects bool, accept string, etag string, lastModified string) (FetchResult, error) {
	headers := make(map[string]string)
	if strings.TrimSpace(etag) != "" {
		headers["If-None-Match"] = etag
	}
	if strings.TrimSpace(lastModified) != "" {
		headers["If-Modified-Since"] = lastModified
	}
	return c.doFetch(ctx, url, followRedirects, accept, headers)
}

func (c *Client) TextWithHeaders(ctx context.Context, url string, followRedirects bool, accept string, extraHeaders map[string]string) ([]byte, error) {
	result, err := c.doFetch(ctx, url, followRedirects, accept, extraHeaders)
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

func (c *Client) doFetch(ctx context.Context, rawURL string, followRedirects bool, accept string, extraHeaders map[string]string) (FetchResult, error) {
	url := punycodeURL(rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("build request %s: %w", url, err)
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
		client = noRedirectClient(c.httpClient)
	}

	res, err := client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		return FetchResult{
			ETag:         res.Header.Get("ETag"),
			LastModified: res.Header.Get("Last-Modified"),
			NotModified:  true,
		}, nil
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf("fetch %s: status %d", url, res.StatusCode)
	}

	body, err := readBody(res, c.maxBodyBytes)
	if err != nil {
		return FetchResult{}, fmt.Errorf("read %s: %w", url, err)
	}
	if int64(len(body)) > c.maxBodyBytes {
		return FetchResult{}, fmt.Errorf("response too large for %s", url)
	}

	return FetchResult{
		Body:         body,
		ETag:         res.Header.Get("ETag"),
		LastModified: res.Header.Get("Last-Modified"),
	}, nil
}

// HeadStatus performs a lightweight HEAD probe and returns the resulting HTTP
// status code (or an error when the probe failed).
func (c *Client) HeadStatus(ctx context.Context, rawURL string, followRedirects bool) (int, error) {
	url := punycodeURL(rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	client := c.httpClient
	if !followRedirects {
		client = noRedirectClient(c.httpClient)
	}

	res, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("probe %s: %w", url, err)
	}
	defer res.Body.Close()
	return res.StatusCode, nil
}

// readBody reads the response body, handling gzip/br/deflate transparently.
// The stealth transport configures decompression, but if a test transport is
// injected the body may already be plain text.
func readBody(res *http.Response, limit int64) ([]byte, error) {
	reader := io.LimitReader(res.Body, limit+1)
	return io.ReadAll(reader)
}

func noRedirectClient(base *http.Client) *http.Client {
	copyClient := *base
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copyClient
}

// punycodeURL converts IDN (internationalized) hostnames to ASCII punycode
// so Go's HTTP client can resolve and TLS-connect to them correctly.
// E.g. "https://мвд.рф/news" → "https://xn--b1aew.xn--p1ai/news"
func punycodeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := u.Hostname()
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == host {
		return rawURL
	}
	// Preserve port if present.
	if port := u.Port(); port != "" {
		u.Host = ascii + ":" + port
	} else {
		u.Host = ascii
	}
	return u.String()
}
