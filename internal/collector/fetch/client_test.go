package fetch

import (
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
