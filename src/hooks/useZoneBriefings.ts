import { useEffect, useState } from "react";
import type { ZoneBriefingRecord } from "@/types/zone-briefing";

const ZONE_BRIEFINGS_URL = `${import.meta.env.BASE_URL}zone-briefings.json`;

function normalizeZoneBriefings(data: unknown): ZoneBriefingRecord[] {
  if (!Array.isArray(data)) return [];
  return data.filter((item) => item && typeof item === "object") as ZoneBriefingRecord[];
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
