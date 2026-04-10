// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.OutputPath == "" || cfg.RegistryPath == "" {
		t.Fatalf("default config should populate output and registry paths: %#v", cfg)
	}
	if cfg.MaxPerSource <= 0 {
		t.Fatalf("unexpected max per source %d", cfg.MaxPerSource)
	}
	if cfg.StructuredDiscoveryIntervalHours != 168 {
		t.Fatalf("unexpected structured discovery interval default %d", cfg.StructuredDiscoveryIntervalHours)
	}
	if !cfg.DiscoverSocialEnabled {
		t.Fatal("expected social discovery to be enabled by default")
	}
	if cfg.XFetchPauseMS <= 0 {
		t.Fatalf("expected X fetch pause default > 0, got %d", cfg.XFetchPauseMS)
	}
	if cfg.KafkaEnabled {
		t.Fatal("expected kafka to be disabled by default")
	}
	if cfg.KafkaGroupID == "" || cfg.KafkaClientID == "" {
		t.Fatalf("expected kafka defaults for group/client id, got group=%q client=%q", cfg.KafkaGroupID, cfg.KafkaClientID)
	}
	if cfg.AgentOpsEnabled {
		t.Fatal("expected agentops to be disabled by default")
	}
	if cfg.AgentOpsGroupID == "" || cfg.AgentOpsClientID == "" {
		t.Fatalf("expected agentops defaults for group/client id, got group=%q client=%q", cfg.AgentOpsGroupID, cfg.AgentOpsClientID)
	}
	if cfg.AgentOpsTopicMode != "auto" {
		t.Fatalf("expected agentops topic mode auto by default, got %q", cfg.AgentOpsTopicMode)
	}
	if cfg.UIMode != "OSINT" || cfg.Profile != "osint-default" {
		t.Fatalf("unexpected UI defaults mode=%q profile=%q", cfg.UIMode, cfg.Profile)
	}
}

func TestLoadStopWordsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stop_words.json")
	content := `{"stop_words":["football","celebrity","grammy"]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	words := loadStopWords(path)
	if len(words) != 3 {
		t.Fatalf("expected 3 stop words, got %d: %v", len(words), words)
	}
	expected := map[string]bool{"football": true, "celebrity": true, "grammy": true}
	for _, w := range words {
		if !expected[w] {
			t.Fatalf("unexpected stop word: %q", w)
		}
	}
}

func TestLoadStopWordsMissingFileReturnsNil(t *testing.T) {
	words := loadStopWords("/nonexistent/path/stop_words.json")
	if words != nil {
		t.Fatalf("expected nil for missing file, got %v", words)
	}
}

func TestLoadStopWordsEmptyPath(t *testing.T) {
	words := loadStopWords("")
	if words != nil {
		t.Fatalf("expected nil for empty path, got %v", words)
	}
}

func TestLoadStopWordsShippedDefault(t *testing.T) {
	// Verify the shipped registry/stop_words.json loads correctly.
	path := filepath.Join("..", "..", "..", "registry", "stop_words.json")
	words := loadStopWords(path)
	if len(words) == 0 {
		t.Fatal("shipped stop_words.json should contain at least one term")
	}
	found := false
	for _, w := range words {
		if w == "football" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("shipped stop_words.json should contain 'football'")
	}
}

func TestDefaultNoisePolicyPath(t *testing.T) {
	cfg := Default()
	if cfg.NoisePolicyPath != "registry/noise_policy.json" {
		t.Fatalf("unexpected default noise policy path %q", cfg.NoisePolicyPath)
	}
	if cfg.NoisePolicyBPath != "" || cfg.NoisePolicyBPercent != 0 {
		t.Fatalf("unexpected noise policy B defaults: path=%q percent=%d", cfg.NoisePolicyBPath, cfg.NoisePolicyBPercent)
	}
	if cfg.NoiseMetricsOutputPath != "public/noise-metrics.json" {
		t.Fatalf("unexpected default noise metrics path %q", cfg.NoiseMetricsOutputPath)
	}
	if cfg.ZoneBriefingsOutputPath != "public/zone-briefings.json" {
		t.Fatalf("unexpected default zone briefings path %q", cfg.ZoneBriefingsOutputPath)
	}
	if cfg.CountryBoundariesPath != "registry/geo/countries-adm0.geojson" {
		t.Fatalf("unexpected default country boundaries path %q", cfg.CountryBoundariesPath)
	}
	if cfg.CollectorRole != "all" {
		t.Fatalf("unexpected collector role default %q", cfg.CollectorRole)
	}
}

func TestNoisePolicyPathFromEnv(t *testing.T) {
	t.Setenv("NOISE_POLICY_PATH", "/tmp/noise_policy.json")
	t.Setenv("NOISE_POLICY_B_PATH", "/tmp/noise_policy_b.json")
	t.Setenv("NOISE_POLICY_B_PERCENT", "25")
	t.Setenv("NOISE_METRICS_OUTPUT_PATH", "/tmp/noise_metrics.json")
	t.Setenv("ZONE_BRIEFINGS_OUTPUT_PATH", "/tmp/zone_briefings.json")
	t.Setenv("COUNTRY_BOUNDARIES_PATH", "/tmp/countries-adm0.geojson")
	t.Setenv("COLLECTOR_ROLE", "api-ucdp")
	t.Setenv("KAFKA_ENABLED", "true")
	t.Setenv("KAFKA_BROKERS", "kafka-1:9092,kafka-2:9092")
	t.Setenv("KAFKA_TOPICS", "alerts-a,alerts-b")
	t.Setenv("KAFKA_GROUP_ID", "euosint-kafka-test")
	t.Setenv("KAFKA_CLIENT_ID", "euosint-kafka-client")
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "sasl_ssl")
	t.Setenv("KAFKA_SASL_MECHANISM", "scram-sha-512")
	t.Setenv("KAFKA_USERNAME", "alice")
	t.Setenv("KAFKA_PASSWORD", "secret")
	t.Setenv("KAFKA_TLS_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("KAFKA_TEST_ON_START", "false")
	t.Setenv("KAFKA_MAX_RECORD_BYTES", "2048")
	t.Setenv("KAFKA_MAX_PER_CYCLE", "123")
	t.Setenv("KAFKA_POLL_TIMEOUT_MS", "3210")
	t.Setenv("KAFKA_MAPPER_PATH", "/tmp/kafka_mapper.json")
	t.Setenv("AGENTOPS_ENABLED", "true")
	t.Setenv("AGENTOPS_BROKERS", "broker-a:9092,broker-b:9092")
	t.Setenv("AGENTOPS_GROUP_NAME", "core")
	t.Setenv("AGENTOPS_GROUP_ID", "euosint-agentops-prod")
	t.Setenv("AGENTOPS_CLIENT_ID", "euosint-agentops-ui")
	t.Setenv("AGENTOPS_TOPIC_MODE", "manual")
	t.Setenv("AGENTOPS_TOPICS", "group.core.requests,group.core.responses")
	t.Setenv("AGENTOPS_SECURITY_PROTOCOL", "sasl_ssl")
	t.Setenv("AGENTOPS_SASL_MECHANISM", "scram-sha-256")
	t.Setenv("AGENTOPS_USERNAME", "bob")
	t.Setenv("AGENTOPS_PASSWORD", "secret-2")
	t.Setenv("AGENTOPS_TLS_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("AGENTOPS_POLICY_PATH", "/config/agentops_policy.yaml")
	t.Setenv("AGENTOPS_REPLAY_ENABLED", "false")
	t.Setenv("AGENTOPS_REPLAY_PREFIX", "replay-core")
	t.Setenv("UI_MODE", "agentops")
	t.Setenv("PROFILE", "agentops-default")
	t.Setenv("UI_POLICY_PATH", "/config/ui_policy.yaml")
	cfg := FromEnv()
	if cfg.NoisePolicyPath != "/tmp/noise_policy.json" {
		t.Fatalf("expected NOISE_POLICY_PATH override, got %q", cfg.NoisePolicyPath)
	}
	if cfg.NoisePolicyBPath != "/tmp/noise_policy_b.json" {
		t.Fatalf("expected NOISE_POLICY_B_PATH override, got %q", cfg.NoisePolicyBPath)
	}
	if cfg.NoisePolicyBPercent != 25 {
		t.Fatalf("expected NOISE_POLICY_B_PERCENT override, got %d", cfg.NoisePolicyBPercent)
	}
	if cfg.NoiseMetricsOutputPath != "/tmp/noise_metrics.json" {
		t.Fatalf("expected NOISE_METRICS_OUTPUT_PATH override, got %q", cfg.NoiseMetricsOutputPath)
	}
	if cfg.ZoneBriefingsOutputPath != "/tmp/zone_briefings.json" {
		t.Fatalf("expected ZONE_BRIEFINGS_OUTPUT_PATH override, got %q", cfg.ZoneBriefingsOutputPath)
	}
	if cfg.CountryBoundariesPath != "/tmp/countries-adm0.geojson" {
		t.Fatalf("expected COUNTRY_BOUNDARIES_PATH override, got %q", cfg.CountryBoundariesPath)
	}
	if cfg.CollectorRole != "api-ucdp" {
		t.Fatalf("expected COLLECTOR_ROLE override, got %q", cfg.CollectorRole)
	}
	if !cfg.KafkaEnabled {
		t.Fatal("expected KAFKA_ENABLED override")
	}
	if len(cfg.KafkaBrokers) != 2 || cfg.KafkaBrokers[0] != "kafka-1:9092" {
		t.Fatalf("unexpected kafka brokers: %#v", cfg.KafkaBrokers)
	}
	if len(cfg.KafkaTopics) != 2 || cfg.KafkaTopics[1] != "alerts-b" {
		t.Fatalf("unexpected kafka topics: %#v", cfg.KafkaTopics)
	}
	if cfg.KafkaGroupID != "euosint-kafka-test" || cfg.KafkaClientID != "euosint-kafka-client" {
		t.Fatalf("unexpected kafka ids group=%q client=%q", cfg.KafkaGroupID, cfg.KafkaClientID)
	}
	if cfg.KafkaSecurityProtocol != "SASL_SSL" || cfg.KafkaSASLMechanism != "SCRAM-SHA-512" {
		t.Fatalf("unexpected kafka security config protocol=%q mech=%q", cfg.KafkaSecurityProtocol, cfg.KafkaSASLMechanism)
	}
	if cfg.KafkaUsername != "alice" || cfg.KafkaPassword != "secret" {
		t.Fatalf("unexpected kafka credentials user=%q pass=%q", cfg.KafkaUsername, cfg.KafkaPassword)
	}
	if !cfg.KafkaTLSInsecureSkipVerify {
		t.Fatal("expected KAFKA_TLS_INSECURE_SKIP_VERIFY override")
	}
	if cfg.KafkaTestOnStart {
		t.Fatal("expected KAFKA_TEST_ON_START override")
	}
	if cfg.KafkaMaxRecordBytes != 2048 || cfg.KafkaMaxPerCycle != 123 || cfg.KafkaPollTimeoutMS != 3210 {
		t.Fatalf("unexpected kafka limits bytes=%d perCycle=%d timeout=%d", cfg.KafkaMaxRecordBytes, cfg.KafkaMaxPerCycle, cfg.KafkaPollTimeoutMS)
	}
	if cfg.KafkaMapperPath != "/tmp/kafka_mapper.json" {
		t.Fatalf("expected KAFKA_MAPPER_PATH override, got %q", cfg.KafkaMapperPath)
	}
	if !cfg.AgentOpsEnabled {
		t.Fatal("expected AGENTOPS_ENABLED override")
	}
	if len(cfg.AgentOpsBrokers) != 2 || cfg.AgentOpsBrokers[1] != "broker-b:9092" {
		t.Fatalf("unexpected agentops brokers: %#v", cfg.AgentOpsBrokers)
	}
	if cfg.AgentOpsGroupName != "core" || cfg.AgentOpsGroupID != "euosint-agentops-prod" || cfg.AgentOpsClientID != "euosint-agentops-ui" {
		t.Fatalf("unexpected agentops ids name=%q group=%q client=%q", cfg.AgentOpsGroupName, cfg.AgentOpsGroupID, cfg.AgentOpsClientID)
	}
	if cfg.AgentOpsTopicMode != "manual" {
		t.Fatalf("expected AGENTOPS_TOPIC_MODE override, got %q", cfg.AgentOpsTopicMode)
	}
	if len(cfg.AgentOpsTopics) != 2 || cfg.AgentOpsTopics[0] != "group.core.requests" {
		t.Fatalf("unexpected agentops topics: %#v", cfg.AgentOpsTopics)
	}
	if cfg.AgentOpsSecurityProtocol != "SASL_SSL" || cfg.AgentOpsSASLMechanism != "SCRAM-SHA-256" {
		t.Fatalf("unexpected agentops security protocol=%q mech=%q", cfg.AgentOpsSecurityProtocol, cfg.AgentOpsSASLMechanism)
	}
	if cfg.AgentOpsUsername != "bob" || cfg.AgentOpsPassword != "secret-2" {
		t.Fatalf("unexpected agentops credentials user=%q pass=%q", cfg.AgentOpsUsername, cfg.AgentOpsPassword)
	}
	if !cfg.AgentOpsTLSInsecureSkipVerify {
		t.Fatal("expected AGENTOPS_TLS_INSECURE_SKIP_VERIFY override")
	}
	if cfg.AgentOpsPolicyPath != "/config/agentops_policy.yaml" || cfg.AgentOpsReplayEnabled || cfg.AgentOpsReplayPrefix != "replay-core" {
		t.Fatalf("unexpected agentops policy/replay path=%q enabled=%v prefix=%q", cfg.AgentOpsPolicyPath, cfg.AgentOpsReplayEnabled, cfg.AgentOpsReplayPrefix)
	}
	if cfg.UIMode != "AGENTOPS" || cfg.Profile != "agentops-default" || cfg.UIPolicyPath != "/config/ui_policy.yaml" {
		t.Fatalf("unexpected UI config mode=%q profile=%q policy=%q", cfg.UIMode, cfg.Profile, cfg.UIPolicyPath)
	}
}

func TestAgentOpsInvalidEnumsFallbackToDefaults(t *testing.T) {
	t.Setenv("AGENTOPS_TOPIC_MODE", "broken")
	t.Setenv("UI_MODE", "broken")
	t.Setenv("PROFILE", "broken")

	cfg := FromEnv()
	if cfg.AgentOpsTopicMode != "auto" {
		t.Fatalf("expected invalid topic mode to fall back to auto, got %q", cfg.AgentOpsTopicMode)
	}
	if cfg.UIMode != "OSINT" {
		t.Fatalf("expected invalid UI mode to fall back to OSINT, got %q", cfg.UIMode)
	}
	if cfg.Profile != "osint-default" {
		t.Fatalf("expected invalid profile to fall back to osint-default, got %q", cfg.Profile)
	}
}
