import { useEffect, useState } from "react";
import type { ZoneBriefingRecord, ZoneBriefingHotspot } from "@/types/zone-briefing";

const ZONE_BRIEFINGS_URL = `${import.meta.env.BASE_URL}zone-briefings.json`;

function mapHotspot(raw: Record<string, unknown>): ZoneBriefingHotspot {
  return {
    label: (raw.label as string) ?? "",
    lat: (raw.lat as number) ?? 0,
    lng: (raw.lng as number) ?? 0,
    eventCount: (raw.event_count as number) ?? (raw.eventCount as number) ?? 0,
  };
}

function mapRecord(raw: Record<string, unknown>): ZoneBriefingRecord {
  const hotspots = Array.isArray(raw.hotspots)
    ? (raw.hotspots as Record<string, unknown>[]).map(mapHotspot)
    : undefined;
  return {
    lensId: (raw.lens_id as string) ?? (raw.lensId as string) ?? "",
    source: (raw.source as string) ?? "",
    sourceUrl: (raw.source_url as string) ?? (raw.sourceUrl as string),
    status: (raw.status as string),
    updatedAt: (raw.updated_at as string) ?? (raw.updatedAt as string),
    coverageNote: (raw.coverage_note as string) ?? (raw.coverageNote as string),
    countryIds: (raw.country_ids as string[]) ?? (raw.countryIds as string[]),
    countryLabels: (raw.country_labels as string[]) ?? (raw.countryLabels as string[]),
    actors: (raw.actors as string[]),
    violenceTypes: (raw.violence_types as string[]) ?? (raw.violenceTypes as string[]),
    hotspots,
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

    async function load() {
      try {
        const response = await fetch(`${ZONE_BRIEFINGS_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (!response.ok) {
          throw new Error(`zone-briefings fetch failed: ${response.status}`);
        }
        const data = (await response.json()) as unknown;
        if (!cancelled) {
          setBriefings(normalizeZoneBriefings(data));
        }
      } catch {
        if (!cancelled) {
          setBriefings([]);
        }
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, []);

  return { briefings };
}
