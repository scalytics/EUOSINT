// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
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

func TestLoadKafkaMapperValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kafka_mapper.json")
	raw := `{"topics":{"":{"source":{"source_id":"s","authority_name":"a"},"title_path":"title"}}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKafkaMapper(path); err == nil || !strings.Contains(err.Error(), "topic key must not be empty") {
		t.Fatalf("expected empty topic validation error, got %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"topics":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKafkaMapper(path); err == nil || !strings.Contains(err.Error(), "topics mapping is empty") {
		t.Fatalf("expected empty topics error, got %v", err)
	}
}

func TestLoadKafkaMapperRequiresSourceAndTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kafka_mapper.json")
	raw := `{"topics":{"topic-a":{"source":{"authority_name":"a"},"title_path":""}}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKafkaMapper(path); err == nil || !strings.Contains(err.Error(), "source.source_id is required") {
		t.Fatalf("expected source id validation error, got %v", err)
	}

	raw = `{"topics":{"topic-a":{"source":{"source_id":"s"},"title_path":"title"}}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKafkaMapper(path); err == nil || !strings.Contains(err.Error(), "authority_name is required") {
		t.Fatalf("expected authority name validation error, got %v", err)
	}

	raw = `{"topics":{"topic-a":{"source":{"source_id":"s","authority_name":"a"}}}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKafkaMapper(path); err == nil || !strings.Contains(err.Error(), "title_path is required") {
		t.Fatalf("expected title path validation error, got %v", err)
	}
}

func TestMapKafkaRecordDefaultsAndFallbacks(t *testing.T) {
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	rec := &kgo.Record{
		Topic:     "alerts",
		Partition: 1,
		Offset:    5,
		Value:     []byte(`{"severity":"unknown","timestamp":"1711791000"}`),
	}
	mapper := kafkaTopicMapper{
		Source: model.SourceMetadata{
			SourceID:      "kafka-alerts",
			AuthorityName: "Kafka Alerts",
		},
		SeverityPath:    "severity",
		SeverityDefault: "informational",
		PublishedAtPath: "timestamp",
	}
	alert, err := mapKafkaRecord(rec, mapper, now)
	if err != nil {
		t.Fatal(err)
	}
	if alert.Title != "Kafka event alerts/1/5" {
		t.Fatalf("unexpected fallback title %q", alert.Title)
	}
	if alert.Category != "informational" || alert.Severity != "info" {
		t.Fatalf("unexpected defaults %#v", alert)
	}
	if alert.Source.Country != "Unknown" || alert.Source.CountryCode != "XX" || alert.Source.BaseURL != "kafka://alerts" {
		t.Fatalf("unexpected source defaults %#v", alert.Source)
	}
	if alert.RegionTag != "International" {
		t.Fatalf("unexpected region tag %q", alert.RegionTag)
	}
	if alert.CanonicalURL != "kafka://alerts/1/5" {
		t.Fatalf("unexpected canonical url %q", alert.CanonicalURL)
	}
	if alert.FreshnessHours <= 0 {
		t.Fatalf("expected positive freshness, got %d", alert.FreshnessHours)
	}
}

func TestMapKafkaRecordInvalidJSON(t *testing.T) {
	_, err := mapKafkaRecord(&kgo.Record{Topic: "alerts", Offset: 4, Value: []byte(`{`)}, kafkaTopicMapper{}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "invalid json") {
		t.Fatalf("expected invalid json error, got %v", err)
	}
}

func TestReadHelpersAndTimeParsing(t *testing.T) {
	payload := map[string]any{
		"flat":          "value",
		"bool":          true,
		"int":           int(9),
		"float32":       float32(3.5),
		"int64":         int64(7),
		"string_number": "21.25",
		"nested": map[string]any{
			"number": json.Number("12.5"),
			"id":     42,
		},
	}
	if got := readString(payload, "flat"); got != "value" {
		t.Fatalf("readString flat=%q", got)
	}
	if got := readString(payload, "nested.id"); got != "42" {
		t.Fatalf("readString nested.id=%q", got)
	}
	if got := readFloat(payload, "nested.number"); got != 12.5 {
		t.Fatalf("readFloat nested.number=%v", got)
	}
	if got := readFloat(payload, "float32"); got != 3.5 {
		t.Fatalf("readFloat float32=%v", got)
	}
	if got := readFloat(payload, "int64"); got != 7 {
		t.Fatalf("readFloat int64=%v", got)
	}
	if got := readFloat(payload, "string_number"); got != 21.25 {
		t.Fatalf("readFloat string_number=%v", got)
	}
	if got := readFloat(payload, "int"); got != 9 {
		t.Fatalf("readFloat int=%v", got)
	}
	if got := readFloat(payload, "flat"); got != 0 {
		t.Fatalf("expected unsupported float conversion to return 0, got %v", got)
	}
	if got := readFloat(payload, "bool"); got != 0 {
		t.Fatalf("expected bool conversion to return 0, got %v", got)
	}
	if got := readString(payload, "nested.number"); got != "12.5" {
		t.Fatalf("readString nested.number=%q", got)
	}
	if got := readString(payload, "bool"); got != "true" {
		t.Fatalf("readString bool=%q", got)
	}
	if _, ok := readPath(payload, "nested.missing"); ok {
		t.Fatal("expected missing path lookup to fail")
	}
	if _, ok := readPath(payload, "nested..missing"); ok {
		t.Fatal("expected malformed path lookup to fail")
	}
	if _, ok := readPath(payload, ""); ok {
		t.Fatal("expected empty path lookup to fail")
	}
	if _, ok := readPath(payload, "flat.child"); ok {
		t.Fatal("expected non-map intermediate lookup to fail")
	}

	fallback := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	if got := parseKafkaTime("", fallback); !got.Equal(fallback) {
		t.Fatalf("expected fallback time, got %v", got)
	}
	if got := parseKafkaTime("2026-03-30T09:30:00Z", fallback); got.Format(time.RFC3339) != "2026-03-30T09:30:00Z" {
		t.Fatalf("unexpected RFC3339 parse %v", got)
	}
	if got := parseKafkaTime("1711791000000", fallback); got.IsZero() {
		t.Fatal("expected unix millis parse")
	}
	if got := parseKafkaTime("1711791000", fallback); got.IsZero() {
		t.Fatal("expected unix seconds parse")
	}
	if got := parseKafkaTime("not-a-time", fallback); !got.Equal(fallback) {
		t.Fatalf("expected invalid time to fall back, got %v", got)
	}
}

func TestKafkaSASLMechanismValidation(t *testing.T) {
	_, err := kafkaSASLMechanism(config.Config{KafkaUsername: "user"})
	if err == nil || !strings.Contains(err.Error(), "KAFKA_USERNAME and KAFKA_PASSWORD") {
		t.Fatalf("expected missing creds error, got %v", err)
	}
	for _, mech := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		if _, err := kafkaSASLMechanism(config.Config{KafkaUsername: "user", KafkaPassword: "pass", KafkaSASLMechanism: mech}); err != nil {
			t.Fatalf("expected mechanism %s to work: %v", mech, err)
		}
	}
	if _, err := kafkaSASLMechanism(config.Config{KafkaUsername: "user", KafkaPassword: "pass", KafkaSASLMechanism: "BAD"}); err == nil {
		t.Fatal("expected unsupported mechanism error")
	}
}

func TestNewKafkaClientValidation(t *testing.T) {
	if _, err := newKafkaClient(config.Config{}); err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
		t.Fatalf("expected brokers validation error, got %v", err)
	}
	if _, err := newKafkaClient(config.Config{KafkaBrokers: []string{"localhost:9092"}}); err == nil || !strings.Contains(err.Error(), "KAFKA_TOPICS") {
		t.Fatalf("expected topics validation error, got %v", err)
	}
	if _, err := newKafkaClient(config.Config{KafkaBrokers: []string{"localhost:9092"}, KafkaTopics: []string{"alerts"}}); err == nil || !strings.Contains(err.Error(), "KAFKA_GROUP_ID") {
		t.Fatalf("expected group validation error, got %v", err)
	}
	if _, err := newKafkaClient(config.Config{
		KafkaBrokers:          []string{"localhost:9092"},
		KafkaTopics:           []string{"alerts"},
		KafkaGroupID:          "group-a",
		KafkaSecurityProtocol: "UNSUPPORTED",
	}); err == nil {
		t.Fatal("expected security protocol validation error")
	}
	client, err := newKafkaClient(config.Config{
		KafkaBrokers:  []string{"localhost:9092"},
		KafkaTopics:   []string{"alerts"},
		KafkaGroupID:  "group-a",
		KafkaClientID: "collector",
	})
	if err != nil {
		t.Fatalf("expected plaintext client to build: %v", err)
	}
	client.Close()

	for _, cfg := range []config.Config{
		{
			KafkaBrokers:               []string{"localhost:9092"},
			KafkaTopics:                []string{"alerts"},
			KafkaGroupID:               "group-a",
			KafkaSecurityProtocol:      "SSL",
			KafkaTLSInsecureSkipVerify: true,
		},
		{
			KafkaBrokers:               []string{"localhost:9092"},
			KafkaTopics:                []string{"alerts"},
			KafkaGroupID:               "group-a",
			KafkaSecurityProtocol:      "SASL_SSL",
			KafkaUsername:              "user",
			KafkaPassword:              "pass",
			KafkaSASLMechanism:         "PLAIN",
			KafkaTLSInsecureSkipVerify: true,
		},
		{
			KafkaBrokers:          []string{"localhost:9092"},
			KafkaTopics:           []string{"alerts"},
			KafkaGroupID:          "group-a",
			KafkaSecurityProtocol: "SASL_PLAINTEXT",
			KafkaUsername:         "user",
			KafkaPassword:         "pass",
			KafkaSASLMechanism:    "SCRAM-SHA-256",
		},
	} {
		client, err := newKafkaClient(cfg)
		if err != nil {
			t.Fatalf("expected client to build for %#v: %v", cfg.KafkaSecurityProtocol, err)
		}
		client.Close()
	}
}

func TestFirstFatalKafkaError(t *testing.T) {
	if err := firstFatalKafkaError(nil); err != nil {
		t.Fatalf("expected nil fetches to return nil, got %v", err)
	}
	if err := firstFatalKafkaError(fetchWithErr("alerts", context.Canceled)); err != nil {
		t.Fatalf("expected context canceled to be ignored, got %v", err)
	}
	if err := firstFatalKafkaError(fetchWithErr("alerts", context.DeadlineExceeded)); err != nil {
		t.Fatalf("expected context deadline to be ignored, got %v", err)
	}
	if err := firstFatalKafkaError(fetchWithErr("alerts", kgo.ErrClientClosed)); !errors.Is(err, kgo.ErrClientClosed) {
		t.Fatalf("expected client closed error, got %v", err)
	}
	want := errors.New("boom")
	if err := firstFatalKafkaError(fetchWithErr("alerts", want)); !errors.Is(err, want) {
		t.Fatalf("expected fatal error, got %v", err)
	}
}

func TestMapKafkaRecordFutureTimestampClampsFreshness(t *testing.T) {
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	rec := &kgo.Record{
		Topic:     "alerts",
		Partition: 0,
		Offset:    3,
		Value:     []byte(`{"title":"future","timestamp":"3026-03-30T10:00:00Z"}`),
	}
	mapper := kafkaTopicMapper{
		Source:          model.SourceMetadata{SourceID: "alerts", AuthorityName: "Alerts"},
		TitlePath:       "title",
		PublishedAtPath: "timestamp",
	}
	alert, err := mapKafkaRecord(rec, mapper, now)
	if err != nil {
		t.Fatal(err)
	}
	if alert.FreshnessHours != 0 {
		t.Fatalf("expected clamped freshness, got %d", alert.FreshnessHours)
	}
}

func TestCollectKafkaAlertsExecutionPaths(t *testing.T) {
	dir := t.TempDir()
	mapperPath := filepath.Join(dir, "mapper.json")
	mapperJSON := `{
	  "topics": {
	    "alerts": {
	      "source": {"source_id":"kafka-alerts","authority_name":"Kafka Alerts"},
	      "title_path":"title",
	      "severity_default":"high",
	      "category_default":"cyber_advisory"
	    }
	  }
	}`
	if err := os.WriteFile(mapperPath, []byte(mapperJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	valid := &kgo.Record{Topic: "alerts", Partition: 0, Offset: 1, Value: []byte(`{"title":"A"}`)}
	large := &kgo.Record{Topic: "alerts", Partition: 0, Offset: 2, Value: []byte(strings.Repeat("x", 2048))}
	missingMapper := &kgo.Record{Topic: "other", Partition: 0, Offset: 3, Value: []byte(`{"title":"B"}`)}
	invalid := &kgo.Record{Topic: "alerts", Partition: 0, Offset: 4, Value: []byte(`{`)}

	runner := New(nil, nil)
	mock := &mockKafkaClient{
		polls: []kgo.Fetches{
			nil,
			fetchWithRecords("alerts", valid, large),
			fetchWithRecords("alerts", missingMapper, invalid),
		},
	}
	runner.kafkaClientFactory = func(cfg config.Config) (kafkaClient, error) {
		return mock, nil
	}
	cfg := config.Config{
		KafkaEnabled:        true,
		KafkaMapperPath:     mapperPath,
		KafkaTopics:         []string{"alerts", "other"},
		KafkaMaxPerCycle:    10,
		KafkaMaxRecordBytes: 512,
		KafkaPollTimeoutMS:  10,
		KafkaTestOnStart:    true,
	}
	alerts, health := runner.collectKafkaAlerts(context.Background(), cfg, now)
	if len(alerts) != 1 || health == nil {
		t.Fatalf("expected one alert and health entry, got alerts=%d health=%#v", len(alerts), health)
	}
	if health.Status != "ok" || health.FetchedCount != 1 {
		t.Fatalf("unexpected health %#v", health)
	}
	if health.ErrorClass != "dropped" || !strings.Contains(health.Error, "dropped=1") {
		t.Fatalf("expected dropped summary, got %#v", health)
	}
	if len(mock.committed) != 1 || mock.committed[0].Offset != 1 {
		t.Fatalf("expected only valid record committed, got %#v", mock.committed)
	}
	if !mock.closed {
		t.Fatal("expected kafka client to close")
	}
}

func TestCollectKafkaAlertsErrorPaths(t *testing.T) {
	runner := New(nil, nil)
	if alerts, health := runner.collectKafkaAlerts(context.Background(), config.Config{}, time.Now().UTC()); alerts != nil || health != nil {
		t.Fatalf("expected disabled kafka collector to no-op, got alerts=%v health=%#v", alerts, health)
	}

	cfg := config.Config{KafkaEnabled: true, KafkaMapperPath: filepath.Join(t.TempDir(), "missing.json")}
	if alerts, health := runner.collectKafkaAlerts(context.Background(), cfg, time.Now().UTC()); len(alerts) != 0 || health == nil || health.ErrorClass != "mapper" {
		t.Fatalf("expected mapper error health, got alerts=%v health=%#v", alerts, health)
	}

	dir := t.TempDir()
	mapperPath := filepath.Join(dir, "mapper.json")
	if err := os.WriteFile(mapperPath, []byte(`{"topics":{"alerts":{"source":{"source_id":"s","authority_name":"a"},"title_path":"title"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.KafkaMapperPath = mapperPath
	runner.kafkaClientFactory = func(config.Config) (kafkaClient, error) {
		return nil, errors.New("bad config")
	}
	if _, health := runner.collectKafkaAlerts(context.Background(), cfg, time.Now().UTC()); health == nil || health.ErrorClass != "config" {
		t.Fatalf("expected config error health, got %#v", health)
	}

	runner.kafkaClientFactory = func(config.Config) (kafkaClient, error) {
		return &mockKafkaClient{polls: []kgo.Fetches{fetchWithErr("alerts", errors.New("startup fail"))}}, nil
	}
	cfg.KafkaTestOnStart = true
	if _, health := runner.collectKafkaAlerts(context.Background(), cfg, time.Now().UTC()); health == nil || health.ErrorClass != "startup" {
		t.Fatalf("expected startup error health, got %#v", health)
	}

	runner.kafkaClientFactory = func(config.Config) (kafkaClient, error) {
		return &mockKafkaClient{polls: []kgo.Fetches{fetchWithErr("alerts", context.Canceled), fetchWithErr("alerts", errors.New("poll fail"))}}, nil
	}
	cfg.KafkaTestOnStart = true
	if _, health := runner.collectKafkaAlerts(context.Background(), cfg, time.Now().UTC()); health == nil || health.ErrorClass != "poll" {
		t.Fatalf("expected poll error health, got %#v", health)
	}

	record := &kgo.Record{Topic: "alerts", Partition: 0, Offset: 7, Value: []byte(`{"title":"A"}`)}
	runner.kafkaClientFactory = func(config.Config) (kafkaClient, error) {
		return &mockKafkaClient{polls: []kgo.Fetches{fetchWithRecords("alerts", record)}, commitErr: errors.New("commit fail")}, nil
	}
	cfg.KafkaTestOnStart = false
	alerts, health := runner.collectKafkaAlerts(context.Background(), cfg, time.Now().UTC())
	if len(alerts) != 1 || health == nil || health.ErrorClass != "commit" {
		t.Fatalf("expected commit error with mapped alerts, got alerts=%d health=%#v", len(alerts), health)
	}
}

type mockKafkaClient struct {
	polls     []kgo.Fetches
	pollIndex int
	committed []*kgo.Record
	commitErr error
	closed    bool
}

func (m *mockKafkaClient) PollFetches(context.Context) kgo.Fetches {
	if m.pollIndex >= len(m.polls) {
		return nil
	}
	out := m.polls[m.pollIndex]
	m.pollIndex++
	return out
}

func (m *mockKafkaClient) CommitRecords(_ context.Context, recs ...*kgo.Record) error {
	m.committed = append(m.committed, recs...)
	return m.commitErr
}

func (m *mockKafkaClient) Close() {
	m.closed = true
}

func fetchWithRecords(topic string, records ...*kgo.Record) kgo.Fetches {
	return kgo.Fetches{
		{
			Topics: []kgo.FetchTopic{
				{
					Topic: topic,
					Partitions: []kgo.FetchPartition{
						{Partition: 0, Records: records},
					},
				},
			},
		},
	}
}

func fetchWithErr(topic string, err error) kgo.Fetches {
	return kgo.Fetches{
		{
			Topics: []kgo.FetchTopic{
				{
					Topic: topic,
					Partitions: []kgo.FetchPartition{
						{Partition: 0, Err: err},
					},
				},
			},
		},
	}
}
