// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// newStealthTransport builds an http.RoundTripper whose TLS ClientHello
// impersonates a recent Chrome release. This prevents WAFs that
// fingerprint Go's default TLS stack (JA3/JA4) from blocking requests.
//
// It supports both HTTP/1.1 and HTTP/2: after the uTLS handshake the
// negotiated ALPN protocol is cached per-host and subsequent requests
// are routed to the correct transport automatically.
//
// It also transparently decompresses gzip/br/deflate responses, since we
// explicitly send Accept-Encoding to look like a real browser (which
// disables Go's automatic gzip handling).
func newStealthTransport(dialer *net.Dialer) http.RoundTripper {
	dt := &dualProtoTransport{
		dialer:    dialer,
		protoByHost: make(map[string]string),
	}

	dt.h1 = &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		ForceAttemptHTTP2: false,
		MaxIdleConns:      100,
		IdleConnTimeout:   90 * time.Second,
		DialTLSContext:    dt.dialTLS,
		DialContext:       dialer.DialContext,
	}

	dt.h2 = &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dt.dialTLS(ctx, network, addr)
		},
	}

	return &stealthRoundTripper{dual: dt}
}

// dualProtoTransport manages uTLS connections and routes them to the
// appropriate HTTP/1.1 or HTTP/2 transport based on ALPN negotiation.
type dualProtoTransport struct {
	dialer *net.Dialer
	h1     *http.Transport
	h2     *http2.Transport

	mu          sync.Mutex
	protoByHost map[string]string // hostname -> "h2" | "h1"
}

func (dt *dualProtoTransport) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	rawConn, err := dt.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	tlsConn := utls.UClient(rawConn, &utls.Config{
		ServerName: host,
	}, utls.HelloChrome_Auto)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}

	// Cache the negotiated protocol so future requests skip the probe.
	proto := tlsConn.ConnectionState().NegotiatedProtocol
	dt.mu.Lock()
	if proto == "h2" {
		dt.protoByHost[host] = "h2"
	} else {
		dt.protoByHost[host] = "h1"
	}
	dt.mu.Unlock()

	return tlsConn, nil
}

func (dt *dualProtoTransport) getProto(host string) string {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.protoByHost[host]
}

// stealthRoundTripper routes requests to the appropriate protocol
// transport and transparently decompresses response bodies.
type stealthRoundTripper struct {
	dual *dualProtoTransport
}

func (s *stealthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	proto := s.dual.getProto(host)

	var res *http.Response
	var err error

	switch proto {
	case "h2":
		res, err = s.dual.h2.RoundTrip(req)
	case "h1":
		res, err = s.dual.h1.RoundTrip(req)
	default:
		// Unknown host — try h1 first. If the server negotiates h2 via
		// ALPN, the h1 transport will fail with "malformed HTTP response"
		// because it tries HTTP/1.1 framing on an h2 connection. The
		// dialTLS callback caches the negotiated proto regardless, so we
		// can detect this and retry with the h2 transport.
		res, err = s.dual.h1.RoundTrip(req)
		if err != nil && s.dual.getProto(host) == "h2" {
			res, err = s.dual.h2.RoundTrip(req)
		}
	}

	if err != nil {
		return nil, err
	}
	if derr := decompressBody(res); derr != nil {
		// If decompression setup fails, still return the raw body.
		res.Body = io.NopCloser(bufio.NewReader(res.Body))
	}
	return res, nil
}

// decompressBody wraps the response body reader to handle Content-Encoding.
func decompressBody(res *http.Response) error {
	ce := strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Encoding")))
	if ce == "" || ce == "identity" {
		return nil
	}

	var reader io.ReadCloser
	switch ce {
	case "gzip":
		gr, err := gzip.NewReader(res.Body)
		if err != nil {
			return err
		}
		reader = gr
	case "br":
		reader = io.NopCloser(brotli.NewReader(res.Body))
	case "deflate":
		reader = flate.NewReader(res.Body)
	default:
		return nil
	}

	original := res.Body
	res.Body = &wrappedBody{reader: reader, closer: original}
	res.Header.Del("Content-Encoding")
	res.Header.Del("Content-Length")
	return nil
}

// wrappedBody reads from the decompressor but closes the underlying conn.
type wrappedBody struct {
	reader io.ReadCloser
	closer io.Closer
}

func (w *wrappedBody) Read(p []byte) (int, error) { return w.reader.Read(p) }
func (w *wrappedBody) Close() error {
	w.reader.Close()
	return w.closer.Close()
}
