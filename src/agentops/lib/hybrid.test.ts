import { describe, expect, test } from "vitest";
import { buildFusionMatches } from "@/agentops/lib/hybrid";
import type { AgentOpsFlow, AgentOpsMessage } from "@/agentops/types";
import type { Alert } from "@/types/alert";

const flow: AgentOpsFlow = {
  id: "corr-1",
  topic_count: 1,
  sender_count: 1,
  topics: ["group.core.requests"],
  senders: ["worker-a"],
  trace_ids: [],
  task_ids: [],
  first_seen: "2026-04-10T12:00:00Z",
  last_seen: "2026-04-10T12:00:00Z",
  message_count: 1,
};

function baseAlert(overrides: Partial<Alert>): Alert {
  return {
    alert_id: "a1",
    source_id: "src",
    source: {
      source_id: "src",
      authority_name: "CERT-UA",
      country: "Ukraine",
      country_code: "UA",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://example.test",
    },
    title: "Default alert",
    canonical_url: "https://example.test/a1",
    first_seen: "2026-04-10T10:00:00Z",
    last_seen: "2026-04-10T11:00:00Z",
    status: "active",
    category: "cyber_advisory",
    severity: "high",
    region_tag: "EU",
    lat: 0,
    lng: 0,
    freshness_hours: 1,
    ...overrides,
  };
}

function jsonMessage(payload: string): AgentOpsMessage {
  return {
    id: "m1",
    topic: "group.core.requests",
    topic_family: "requests",
    partition: 0,
    offset: 1,
    timestamp: "2026-04-10T12:00:00Z",
    correlation_id: "corr-1",
    content: payload,
  };
}

describe("buildFusionMatches", () => {
  test("matches on registry category", () => {
    const matches = buildFusionMatches(flow, [jsonMessage(`{"category":"cyber_advisory"}`)], [baseAlert({})]);
    expect(matches[0]?.match_reasons).toContain("category:cyber_advisory");
  });

  test("matches on geography", () => {
    const matches = buildFusionMatches(flow, [jsonMessage(`{"event_country_code":"ua"}`)], [baseAlert({})]);
    expect(matches[0]?.match_reasons).toContain("geography:UA");
  });

  test("matches on sector", () => {
    const matches = buildFusionMatches(flow, [jsonMessage(`{"sector":"energy"}`)], [baseAlert({ title: "Energy sector malware campaign" })]);
    expect(matches[0]?.match_reasons).toContain("sector:energy");
  });

  test("matches on vendor and product", () => {
    const matches = buildFusionMatches(
      flow,
      [jsonMessage(`{"vendor":"splunk","product":"f5"}`)],
      [baseAlert({ title: "Splunk F5 telemetry exploit bulletin" })],
    );
    expect(matches[0]?.match_reasons.join(" ")).toContain("vendor:splunk");
    expect(matches[0]?.match_reasons.join(" ")).toContain("product:f5");
  });

  test("matches on cve", () => {
    const matches = buildFusionMatches(flow, [jsonMessage(`{"cve":"CVE-2026-12345"}`)], [baseAlert({ title: "Advisory for CVE-2026-12345" })]);
    expect(matches[0]?.match_reasons).toContain("cve:CVE-2026-12345");
  });

  test("matches on time window proximity", () => {
    const matches = buildFusionMatches(flow, [jsonMessage(`{"category":"other"}`)], [baseAlert({ title: "Unrelated but nearby" })]);
    expect(matches[0]?.match_reasons).toContain("time-window:72h");
  });
});
