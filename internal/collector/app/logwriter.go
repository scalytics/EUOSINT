// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"io"
	"sync"
	"time"
)

type timestampWriter struct {
	mu  sync.Mutex
	dst io.Writer
	buf bytes.Buffer
}

func newTimestampWriter(dst io.Writer) io.Writer {
	if dst == nil {
		return nil
	}
	return &timestampWriter{dst: dst}
}

func (w *timestampWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := len(p)
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		if idx < 0 {
			_, _ = w.buf.Write(p)
			break
		}
		_, _ = w.buf.Write(p[:idx])
		line := w.buf.String()
		w.buf.Reset()
		if _, err := io.WriteString(w.dst, time.Now().UTC().Format(time.RFC3339)+" "+line+"\n"); err != nil {
			return 0, err
		}
		p = p[idx+1:]
	}
	return total, nil
}
