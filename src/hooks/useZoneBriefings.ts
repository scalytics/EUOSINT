import { useEffect, useState } from "react";
import type { ZoneBriefingRecord } from "@/types/zone-briefing";

const ZONE_BRIEFINGS_URL = `${import.meta.env.BASE_URL}zone-briefings.json`;
const ZONE_BRIEFINGS_API_URL = "/api/zone-briefings";

function normalizeZoneBriefings(data: unknown): ZoneBriefingRecord[] {
  if (!Array.isArray(data)) return [];
  return data
    .filter((item) => item && typeof item === "object")
    .map((item) => {
      const raw = item as Record<string, unknown>;
      const hotspots = Array.isArray(raw.hotspots)
        ? raw.hotspots
            .filter((hotspot) => hotspot && typeof hotspot === "object")
            .map((hotspot) => {
              const h = hotspot as Record<string, unknown>;
              return {
                label: typeof h.label === "string" ? h.label : "",
                lat: typeof h.lat === "number" ? h.lat : 0,
                lng: typeof h.lng === "number" ? h.lng : 0,
                eventCount:
                  typeof h.eventCount === "number"
                    ? h.eventCount
                    : typeof h.event_count === "number"
                      ? h.event_count
                      : 0,
              };
            })
        : [];
      const metricsRaw =
        raw.metrics && typeof raw.metrics === "object"
          ? (raw.metrics as Record<string, unknown>)
          : null;
      const summaryRaw =
        raw.summary && typeof raw.summary === "object"
          ? (raw.summary as Record<string, unknown>)
          : null;
      const recentEventsRaw = Array.isArray(raw.recentEvents)
        ? raw.recentEvents
        : Array.isArray(raw.recent_events)
          ? raw.recent_events
          : [];

      return {
        lensId:
          typeof raw.lensId === "string"
            ? raw.lensId
            : typeof raw.lens_id === "string"
              ? raw.lens_id
              : "",
        source: typeof raw.source === "string" ? raw.source : "",
        title:
          typeof raw.title === "string"
            ? raw.title
            : undefined,
        sourceUrl:
          typeof raw.sourceUrl === "string"
            ? raw.sourceUrl
            : typeof raw.source_url === "string"
              ? raw.source_url
              : undefined,
        status: typeof raw.status === "string" ? raw.status : undefined,
        updatedAt:
          typeof raw.updatedAt === "string"
            ? raw.updatedAt
            : typeof raw.updated_at === "string"
              ? raw.updated_at
              : undefined,
        coverageNote:
          typeof raw.coverageNote === "string"
            ? raw.coverageNote
            : typeof raw.coverage_note === "string"
              ? raw.coverage_note
              : undefined,
        countryIds: Array.isArray(raw.countryIds)
          ? (raw.countryIds.filter((value): value is string => typeof value === "string"))
          : Array.isArray(raw.country_ids)
            ? (raw.country_ids.filter((value): value is string => typeof value === "string"))
            : undefined,
        countryLabels: Array.isArray(raw.countryLabels)
          ? (raw.countryLabels.filter((value): value is string => typeof value === "string"))
          : Array.isArray(raw.country_labels)
            ? (raw.country_labels.filter((value): value is string => typeof value === "string"))
            : undefined,
        actors: Array.isArray(raw.actors)
          ? raw.actors.filter((value): value is string => typeof value === "string")
          : undefined,
        violenceTypes: Array.isArray(raw.violenceTypes)
          ? raw.violenceTypes.filter((value): value is string => typeof value === "string")
          : Array.isArray(raw.violence_types)
            ? raw.violence_types.filter((value): value is string => typeof value === "string")
            : undefined,
        hotspots: hotspots.length > 0 ? hotspots : undefined,
        metrics: metricsRaw
          ? {
              events7d:
                typeof metricsRaw.events7d === "number"
                  ? metricsRaw.events7d
                  : typeof metricsRaw.events_7d === "number"
                    ? metricsRaw.events_7d
                    : 0,
              events30d:
                typeof metricsRaw.events30d === "number"
                  ? metricsRaw.events30d
                  : typeof metricsRaw.events_30d === "number"
                    ? metricsRaw.events_30d
                    : 0,
              fatalitiesBest7d:
                typeof metricsRaw.fatalitiesBest7d === "number"
                  ? metricsRaw.fatalitiesBest7d
                  : typeof metricsRaw.fatalities_best_7d === "number"
                    ? metricsRaw.fatalities_best_7d
                    : 0,
              fatalitiesBest30d:
                typeof metricsRaw.fatalitiesBest30d === "number"
                  ? metricsRaw.fatalitiesBest30d
                  : typeof metricsRaw.fatalities_best_30d === "number"
                    ? metricsRaw.fatalities_best_30d
                    : 0,
              civilianDeaths30d:
                typeof metricsRaw.civilianDeaths30d === "number"
                  ? metricsRaw.civilianDeaths30d
                  : typeof metricsRaw.civilian_deaths_30d === "number"
                    ? metricsRaw.civilian_deaths_30d
                    : 0,
              trend7d:
                typeof metricsRaw.trend7d === "string"
                  ? metricsRaw.trend7d
                  : typeof metricsRaw.trend_7d === "string"
                    ? metricsRaw.trend_7d
                    : "flat",
              trend30d:
                typeof metricsRaw.trend30d === "string"
                  ? metricsRaw.trend30d
                  : typeof metricsRaw.trend_30d === "string"
                    ? metricsRaw.trend_30d
                    : "flat",
            }
          : undefined,
        summary: summaryRaw
          ? {
              headline: typeof summaryRaw.headline === "string" ? summaryRaw.headline : undefined,
              bullets: Array.isArray(summaryRaw.bullets)
                ? summaryRaw.bullets.filter((value): value is string => typeof value === "string")
                : undefined,
              watchItems: Array.isArray(summaryRaw.watchItems)
                ? summaryRaw.watchItems.filter((value): value is string => typeof value === "string")
                : Array.isArray(summaryRaw.watch_items)
                  ? summaryRaw.watch_items.filter((value): value is string => typeof value === "string")
                  : undefined,
            }
          : undefined,
        recentEvents: recentEventsRaw
          .filter((event) => event && typeof event === "object")
          .map((event) => {
            const e = event as Record<string, unknown>;
            return {
              title: typeof e.title === "string" ? e.title : "",
              published: typeof e.published === "string" ? e.published : undefined,
              country: typeof e.country === "string" ? e.country : undefined,
              countryCode:
                typeof e.countryCode === "string"
                  ? e.countryCode
                  : typeof e.country_code === "string"
                    ? e.country_code
                    : undefined,
              fatalities: typeof e.fatalities === "number" ? e.fatalities : undefined,
              civilianDeaths:
                typeof e.civilianDeaths === "number"
                  ? e.civilianDeaths
                  : typeof e.civilian_deaths === "number"
                    ? e.civilian_deaths
                    : undefined,
              lat: typeof e.lat === "number" ? e.lat : undefined,
              lng: typeof e.lng === "number" ? e.lng : undefined,
              link: typeof e.link === "string" ? e.link : undefined,
            };
          })
          .filter((event) => event.title.trim() !== ""),
      } as ZoneBriefingRecord;
    })
    .filter((briefing) => briefing.lensId.trim() !== "");
}

export function useZoneBriefings() {
  const [briefings, setBriefings] = useState<ZoneBriefingRecord[]>([]);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const response = await fetch(`${ZONE_BRIEFINGS_API_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (response.ok) {
          const data = (await response.json()) as unknown;
          if (!cancelled) {
            setBriefings(normalizeZoneBriefings(data));
          }
          return;
        }
      } catch {
        // Fall through to static artifact fallback.
      }

      try {
        const fallbackResponse = await fetch(`${ZONE_BRIEFINGS_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (!fallbackResponse.ok) {
          throw new Error(`zone-briefings fallback fetch failed: ${fallbackResponse.status}`);
        }
        const fallbackData = (await fallbackResponse.json()) as unknown;
        if (!cancelled) {
          setBriefings(normalizeZoneBriefings(fallbackData));
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
