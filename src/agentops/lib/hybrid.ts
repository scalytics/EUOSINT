import type { AgentOpsFlow, AgentOpsFusionMatch, AgentOpsMessage } from "@/agentops/types";
import type { Alert } from "@/types/alert";

const CVE_PATTERN = /\bCVE-\d{4}-\d{4,}\b/gi;
const TIME_WINDOW_HOURS = 72;

interface FusionIndicators {
  categories: Set<string>;
  geography: Set<string>;
  sectors: Set<string>;
  vendors: Set<string>;
  products: Set<string>;
  cves: Set<string>;
}

export function buildFusionMatches(flow: AgentOpsFlow | null, messages: AgentOpsMessage[], alerts: Alert[]): AgentOpsFusionMatch[] {
  if (!flow) return [];
  const relatedMessages = messages.filter((message) => message.correlation_id === flow.id);
  const indicators = extractIndicators(relatedMessages);
  const out: AgentOpsFusionMatch[] = [];
  for (const alert of alerts) {
    const reasons = matchReasons(flow, alert, indicators);
    if (reasons.length === 0) continue;
    out.push({
      alert_id: alert.alert_id,
      title: alert.title,
      category: alert.category,
      severity: alert.severity,
      source: alert.source.authority_name,
      canonical_url: alert.canonical_url,
      match_reasons: reasons,
    });
  }
  return out.sort((a, b) => b.match_reasons.length - a.match_reasons.length || a.title.localeCompare(b.title));
}

function matchReasons(flow: AgentOpsFlow, alert: Alert, indicators: FusionIndicators): string[] {
  const haystack = [alert.title, alert.subcategory ?? "", alert.source.authority_name].join(" ").toLowerCase();
  const reasons: string[] = [];
  const flowTime = parseTime(flow.last_seen);
  const alertTime = parseTime(alert.last_seen || alert.first_seen);

  if (indicators.categories.has(alert.category.toLowerCase())) {
    reasons.push(`category:${alert.category}`);
  }
  const alertCountry = (alert.event_country_code || alert.source.country_code || "").toLowerCase();
  if (alertCountry && indicators.geography.has(alertCountry)) {
    reasons.push(`geography:${alertCountry.toUpperCase()}`);
  }
  for (const sector of indicators.sectors) {
    if (haystack.includes(sector)) {
      reasons.push(`sector:${sector}`);
      break;
    }
  }
  for (const vendor of indicators.vendors) {
    if (haystack.includes(vendor)) {
      reasons.push(`vendor:${vendor}`);
      break;
    }
  }
  for (const product of indicators.products) {
    if (haystack.includes(product)) {
      reasons.push(`product:${product}`);
      break;
    }
  }
  const alertCVEs = new Set((alert.title.match(CVE_PATTERN) ?? []).map((item) => item.toUpperCase()));
  for (const cve of indicators.cves) {
    if (alertCVEs.has(cve)) {
      reasons.push(`cve:${cve}`);
      break;
    }
  }
  if (flowTime !== null && alertTime !== null && Math.abs(flowTime - alertTime) <= TIME_WINDOW_HOURS * 60 * 60 * 1000) {
    reasons.push("time-window:72h");
  }
  return reasons;
}

function extractIndicators(messages: AgentOpsMessage[]): FusionIndicators {
  const indicators: FusionIndicators = {
    categories: new Set<string>(),
    geography: new Set<string>(),
    sectors: new Set<string>(),
    vendors: new Set<string>(),
    products: new Set<string>(),
    cves: new Set<string>(),
  };
  for (const message of messages) {
    for (const value of collectValues(parseMessageContent(message))) {
      addIndicators(indicators, value.key, value.value);
    }
    for (const cve of (message.content ?? message.preview ?? "").match(CVE_PATTERN) ?? []) {
      indicators.cves.add(cve.toUpperCase());
    }
  }
  return indicators;
}

function parseMessageContent(message: AgentOpsMessage): unknown {
  if (message.content) {
    try {
      return JSON.parse(message.content);
    } catch {
      return message.content;
    }
  }
  return message.preview ?? "";
}

function collectValues(input: unknown, parentKey = ""): Array<{ key: string; value: string }> {
  if (Array.isArray(input)) {
    return input.flatMap((item) => collectValues(item, parentKey));
  }
  if (input && typeof input === "object") {
    return Object.entries(input as Record<string, unknown>).flatMap(([key, value]) =>
      collectValues(value, parentKey ? `${parentKey}.${key}` : key),
    );
  }
  if (typeof input === "string" || typeof input === "number") {
    const value = String(input).trim();
    if (!value) return [];
    return [{ key: parentKey.toLowerCase(), value }];
  }
  return [];
}

function addIndicators(indicators: FusionIndicators, key: string, rawValue: string) {
  const value = rawValue.trim().toLowerCase();
  if (!value) return;
  if (key.includes("category")) indicators.categories.add(value);
  if (key.includes("country_code") || key.endsWith("country") || key.includes("geography")) indicators.geography.add(value);
  if (key.includes("sector") || key.includes("vertical")) indicators.sectors.add(value);
  if (key.includes("vendor")) indicators.vendors.add(value);
  if (key.includes("product")) indicators.products.add(value);
  if (key.includes("cve")) {
    for (const cve of rawValue.match(CVE_PATTERN) ?? []) {
      indicators.cves.add(cve.toUpperCase());
    }
  }
}

function parseTime(value: string): number | null {
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}
