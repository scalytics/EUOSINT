/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import type { Alert } from "@/types/alert";

type RegionBounds = {
  latMin: number;
  latMax: number;
  lngMin: number;
  lngMax: number;
};

export const CARIBBEAN_INTERACTION_BOUNDS: RegionBounds = {
  latMin: 5,
  latMax: 33,
  lngMin: -118,
  lngMax: -58,
};

function inBounds(lat: number, lng: number, bounds: RegionBounds): boolean {
  return (
    lat >= bounds.latMin &&
    lat <= bounds.latMax &&
    lng >= bounds.lngMin &&
    lng <= bounds.lngMax
  );
}

export function latLngToRegion(lat: number, lng: number): string | null {
  // Caribbean interaction zone includes Mexico + Central America + island states.
  if (inBounds(lat, lng, CARIBBEAN_INTERACTION_BOUNDS)) return "Caribbean";
  // Continental regions — ordered narrow-to-wide to avoid swallowing smaller areas
  if (lat >= 7 && lat <= 84 && lng >= -170 && lng <= -50) return "North America";
  if (lat >= -57 && lat <= 15 && lng >= -82 && lng <= -33) return "South America";
  // Europe: extend south to include Mediterranean, east to cover Turkey/Scandinavia
  if (lat >= 34 && lat <= 72 && lng >= -12 && lng <= 45) return "Europe";
  // Africa: full continent including Madagascar
  if (lat >= -36 && lat <= 38 && lng >= -18 && lng <= 52) return "Africa";
  // Middle East bridges into Asia
  if (lat >= 12 && lat <= 42 && lng >= 34 && lng <= 60) return "Asia";
  // Asia: main body + far-east dateline wrap
  if (lat >= -11 && lat <= 78 && lng >= 40 && lng <= 180) return "Asia";
  if (lat >= 30 && lat <= 78 && lng >= -180 && lng <= -168) return "Asia";
  // Oceania: Australia, NZ, Pacific islands
  if (lat >= -50 && lat <= 0 && lng >= 110 && lng <= 180) return "Oceania";
  if (lat >= -50 && lat <= 15 && lng >= 95 && lng <= 180) return "Oceania";
  return null;
}

export function alertMatchesRegionFilter(alert: Alert, regionFilter: string): boolean {
  if (regionFilter === "all") return true;
  if (regionFilter.startsWith("country:")) {
    return alert.source.country_code === regionFilter.slice(8);
  }
  if (regionFilter === "Caribbean") {
    return (
      alert.source.region === "Caribbean" ||
      inBounds(alert.lat, alert.lng, CARIBBEAN_INTERACTION_BOUNDS)
    );
  }
  return alert.source.region === regionFilter;
}
