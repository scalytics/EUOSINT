// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type FileStore struct {
	path string
	mu   sync.Mutex
	doc  Document
}

func NewFileStore(path string, initial Document) (*FileStore, error) {
	fs := &FileStore{path: path, doc: initial}
	if path == "" {
		return fs, nil
	}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (s *FileStore) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.GeneratedAt != "" {
		s.doc = doc
	}
	return nil
}

func (s *FileStore) Snapshot() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneDocument(s.doc)
}

func (s *FileStore) Update(apply func(*Document)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	apply(&s.doc)
	s.doc.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	sortDocument(&s.doc)
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path, data, 0o644)
}

func cloneDocument(doc Document) Document {
	raw, _ := json.Marshal(doc)
	var out Document
	_ = json.Unmarshal(raw, &out)
	return out
}

func sortDocument(doc *Document) {
	sort.Slice(doc.Flows, func(i, j int) bool { return doc.Flows[i].LastSeen > doc.Flows[j].LastSeen })
	sort.Slice(doc.Traces, func(i, j int) bool { return doc.Traces[i].StartedAt > doc.Traces[j].StartedAt })
	sort.Slice(doc.Tasks, func(i, j int) bool { return doc.Tasks[i].LastSeen > doc.Tasks[j].LastSeen })
	sort.Slice(doc.Messages, func(i, j int) bool { return doc.Messages[i].Timestamp > doc.Messages[j].Timestamp })
	sort.Slice(doc.Health.TopicHealth, func(i, j int) bool { return doc.Health.TopicHealth[i].Topic < doc.Health.TopicHealth[j].Topic })
}
