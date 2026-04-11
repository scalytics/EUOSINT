// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

const kafkaConnectorSourceID = "kafka-consumer"

type kafkaClient interface {
	PollFetches(context.Context) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	Close()
}

type kafkaMapperDocument struct {
	Topics map[string]kafkaTopicMapper `json:"topics"`
}

type kafkaTopicMapper struct {
	Source               model.SourceMetadata `json:"source"`
	RegionTag            string               `json:"region_tag,omitempty"`
	TitlePath            string               `json:"title_path"`
	SeverityPath         string               `json:"severity_path,omitempty"`
	SeverityDefault      string               `json:"severity_default,omitempty"`
	CategoryPath         string               `json:"category_path,omitempty"`
	CategoryDefault      string               `json:"category_default,omitempty"`
	CanonicalURLPath     string               `json:"canonical_url_path,omitempty"`
	PublishedAtPath      string               `json:"published_at_path,omitempty"`
	LatPath              string               `json:"lat_path,omitempty"`
	LngPath              string               `json:"lng_path,omitempty"`
	EventCountryPath     string               `json:"event_country_path,omitempty"`
	EventCountryCodePath string               `json:"event_country_code_path,omitempty"`
}

func (r Runner) collectKafkaAlerts(ctx context.Context, cfg config.Config, now time.Time) ([]model.Alert, *model.SourceHealthEntry) {
	if !cfg.KafkaEnabled {
		return nil, nil
	}
	startedAt := time.Now().UTC()
	entry := &model.SourceHealthEntry{
		SourceID:      kafkaConnectorSourceID,
		AuthorityName: "Kafka Consumer",
		Type:          "kafka",
		FeedURL:       strings.Join(cfg.KafkaTopics, ","),
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		Status:        "error",
	}

	mappers, err := loadKafkaMapper(cfg.KafkaMapperPath)
	if err != nil {
		entry.Error = fmt.Sprintf("kafka mapper: %v", err)
		entry.ErrorClass = "mapper"
		return nil, entry
	}

	clientFactory := r.kafkaClientFactory
	if clientFactory == nil {
		clientFactory = func(cfg config.Config) (kafkaClient, error) {
			return newKafkaClient(cfg)
		}
	}
	client, err := clientFactory(cfg)
	if err != nil {
		entry.Error = fmt.Sprintf("kafka client: %v", err)
		entry.ErrorClass = "config"
		return nil, entry
	}
	defer client.Close()

	maxPerCycle := cfg.KafkaMaxPerCycle
	if maxPerCycle <= 0 {
		maxPerCycle = 500
	}
	maxRecordBytes := cfg.KafkaMaxRecordBytes
	if maxRecordBytes <= 0 {
		maxRecordBytes = 1 << 20
	}
	pollTimeout := cfg.KafkaPollTimeoutMS
	if pollTimeout <= 0 {
		pollTimeout = 2000
	}
	testTimeout := pollTimeout
	if testTimeout > 1000 {
		testTimeout = 1000
	}

	if cfg.KafkaTestOnStart {
		testCtx, cancel := context.WithTimeout(ctx, time.Duration(testTimeout)*time.Millisecond)
		testFetches := client.PollFetches(testCtx)
		cancel()
		if err := firstFatalKafkaError(testFetches); err != nil {
			entry.Error = fmt.Sprintf("kafka startup test: %v", err)
			entry.ErrorClass = "startup"
			entry.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			return nil, entry
		}
	}

	pollCtx, cancel := context.WithTimeout(ctx, time.Duration(pollTimeout)*time.Millisecond)
	fetches := client.PollFetches(pollCtx)
	cancel()
	if err := firstFatalKafkaError(fetches); err != nil {
		entry.Error = fmt.Sprintf("kafka poll: %v", err)
		entry.ErrorClass = "poll"
		entry.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		return nil, entry
	}

	alerts := make([]model.Alert, 0, maxPerCycle)
	toCommit := make([]*kgo.Record, 0, maxPerCycle)
	dropped := 0
	lastDropErr := ""

	fetches.EachRecord(func(rec *kgo.Record) {
		if len(alerts) >= maxPerCycle {
			return
		}
		if len(rec.Value) > maxRecordBytes {
			dropped++
			lastDropErr = fmt.Sprintf("record too large topic=%s partition=%d offset=%d", rec.Topic, rec.Partition, rec.Offset)
			return
		}
		mapper, ok := mappers.Topics[rec.Topic]
		if !ok {
			dropped++
			lastDropErr = fmt.Sprintf("topic mapper missing for %s", rec.Topic)
			return
		}
		alert, err := mapKafkaRecord(rec, mapper, now)
		if err != nil {
			dropped++
			lastDropErr = err.Error()
			return
		}
		alerts = append(alerts, alert)
		toCommit = append(toCommit, rec)
	})

	if len(toCommit) > 0 {
		commitCtx, cancelCommit := context.WithTimeout(ctx, 5*time.Second)
		commitErr := client.CommitRecords(commitCtx, toCommit...)
		cancelCommit()
		if commitErr != nil {
			entry.Error = fmt.Sprintf("kafka commit: %v", commitErr)
			entry.ErrorClass = "commit"
			entry.FetchedCount = len(alerts)
			entry.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			return alerts, entry
		}
	}

	entry.Status = "ok"
	entry.FetchedCount = len(alerts)
	if dropped > 0 {
		entry.ErrorClass = "dropped"
		entry.Error = fmt.Sprintf("dropped=%d last=%s", dropped, strings.TrimSpace(lastDropErr))
	}
	entry.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return alerts, entry
}

func newKafkaClient(cfg config.Config) (*kgo.Client, error) {
	if len(cfg.KafkaBrokers) == 0 {
		return nil, errors.New("KAFKA_BROKERS is required when KAFKA_ENABLED=true")
	}
	if len(cfg.KafkaTopics) == 0 {
		return nil, errors.New("KAFKA_TOPICS is required when KAFKA_ENABLED=true")
	}
	if strings.TrimSpace(cfg.KafkaGroupID) == "" {
		return nil, errors.New("KAFKA_GROUP_ID is required when KAFKA_ENABLED=true")
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.KafkaBrokers...),
		kgo.ConsumeTopics(cfg.KafkaTopics...),
		kgo.ConsumerGroup(cfg.KafkaGroupID),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
		kgo.DisableAutoCommit(),
	}
	if strings.TrimSpace(cfg.KafkaClientID) != "" {
		opts = append(opts, kgo.ClientID(strings.TrimSpace(cfg.KafkaClientID)))
	}

	securityProtocol := strings.ToUpper(strings.TrimSpace(cfg.KafkaSecurityProtocol))
	switch securityProtocol {
	case "", "PLAINTEXT":
	case "SSL":
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: cfg.KafkaTLSInsecureSkipVerify}))
	case "SASL_SSL":
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: cfg.KafkaTLSInsecureSkipVerify}))
		fallthrough
	case "SASL_PLAINTEXT":
		mech, err := kafkaSASLMechanism(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.SASL(mech))
	default:
		return nil, fmt.Errorf("unsupported KAFKA_SECURITY_PROTOCOL %q", securityProtocol)
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func kafkaSASLMechanism(cfg config.Config) (sasl.Mechanism, error) {
	user := strings.TrimSpace(cfg.KafkaUsername)
	pass := strings.TrimSpace(cfg.KafkaPassword)
	if user == "" || pass == "" {
		return nil, errors.New("KAFKA_USERNAME and KAFKA_PASSWORD are required for SASL")
	}
	mech := strings.ToUpper(strings.TrimSpace(cfg.KafkaSASLMechanism))
	switch mech {
	case "", "PLAIN":
		return plain.Auth{User: user, Pass: pass}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: user, Pass: pass}.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return scram.Auth{User: user, Pass: pass}.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("unsupported KAFKA_SASL_MECHANISM %q", mech)
	}
}

func firstFatalKafkaError(fetches kgo.Fetches) error {
	if err := fetches.Err0(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if fetches.IsClientClosed() {
			return err
		}
	}
	for _, fe := range fetches.Errors() {
		if errors.Is(fe.Err, context.Canceled) || errors.Is(fe.Err, context.DeadlineExceeded) {
			continue
		}
		return fe.Err
	}
	return nil
}

func loadKafkaMapper(path string) (kafkaMapperDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return kafkaMapperDocument{}, err
	}
	var doc kafkaMapperDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return kafkaMapperDocument{}, err
	}
	if len(doc.Topics) == 0 {
		return kafkaMapperDocument{}, errors.New("topics mapping is empty")
	}
	for topic, mapper := range doc.Topics {
		if strings.TrimSpace(topic) == "" {
			return kafkaMapperDocument{}, errors.New("topic key must not be empty")
		}
		if strings.TrimSpace(mapper.Source.SourceID) == "" {
			return kafkaMapperDocument{}, fmt.Errorf("topic %s: source.source_id is required", topic)
		}
		if strings.TrimSpace(mapper.Source.AuthorityName) == "" {
			return kafkaMapperDocument{}, fmt.Errorf("topic %s: source.authority_name is required", topic)
		}
		if strings.TrimSpace(mapper.TitlePath) == "" {
			return kafkaMapperDocument{}, fmt.Errorf("topic %s: title_path is required", topic)
		}
	}
	return doc, nil
}

func mapKafkaRecord(rec *kgo.Record, mapper kafkaTopicMapper, now time.Time) (model.Alert, error) {
	var payload map[string]any
	if err := json.Unmarshal(rec.Value, &payload); err != nil {
		return model.Alert{}, fmt.Errorf("topic %s offset %d: invalid json: %w", rec.Topic, rec.Offset, err)
	}

	title := readString(payload, mapper.TitlePath)
	if title == "" {
		title = fmt.Sprintf("Kafka event %s/%d/%d", rec.Topic, rec.Partition, rec.Offset)
	}
	category := readString(payload, mapper.CategoryPath)
	if category == "" {
		category = strings.TrimSpace(mapper.CategoryDefault)
	}
	if category == "" {
		category = "informational"
	}

	severity := normalizeSeverity(readString(payload, mapper.SeverityPath))
	if severity == "" {
		severity = normalizeSeverity(mapper.SeverityDefault)
	}
	if severity == "" {
		severity = "info"
	}

	published := parseKafkaTime(readString(payload, mapper.PublishedAtPath), now)
	freshnessHours := int(now.Sub(published).Hours())
	if freshnessHours < 0 {
		freshnessHours = 0
	}

	source := mapper.Source
	if strings.TrimSpace(source.Country) == "" {
		source.Country = "Unknown"
	}
	if strings.TrimSpace(source.CountryCode) == "" {
		source.CountryCode = "XX"
	}
	if strings.TrimSpace(source.Region) == "" {
		source.Region = "International"
	}
	if strings.TrimSpace(source.AuthorityType) == "" {
		source.AuthorityType = "enterprise_security"
	}
	if strings.TrimSpace(source.BaseURL) == "" {
		source.BaseURL = "kafka://" + rec.Topic
	}

	regionTag := strings.TrimSpace(mapper.RegionTag)
	if regionTag == "" {
		regionTag = source.Region
	}

	canonicalURL := readString(payload, mapper.CanonicalURLPath)
	if canonicalURL == "" {
		canonicalURL = fmt.Sprintf("kafka://%s/%d/%d", rec.Topic, rec.Partition, rec.Offset)
	}

	alert := model.Alert{
		AlertID:          fmt.Sprintf("kafka:%s:%d:%d", rec.Topic, rec.Partition, rec.Offset),
		SourceID:         source.SourceID,
		Source:           source,
		Title:            title,
		CanonicalURL:     canonicalURL,
		FirstSeen:        published.UTC().Format(time.RFC3339),
		LastSeen:         published.UTC().Format(time.RFC3339),
		Status:           "active",
		Category:         category,
		Severity:         severity,
		RegionTag:        regionTag,
		Lat:              readFloat(payload, mapper.LatPath),
		Lng:              readFloat(payload, mapper.LngPath),
		EventCountry:     readString(payload, mapper.EventCountryPath),
		EventCountryCode: strings.ToUpper(readString(payload, mapper.EventCountryCodePath)),
		FreshnessHours:   freshnessHours,
	}
	if alert.Lat != 0 || alert.Lng != 0 {
		alert.EventGeoSource = "kafka"
		alert.EventGeoConfidence = 0.85
	}
	return alert, nil
}

func readString(payload map[string]any, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	v, ok := readPath(payload, path)
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	case json.Number:
		return s.String()
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func readFloat(payload map[string]any, path string) float64 {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	v, ok := readPath(payload, path)
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	default:
		return 0
	}
}

func readPath(payload map[string]any, path string) (any, bool) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return nil, false
	}
	var current any = payload
	for _, p := range parts {
		key := strings.TrimSpace(p)
		if key == "" {
			return nil, false
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[key]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func parseKafkaTime(raw string, fallback time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed
	}
	if millis, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if millis > 1_000_000_000_000 {
			return time.UnixMilli(millis).UTC()
		}
		return time.Unix(millis, 0).UTC()
	}
	return fallback
}

func normalizeSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "crit", "sev1", "p1":
		return "critical"
	case "high", "sev2", "p2":
		return "high"
	case "medium", "med", "sev3", "p3":
		return "medium"
	case "low", "sev4", "p4":
		return "low"
	case "info", "informational":
		return "info"
	default:
		return ""
	}
}
