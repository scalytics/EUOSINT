// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestLoadKafkaMapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kafka_mapper.json")
	raw := `{
  "topics": {
    "topic-a": {
      "source": {
        "source_id": "kafka-topic-a",
        "authority_name": "Kafka Topic A",
        "country": "Germany",
        "country_code": "DE",
        "region": "Europe",
        "authority_type": "enterprise_security",
        "base_url": "kafka://topic-a"
      },
      "title_path": "title"
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := loadKafkaMapper(path)
	if err != nil {
		t.Fatalf("load mapper: %v", err)
	}
	if len(doc.Topics) != 1 {
		t.Fatalf("expected one topic mapping, got %d", len(doc.Topics))
	}
}

func TestMapKafkaRecord(t *testing.T) {
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	rec := &kgo.Record{
		Topic:     "security-alerts",
		Partition: 2,
		Offset:    99,
		Value: []byte(`{
      "title":"Critical firewall exploit attempt",
      "severity":"critical",
      "category":"cyber_advisory",
      "link":"https://siem.local/events/99",
      "timestamp":"2026-03-30T09:30:00Z",
      "location":{"lat":52.52,"lng":13.4,"country":"Germany","country_code":"DE"}
    }`),
	}
	mapper := kafkaTopicMapper{
		Source: model.SourceMetadata{
			SourceID:      "kafka-security-alerts",
			AuthorityName: "Kafka Security Alerts",
			Country:       "International",
			CountryCode:   "XX",
			Region:        "International",
			AuthorityType: "enterprise_security",
			BaseURL:       "kafka://security-alerts",
		},
		RegionTag:            "INT",
		TitlePath:            "title",
		SeverityPath:         "severity",
		CategoryPath:         "category",
		CanonicalURLPath:     "link",
		PublishedAtPath:      "timestamp",
		LatPath:              "location.lat",
		LngPath:              "location.lng",
		EventCountryPath:     "location.country",
		EventCountryCodePath: "location.country_code",
	}

	alert, err := mapKafkaRecord(rec, mapper, now)
	if err != nil {
		t.Fatalf("map record: %v", err)
	}
	if alert.AlertID != "kafka:security-alerts:2:99" {
		t.Fatalf("unexpected alert id %q", alert.AlertID)
	}
	if alert.Title == "" || alert.Severity != "critical" || alert.Category != "cyber_advisory" {
		t.Fatalf("unexpected mapped fields: %#v", alert)
	}
	if alert.EventCountryCode != "DE" || alert.Lat == 0 || alert.Lng == 0 {
		t.Fatalf("expected geo/country mapping, got %#v", alert)
	}
}

func TestNormalizeSeverity(t *testing.T) {
	tests := map[string]string{
		"critical":      "critical",
		"SEV1":          "critical",
		"high":          "high",
		"sev3":          "medium",
		"P4":            "low",
		"informational": "info",
		"unknown":       "",
	}
	for in, want := range tests {
		if got := normalizeSeverity(in); got != want {
			t.Fatalf("normalizeSeverity(%q)=%q want=%q", in, got, want)
		}
	}
}
