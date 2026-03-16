// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	return NewWithHTTPClient(cfg, &http.Client{
		Timeout: time.Duration(cfg.HTTPTimeoutMS) * time.Millisecond,
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if strings.TrimSpace(accept) != "" {
		req.Header.Set("Accept", accept)
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

	reader := io.LimitReader(res.Body, c.maxBodyBytes+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if int64(len(body)) > c.maxBodyBytes {
		return nil, fmt.Errorf("response too large for %s", url)
	}

	return body, nil
}
