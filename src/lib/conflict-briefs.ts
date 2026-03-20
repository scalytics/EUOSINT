import type { Alert, AlertCategory, Severity } from "@/types/alert";
import type { ConflictLens } from "@/lib/conflict-lenses";
import type { ZoneBriefingEvent, ZoneBriefingHotspot, ZoneBriefingRecord } from "@/types/zone-briefing";
import { alertMatchesConflictLens } from "@/lib/conflict-lenses";
import { categoryLabels } from "@/lib/severity";

const SEVERITY_WEIGHT: Record<Severity, number> = {
  critical: 4,
  high: 3,
  medium: 2,
  low: 1,
  info: 0,
};

type BriefMetric = {
  label: string;
  value: string;
};

export interface ConflictBrief {
  lens: ConflictLens;
  alerts: Alert[];
  asOf: string | null;
  sourceLabel: string;
  sourceURL?: string;
  coverageNote?: string;
  summaryBullets?: string[];
  watchItems?: string[];
  countryIDs: string[];
  countryLabels: string[];
  metrics: BriefMetric[];
  topCategories: Array<{ key: AlertCategory; label: string; count: number }>;
  topCountries: Array<{ code: string; label: string; count: number }>;
  topSources: Array<{ id: string; label: string; count: number }>;
  actors: string[];
  violenceTypes: string[];
  hotspots: ZoneBriefingHotspot[];
  recentEvents: ZoneBriefingEvent[];
  latestAlert: Alert | null;
  recent7d: number;
  prior7d: number;
  trendLabel: string;
}

function buildRankedList(entries: Map<string, { label: string; count: number }>, limit: number) {
  return [...entries.entries()]
    .map(([key, value]) => ({ key, ...value }))
    .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label))
    .slice(0, limit);
}

function formatAgeHours(hours: number): string {
  if (hours < 1) return "<1h";
  if (hours < 24) return `${Math.round(hours)}h`;
  const days = Math.round(hours / 24);
  return `${days}d`;
}

function formatDateTag(value: string | undefined): string {
  if (!value) return "n/a";
  const ts = Date.parse(value);
  if (Number.isNaN(ts)) return "n/a";
  return new Date(ts).toISOString().slice(0, 10);
}

function formatTrend(recent: number, prior: number): string {
  if (recent === 0 && prior === 0) return "flat";
  if (prior === 0) return `up ${recent}`;
  const delta = ((recent - prior) / prior) * 100;
  if (Math.abs(delta) < 5) return "flat";
  return `${delta > 0 ? "up" : "down"} ${Math.abs(Math.round(delta))}%`;
}

function deriveHotspots(alerts: Alert[], lens: ConflictLens): ZoneBriefingHotspot[] {
  if (alerts.length === 0) return [];
  const latStep = Math.max(0.8, (lens.bounds.north - lens.bounds.south) / 4);
  const lngStep = Math.max(0.8, (lens.bounds.east - lens.bounds.west) / 4);
  const buckets = new Map<string, { latSum: number; lngSum: number; count: number }>();

  for (const alert of alerts) {
    if (alert.lat === 0 && alert.lng === 0) continue;
    const latKey = Math.floor((alert.lat - lens.bounds.south) / latStep);
    const lngKey = Math.floor((alert.lng - lens.bounds.west) / lngStep);
    const key = `${latKey}:${lngKey}`;
    const current = buckets.get(key) ?? { latSum: 0, lngSum: 0, count: 0 };
    current.latSum += alert.lat;
    current.lngSum += alert.lng;
    current.count += 1;
    buckets.set(key, current);
  }

  return [...buckets.entries()]
    .map(([key, bucket]) => ({
      key,
      lat: bucket.latSum / bucket.count,
      lng: bucket.lngSum / bucket.count,
      count: bucket.count,
    }))
    .sort((left, right) => right.count - left.count)
    .slice(0, 4)
    .map((bucket, index) => ({
      label: `Hotspot ${index + 1}`,
      lat: bucket.lat,
      lng: bucket.lng,
      eventCount: bucket.count,
    }));
}

function forceLensActors(lensId: string, actors: string[]): string[] {
  if (lensId !== "red-sea") return actors;
  const forced = "Somali pirate networks";
  const out = [forced, ...actors];
  const seen = new Set<string>();
  return out.filter((actor) => {
    const key = actor.trim().toLowerCase();
    if (!key || seen.has(key)) return false;
    seen.add(key);
    return true;
  }).slice(0, 4);
}

export function buildConflictBrief(alerts: Alert[], lens: ConflictLens | null): ConflictBrief | null {
  if (!lens) return null;

  const lensAlerts = alerts
    .filter((alert) => alertMatchesConflictLens(alert, lens))
    .sort((left, right) => new Date(right.last_seen).getTime() - new Date(left.last_seen).getTime());

  const latestAlert = lensAlerts[0] ?? null;
  const categoryCounts = new Map<string, { label: string; count: number }>();
  const countryCounts = new Map<string, { label: string; count: number }>();
  const sourceCounts = new Map<string, { label: string; count: number }>();

  let critical = 0;
  let high = 0;
  let mapped = 0;
  let severityScore = 0;

  for (const alert of lensAlerts) {
    if (alert.severity === "critical") critical += 1;
    if (alert.severity === "high") high += 1;
    if (alert.lat !== 0 || alert.lng !== 0) mapped += 1;
    severityScore += SEVERITY_WEIGHT[alert.severity];

    const categoryLabel = categoryLabels[alert.category] ?? alert.category;
    const categoryEntry = categoryCounts.get(alert.category) ?? { label: categoryLabel, count: 0 };
    categoryEntry.count += 1;
    categoryCounts.set(alert.category, categoryEntry);

    const countryCode = alert.event_country_code || alert.source.country_code || "XX";
    const countryLabel = alert.event_country || alert.source.country || countryCode;
    const countryEntry = countryCounts.get(countryCode) ?? { label: countryLabel, count: 0 };
    countryEntry.count += 1;
    countryCounts.set(countryCode, countryEntry);

    const sourceEntry = sourceCounts.get(alert.source_id) ?? { label: alert.source.authority_name, count: 0 };
    sourceEntry.count += 1;
    sourceCounts.set(alert.source_id, sourceEntry);
  }

  const freshness = latestAlert ? formatAgeHours(latestAlert.freshness_hours) : "n/a";
  const asOfTime = latestAlert ? new Date(latestAlert.last_seen).getTime() : 0;
  const recent7dCutoff = asOfTime - 7 * 86_400_000;
  const prior7dCutoff = asOfTime - 14 * 86_400_000;
  const recent7d = lensAlerts.filter((alert) => new Date(alert.last_seen).getTime() >= recent7dCutoff).length;
  const prior7d = lensAlerts.filter((alert) => {
    const ts = new Date(alert.last_seen).getTime();
    return ts >= prior7dCutoff && ts < recent7dCutoff;
  }).length;
  const trendLabel = formatTrend(recent7d, prior7d);

  return {
    lens,
    alerts: lensAlerts,
    asOf: latestAlert?.last_seen ?? null,
    sourceLabel: "EUOSINT derived context",
    sourceURL: undefined,
    countryIDs: [],
    countryLabels: [],
    metrics: [
      { label: "Volume", value: String(lensAlerts.length) },
      { label: "Critical", value: String(critical) },
      { label: "High", value: String(high) },
      { label: "Freshest", value: freshness },
      { label: "7d", value: String(recent7d) },
      { label: "Trend", value: trendLabel },
      { label: "Mapped", value: lensAlerts.length === 0 ? "0%" : `${Math.round((mapped / lensAlerts.length) * 100)}%` },
      { label: "Signal", value: String(severityScore) },
    ],
    topCategories: buildRankedList(categoryCounts, 3).map((entry) => ({
      key: entry.key as AlertCategory,
      label: entry.label,
      count: entry.count,
    })),
    topCountries: buildRankedList(countryCounts, 3).map((entry) => ({
      code: entry.key,
      label: entry.label,
      count: entry.count,
    })),
    topSources: buildRankedList(sourceCounts, 3).map((entry) => ({
      id: entry.key,
      label: entry.label,
      count: entry.count,
    })),
    actors: forceLensActors(lens.id, buildRankedList(sourceCounts, 3).map((entry) => entry.label)),
    violenceTypes: buildRankedList(categoryCounts, 3).map((entry) => entry.label),
    hotspots: deriveHotspots(lensAlerts, lens),
    recentEvents: [],
    latestAlert,
    recent7d,
    prior7d,
    trendLabel,
  };
}

export function mergeZoneBriefing(
  brief: ConflictBrief | null,
  override: ZoneBriefingRecord | null | undefined,
): ConflictBrief | null {
  if (!brief) return null;
  if (!override) return brief;
  const zoneMetrics = override.metrics
    ? [
        { label: "Volume (30d)", value: String(override.metrics.events30d) },
        { label: "Casualties (30d)", value: String(override.metrics.fatalitiesBest30d) },
        { label: "Civilian (30d)", value: String(override.metrics.civilianDeaths30d) },
        { label: "Events (7d)", value: String(override.metrics.events7d) },
        { label: "Casualties (7d)", value: String(override.metrics.fatalitiesBest7d) },
        { label: "Trend", value: override.metrics.trend7d || "flat" },
        { label: "Freshest", value: formatDateTag(override.updatedAt) },
      ]
    : brief.metrics;
  const mergedActors = override.actors && override.actors.length > 0 ? override.actors : brief.actors;
  return {
    ...brief,
    metrics: zoneMetrics,
    sourceLabel: override.source || brief.sourceLabel,
    sourceURL: override.sourceUrl ?? brief.sourceURL,
    coverageNote: override.coverageNote ?? brief.coverageNote,
    summaryBullets: override.summary?.bullets && override.summary.bullets.length > 0
      ? override.summary.bullets
      : brief.summaryBullets,
    watchItems: override.summary?.watchItems && override.summary.watchItems.length > 0 ? override.summary.watchItems : brief.watchItems,
    asOf: override.updatedAt ?? brief.asOf,
    countryIDs: override.countryIds && override.countryIds.length > 0 ? override.countryIds : brief.countryIDs,
    countryLabels: override.countryLabels && override.countryLabels.length > 0 ? override.countryLabels : brief.countryLabels,
    topSources: [{ id: "ucdp-ged", label: "UCDP GED", count: override.metrics?.events30d ?? brief.topSources[0]?.count ?? 0 }],
    actors: forceLensActors(brief.lens.id, mergedActors),
    violenceTypes: override.violenceTypes && override.violenceTypes.length > 0 ? override.violenceTypes : brief.violenceTypes,
    hotspots: override.hotspots && override.hotspots.length > 0 ? override.hotspots : brief.hotspots,
    recentEvents: override.recentEvents && override.recentEvents.length > 0 ? override.recentEvents : brief.recentEvents,
  };
}
