/*
 * EUOSINT — Activity spike detection
 * Compares recent alert velocity (last 24h) against 14-day baseline
 * to surface countries/regions with unusual activity.
 */

import type { Alert } from "@/types/alert";

export interface ActivitySpike {
  countryCode: string;
  country: string;
  region: string;
  last24h: number;
  dailyAvg: number;
  ratio: number;
  level: "elevated" | "surge";
}

const SPIKE_THRESHOLD = 2.0; // 2x baseline = elevated
const SURGE_THRESHOLD = 4.0; // 4x baseline = surge
const MIN_RECENT = 3; // need at least 3 alerts in 24h to flag

export function detectSpikes(alerts: Alert[]): ActivitySpike[] {
  const now = Date.now();
  const h24 = 24 * 60 * 60 * 1000;
  const windowMs = 14 * 24 * 60 * 60 * 1000;

  // Bucket alerts by country_code
  const buckets = new Map<
    string,
    { country: string; region: string; recent: number; total: number; windowDays: number }
  >();

  for (const alert of alerts) {
    const cc = alert.source.country_code;
    if (!cc || cc === "INT") continue;

    const t = new Date(alert.first_seen).getTime();
    if (isNaN(t)) continue;

    const age = now - t;
    if (age > windowMs) continue;

    let bucket = buckets.get(cc);
    if (!bucket) {
      bucket = {
        country: alert.source.country,
        region: alert.source.region,
        recent: 0,
        total: 0,
        windowDays: 0,
      };
      buckets.set(cc, bucket);
    }

    bucket.total += 1;
    if (age <= h24) {
      bucket.recent += 1;
    }
  }

  // Compute spikes
  const spikes: ActivitySpike[] = [];

  for (const [cc, bucket] of buckets) {
    if (bucket.recent < MIN_RECENT) continue;

    // Daily average over the full window (excluding today to avoid self-comparison)
    const olderAlerts = bucket.total - bucket.recent;
    const olderDays = 13; // 14 day window minus today
    const dailyAvg = olderDays > 0 ? olderAlerts / olderDays : 0;

    if (dailyAvg < 0.5) continue; // not enough baseline data

    const ratio = bucket.recent / dailyAvg;
    if (ratio < SPIKE_THRESHOLD) continue;

    spikes.push({
      countryCode: cc,
      country: bucket.country,
      region: bucket.region,
      last24h: bucket.recent,
      dailyAvg: Math.round(dailyAvg * 10) / 10,
      ratio: Math.round(ratio * 10) / 10,
      level: ratio >= SURGE_THRESHOLD ? "surge" : "elevated",
    });
  }

  return spikes.sort((a, b) => b.ratio - a.ratio);
}
