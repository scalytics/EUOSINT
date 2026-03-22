import { describe, it, expect } from "vitest";
import { detectSpikes } from "./activity-spikes";
import type { Alert } from "@/types/alert";

function makeAlerts(cc: string, country: string, recentCount: number, olderCount: number): Alert[] {
  const now = Date.now();
  const h12 = 12 * 60 * 60 * 1000;
  const d3 = 3 * 24 * 60 * 60 * 1000;

  const alerts: Alert[] = [];
  for (let i = 0; i < recentCount; i++) {
    alerts.push({
      alert_id: `recent-${cc}-${i}`,
      source_id: "src",
      status: "active",
      title: `Recent alert ${i}`,
      canonical_url: `https://example.test/${cc}/${i}`,
      category: "public_safety",
      severity: "high",
      first_seen: new Date(now - h12).toISOString(),
      last_seen: new Date(now - h12).toISOString(),
      region_tag: cc,
      lat: 0,
      lng: 0,
      source: {
        source_id: "src",
        authority_name: "Test",
        country,
        country_code: cc,
        region: "Europe",
      },
    } as Alert);
  }
  for (let i = 0; i < olderCount; i++) {
    alerts.push({
      alert_id: `older-${cc}-${i}`,
      source_id: "src",
      status: "active",
      title: `Older alert ${i}`,
      canonical_url: `https://example.test/${cc}/old/${i}`,
      category: "public_safety",
      severity: "medium",
      first_seen: new Date(now - d3 - i * 60000).toISOString(),
      last_seen: new Date(now - d3 - i * 60000).toISOString(),
      region_tag: cc,
      lat: 0,
      lng: 0,
      source: {
        source_id: "src",
        authority_name: "Test",
        country,
        country_code: cc,
        region: "Europe",
      },
    } as Alert);
  }
  return alerts;
}

describe("detectSpikes", () => {
  it("returns empty for no alerts", () => {
    expect(detectSpikes([])).toEqual([]);
  });

  it("detects elevated spike when recent is 2-4x baseline", () => {
    // 5 alerts in last 24h, 13 older alerts over 13 days = 1/day avg
    // ratio = 5/1 = 5.0 → surge (>=4x)
    // Let's use 3 recent, 13 older → ratio = 3/1 = 3.0 → elevated
    const alerts = makeAlerts("DE", "Germany", 3, 13);
    const spikes = detectSpikes(alerts);
    expect(spikes.length).toBe(1);
    expect(spikes[0].countryCode).toBe("DE");
    expect(spikes[0].level).toBe("elevated");
  });

  it("detects surge when recent is >=4x baseline", () => {
    // 8 alerts in last 24h, 13 older alerts over 13 days = 1/day avg
    // ratio = 8/1 = 8.0 → surge
    const alerts = makeAlerts("UA", "Ukraine", 8, 13);
    const spikes = detectSpikes(alerts);
    expect(spikes.length).toBe(1);
    expect(spikes[0].countryCode).toBe("UA");
    expect(spikes[0].level).toBe("surge");
  });

  it("ignores countries with fewer than 3 recent alerts", () => {
    const alerts = makeAlerts("PT", "Portugal", 2, 20);
    expect(detectSpikes(alerts)).toEqual([]);
  });

  it("ignores INT country code", () => {
    const alerts = makeAlerts("INT", "International", 10, 20);
    expect(detectSpikes(alerts)).toEqual([]);
  });

  it("sorts by ratio descending", () => {
    const alerts = [
      ...makeAlerts("DE", "Germany", 4, 13),
      ...makeAlerts("UA", "Ukraine", 10, 13),
    ];
    const spikes = detectSpikes(alerts);
    expect(spikes.length).toBe(2);
    expect(spikes[0].countryCode).toBe("UA");
    expect(spikes[1].countryCode).toBe("DE");
  });
});
