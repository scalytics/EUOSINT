import { useEffect, useState } from "react";
import type { ConflictStatRecord, ConflictCountryStat } from "@/types/conflict-stats";

const CONFLICT_STATS_URL = `${import.meta.env.BASE_URL}ucdp-conflict-stats.json`;
const CONFLICT_STATS_REFRESH_MS = 60000;
const URL_CANDIDATES = [
  CONFLICT_STATS_URL,
  "/ucdp-conflict-stats.json",
  "ucdp-conflict-stats.json",
];

function mapCountry(raw: Record<string, unknown>): ConflictCountryStat {
  return {
    gwno: (raw.gwno as string) ?? "",
    iso2: (raw.iso2 as string) ?? "",
    label: (raw.label as string) ?? "",
    fatalitiesTotal: (raw.fatalities_total as number) ?? (raw.fatalitiesTotal as number) ?? 0,
    fatalitiesLatest: (raw.fatalities_latest as number) ?? (raw.fatalitiesLatest as number) ?? 0,
    latestYear: (raw.latest_year as number) ?? (raw.latestYear as number) ?? 0,
  };
}

function mapConflict(raw: Record<string, unknown>): ConflictStatRecord {
  return {
    conflictId: (raw.conflict_id as string) ?? (raw.conflictId as string) ?? "",
    title: (raw.title as string) ?? "",
    year: (raw.year as number) ?? 0,
    startDate: (raw.start_date as string) ?? (raw.startDate as string),
    intensityLevel: (raw.intensity_level as number) ?? (raw.intensityLevel as number) ?? 0,
    typeOfConflict: (raw.type_of_conflict as string) ?? (raw.typeOfConflict as string),
    region: (raw.region as string) ?? "",
    sideA: (raw.side_a as string) ?? (raw.sideA as string),
    sideB: (raw.side_b as string) ?? (raw.sideB as string),
    lensIds: (raw.lens_ids as string[]) ?? (raw.lensIds as string[]) ?? [],
    overlayCountryCodes: (raw.overlay_country_codes as string[]) ?? (raw.overlayCountryCodes as string[]) ?? [],
    sourceUrl: (raw.source_url as string) ?? (raw.sourceUrl as string),
    fatalitiesTotal: (raw.fatalities_total as number) ?? (raw.fatalitiesTotal as number) ?? 0,
    fatalitiesLatestYear: (raw.fatalities_latest_year as number) ?? (raw.fatalitiesLatestYear as number) ?? 0,
    fatalitiesLatestYearYear: (raw.fatalities_latest_year_year as number) ?? (raw.fatalitiesLatestYearYear as number) ?? 0,
    countries: Array.isArray(raw.countries)
      ? (raw.countries as Record<string, unknown>[]).map(mapCountry)
      : [],
  };
}

function normalize(data: unknown): ConflictStatRecord[] {
  if (!Array.isArray(data)) return [];
  return data
    .filter((item) => item && typeof item === "object")
    .map((item) => mapConflict(item as Record<string, unknown>))
    .filter((item) => item.conflictId !== "");
}

export function useConflictStats() {
  const [stats, setStats] = useState<ConflictStatRecord[]>([]);

  useEffect(() => {
    let cancelled = false;
    let inflight = false;

    async function load() {
      if (inflight) return;
      inflight = true;
      try {
        const stamp = Date.now();
        for (const candidate of URL_CANDIDATES) {
          const url = `${candidate}${candidate.includes("?") ? "&" : "?"}t=${stamp}`;
          const response = await fetch(url, { cache: "no-store" });
          if (!response.ok) continue;
          const data = (await response.json()) as unknown;
          if (!cancelled) {
            setStats(normalize(data));
          }
          return;
        }
      } catch {
        // Keep last-known good stats.
      } finally {
        inflight = false;
      }
    }

    load();
    const interval = window.setInterval(load, CONFLICT_STATS_REFRESH_MS);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, []);

  return { stats };
}
