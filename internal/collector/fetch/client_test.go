// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/scalytics/euosint/internal/collector/config"
)

func TestClientText(t *testing.T) {
	cfg := config.Default()
	client := NewWithHTTPClient(cfg, &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	})
	body, err := client.Text(context.Background(), "https://collector.test", true, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestClientTextSetsBrowserLikeHeaders(t *testing.T) {
	cfg := config.Default()
	client := NewWithHTTPClient(cfg, &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("User-Agent"); !strings.Contains(got, "Mozilla/5.0") {
				t.Fatalf("unexpected user-agent %q", got)
			}
			if got := req.Header.Get("Accept-Language"); got == "" {
				t.Fatal("missing Accept-Language header")
			}
			if got := req.Header.Get("Upgrade-Insecure-Requests"); got != "1" {
				t.Fatalf("unexpected upgrade header %q", got)
			}
			if got := req.Header.Get("Accept-Encoding"); !strings.Contains(got, "gzip") {
				t.Fatalf("missing Accept-Encoding gzip: %q", got)
			}
			if got := req.Header.Get("Sec-Fetch-Dest"); got != "document" {
				t.Fatalf("unexpected Sec-Fetch-Dest %q", got)
			}
			if got := req.Header.Get("Sec-Fetch-Mode"); got != "navigate" {
				t.Fatalf("unexpected Sec-Fetch-Mode %q", got)
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	})

	if _, err := client.Text(context.Background(), "https://collector.test", true, "text/html"); err != nil {
		t.Fatal(err)
	}
}

func TestDecompressBodyGzip(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte("hello gzip"))
	gw.Close()

	res := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Encoding": {"gzip"}},
		Body:       io.NopCloser(&buf),
	}

	if err := decompressBody(res); err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello gzip" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestDecompressBodyIdentity(t *testing.T) {
	res := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("plain")),
	}
	if err := decompressBody(res); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "plain" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestStealthRoundTripperFallsBackToHTTP11AfterHTTP2PeerError(t *testing.T) {
	rt := &stealthRoundTripper{
		dual: &dualProtoTransport{
			protoByHost: map[string]string{"https://collector.test": "h2"},
			roundTripH2: func(req *http.Request) (*http.Response, error) {
				return nil, roundTripError("stream error: stream ID 3; INTERNAL_ERROR; received from peer")
			},
			roundTripH1: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://collector.test/feed", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type roundTripError string

func (e roundTripError) Error() string { return string(e) }
