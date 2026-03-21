import { useEffect, useState } from "react";
import type { ZoneBriefingRecord, ZoneBriefingHotspot, ZoneBriefingConflict, ZoneBriefingACLED, ZoneBriefingMetrics } from "@/types/zone-briefing";

const ZONE_BRIEFINGS_URL = `${import.meta.env.BASE_URL}zone-briefings.json`;
const ZONE_BRIEFINGS_REFRESH_MS = 10000;

function mapHotspot(raw: Record<string, unknown>): ZoneBriefingHotspot {
  return {
    label: (raw.label as string) ?? "",
    lat: (raw.lat as number) ?? 0,
    lng: (raw.lng as number) ?? 0,
    eventCount: (raw.event_count as number) ?? (raw.eventCount as number) ?? 0,
  };
}

function mapConflict(raw: Record<string, unknown>): ZoneBriefingConflict {
  return {
    conflictId: (raw.conflict_id as string) ?? (raw.conflictId as string) ?? "",
    name: (raw.name as string) ?? "",
    type: (raw.type as string) ?? "",
    intensity: (raw.intensity as number) ?? 0,
  };
}

function mapACLED(raw: Record<string, unknown>): ZoneBriefingACLED {
  return {
    events7d: (raw.events_7d as number) ?? (raw.events7d as number) ?? 0,
    fatalities7d: (raw.fatalities_7d as number) ?? (raw.fatalities7d as number) ?? 0,
    topEvent: (raw.top_event as string) ?? (raw.topEvent as string),
    asOf: (raw.as_of as string) ?? (raw.asOf as string),
  };
}

function mapMetrics(raw: Record<string, unknown>): ZoneBriefingMetrics {
  return {
    events7d: (raw.events_7d as number) ?? (raw.events7d as number) ?? 0,
    events30d: (raw.events_30d as number) ?? (raw.events30d as number) ?? 0,
    fatalitiesBest7d: (raw.fatalities_best_7d as number) ?? (raw.fatalitiesBest7d as number) ?? 0,
    fatalitiesBest30d: (raw.fatalities_best_30d as number) ?? (raw.fatalitiesBest30d as number) ?? 0,
    fatalitiesTotal: (raw.fatalities_total as number) ?? (raw.fatalitiesTotal as number) ?? 0,
    civilianDeaths30d: (raw.civilian_deaths_30d as number) ?? (raw.civilianDeaths30d as number) ?? 0,
    trend7d: (raw.trend_7d as string) ?? (raw.trend7d as string),
    trend30d: (raw.trend_30d as string) ?? (raw.trend30d as string),
  };
}

function mapRecord(raw: Record<string, unknown>): ZoneBriefingRecord {
  const hotspots = Array.isArray(raw.hotspots)
    ? (raw.hotspots as Record<string, unknown>[]).map(mapHotspot)
    : undefined;
  const activeConflicts = Array.isArray(raw.active_conflicts ?? raw.activeConflicts)
    ? ((raw.active_conflicts ?? raw.activeConflicts) as Record<string, unknown>[]).map(mapConflict)
    : undefined;
  const acledRaw = (raw.acled_recency ?? raw.acledRecency) as Record<string, unknown> | undefined;
  const acledRecency = acledRaw && typeof acledRaw === "object" ? mapACLED(acledRaw) : undefined;
  const metricsRaw = (raw.metrics as Record<string, unknown> | undefined);
  const metrics = metricsRaw && typeof metricsRaw === "object" ? mapMetrics(metricsRaw) : undefined;
  return {
    lensId: (raw.lens_id as string) ?? (raw.lensId as string) ?? "",
    source: (raw.source as string) ?? "",
    sourceUrl: (raw.source_url as string) ?? (raw.sourceUrl as string),
    status: (raw.status as string),
    updatedAt: (raw.updated_at as string) ?? (raw.updatedAt as string),
    conflictStartDate: (raw.conflict_start_date as string) ?? (raw.conflictStartDate as string),
    coverageNote: (raw.coverage_note as string) ?? (raw.coverageNote as string),
    metrics,
    countryIds: (raw.country_ids as string[]) ?? (raw.countryIds as string[]),
    countryLabels: (raw.country_labels as string[]) ?? (raw.countryLabels as string[]),
    actors: (raw.actors as string[]),
    violenceTypes: (raw.violence_types as string[]) ?? (raw.violenceTypes as string[]),
    hotspots,
    conflictIntensity: (raw.conflict_intensity as string) ?? (raw.conflictIntensity as string),
    conflictType: (raw.conflict_type as string) ?? (raw.conflictType as string),
    activeConflicts,
    acledRecency,
  };
}

function normalizeZoneBriefings(data: unknown): ZoneBriefingRecord[] {
  if (!Array.isArray(data)) return [];
  return data
    .filter((item) => item && typeof item === "object")
    .map((item) => mapRecord(item as Record<string, unknown>))
    .filter((r) => r.lensId !== "");
}

export function useZoneBriefings() {
  const [briefings, setBriefings] = useState<ZoneBriefingRecord[]>([]);

  useEffect(() => {
    let cancelled = false;
    let inflight = false;

    async function load() {
      if (inflight) return;
      inflight = true;
      try {
        const response = await fetch(`${ZONE_BRIEFINGS_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (response.ok) {
          const data = (await response.json()) as unknown;
          if (!cancelled) {
            setBriefings(normalizeZoneBriefings(data));
          }
          inflight = false;
          return;
        }
      } catch {
        if (!cancelled) {
          setBriefings([]);
        }
      } finally {
        inflight = false;
      }
    }

    load();
    const interval = window.setInterval(load, ZONE_BRIEFINGS_REFRESH_MS);
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        load();
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);

  return { briefings };
}
