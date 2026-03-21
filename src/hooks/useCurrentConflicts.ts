import { useEffect, useState } from "react";
import type { CurrentConflictRecord } from "@/types/current-conflicts";

const CURRENT_CONFLICTS_URL = `${import.meta.env.BASE_URL}ucdp-current-conflicts.json`;
const CURRENT_CONFLICTS_REFRESH_MS = 60000;

function mapConflict(raw: Record<string, unknown>): CurrentConflictRecord {
  return {
    conflictId: (raw.conflict_id as string) ?? (raw.conflictId as string) ?? "",
    title: (raw.title as string) ?? "",
    year: (raw.year as number) ?? 0,
    startDate: (raw.start_date as string) ?? (raw.startDate as string),
    intensityLevel: (raw.intensity_level as number) ?? (raw.intensityLevel as number) ?? 0,
    typeOfConflict: (raw.type_of_conflict as string) ?? (raw.typeOfConflict as string),
    gwnoLoc: (raw.gwno_loc as string) ?? (raw.gwnoLoc as string),
    sideA: (raw.side_a as string) ?? (raw.sideA as string),
    sideB: (raw.side_b as string) ?? (raw.sideB as string),
    lensIds: (raw.lens_ids as string[]) ?? (raw.lensIds as string[]) ?? [],
    sourceUrl: (raw.source_url as string) ?? (raw.sourceUrl as string),
  };
}

function normalize(data: unknown): CurrentConflictRecord[] {
  if (!Array.isArray(data)) return [];
  return data
    .filter((item) => item && typeof item === "object")
    .map((item) => mapConflict(item as Record<string, unknown>))
    .filter((item) => item.conflictId !== "");
}

export function useCurrentConflicts() {
  const [conflicts, setConflicts] = useState<CurrentConflictRecord[]>([]);

  useEffect(() => {
    let cancelled = false;
    let inflight = false;

    async function load() {
      if (inflight) return;
      inflight = true;
      try {
        const response = await fetch(`${CURRENT_CONFLICTS_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (response.ok) {
          const data = (await response.json()) as unknown;
          if (!cancelled) {
            setConflicts(normalize(data));
          }
        }
      } catch {
        if (!cancelled) {
          setConflicts([]);
        }
      } finally {
        inflight = false;
      }
    }

    load();
    const interval = window.setInterval(load, CURRENT_CONFLICTS_REFRESH_MS);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, []);

  return { conflicts };
}
