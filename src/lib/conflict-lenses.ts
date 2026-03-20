import type { OverlayId } from "@/lib/map-overlays";
import type { Alert } from "@/types/alert";

export interface ConflictLens {
  id: string;
  label: string;
  regionFilter: string;
  overlays: OverlayId[];
  description: string;
  countryCodes: string[];
  primaryCountryCodes: string[];
  bounds: {
    south: number;
    west: number;
    north: number;
    east: number;
  };
  viewport: {
    center: [number, number];
    zoom: number;
  };
}

export const CONFLICT_LENSES: ConflictLens[] = [
  {
    id: "gaza",
    label: "Gaza",
    regionFilter: "Asia",
    overlays: ["conflicts", "terrorism"],
    description: "Gaza, southern Israel, and eastern Mediterranean escalation picture.",
    countryCodes: ["PS", "IL", "EG", "LB", "JO"],
    primaryCountryCodes: ["PS", "IL"],
    bounds: { south: 29.5, west: 32.0, north: 34.8, east: 36.5 },
    viewport: { center: [31.35, 34.4], zoom: 8 },
  },
  {
    id: "sudan",
    label: "Sudan",
    regionFilter: "Africa",
    overlays: ["conflicts"],
    description: "Sudan war tracking with Darfur and Khartoum conflict concentration.",
    countryCodes: ["SD", "SS", "TD", "CF", "ET", "ER"],
    primaryCountryCodes: ["SD"],
    bounds: { south: 3.0, west: 21.5, north: 23.5, east: 39.5 },
    viewport: { center: [15.4, 30.2], zoom: 5 },
  },
  {
    id: "ukraine",
    label: "Ukraine South",
    regionFilter: "Europe",
    overlays: ["conflicts", "bases"],
    description: "Southern Ukraine, Black Sea, and adjacent strike and logistics theatre.",
    countryCodes: ["UA", "RU", "RO", "BG", "TR"],
    primaryCountryCodes: ["UA"],
    bounds: { south: 43.0, west: 27.0, north: 49.5, east: 39.5 },
    viewport: { center: [47.1, 34.8], zoom: 6 },
  },
  {
    id: "red-sea",
    label: "Red Sea",
    regionFilter: "Africa",
    overlays: ["ports", "piracy", "terrorism"],
    description: "Red Sea maritime disruption, chokepoints, and Houthi-linked threat picture.",
    countryCodes: ["YE", "SA", "EG", "SD", "ER", "DJ", "SO"],
    primaryCountryCodes: ["YE", "SA", "EG"],
    bounds: { south: 10.0, west: 31.0, north: 31.8, east: 45.5 },
    viewport: { center: [17.8, 42.6], zoom: 5 },
  },
  {
    id: "sahel",
    label: "Sahel",
    regionFilter: "Africa",
    overlays: ["conflicts", "terrorism"],
    description: "Sahel insurgency and cross-border violence across Mali, Niger, and Burkina Faso.",
    countryCodes: ["ML", "NE", "BF", "MR", "DZ", "TD"],
    primaryCountryCodes: ["ML", "NE", "BF"],
    bounds: { south: 10.0, west: -17.5, north: 24.5, east: 25.0 },
    viewport: { center: [15.8, -0.8], zoom: 5 },
  },
  {
    id: "drc-east",
    label: "DRC East",
    regionFilter: "Africa",
    overlays: ["conflicts"],
    description: "Eastern DRC armed group activity and civilian harm concentration.",
    countryCodes: ["CD", "RW", "UG", "BI"],
    primaryCountryCodes: ["CD"],
    bounds: { south: -8.5, west: 27.0, north: 4.5, east: 31.8 },
    viewport: { center: [-1.8, 29.1], zoom: 7 },
  },
];

export function getConflictLensById(id: string | null | undefined): ConflictLens | null {
  if (!id) return null;
  return CONFLICT_LENSES.find((lens) => lens.id === id) ?? null;
}

export function alertMatchesConflictLens(alert: Alert, lens: ConflictLens | null): boolean {
  if (!lens) return true;

  // Lens matching is geography-first. UCDP items are geocoded, so we should
  // avoid broad neighbour-country fallback that pollutes lens context.
  if (alert.lat !== 0 || alert.lng !== 0) {
    if (
      alert.lat >= lens.bounds.south &&
      alert.lat <= lens.bounds.north &&
      alert.lng >= lens.bounds.west &&
      alert.lng <= lens.bounds.east
    ) {
      return true;
    }
  }

  const countryCode = (alert.event_country_code || alert.source.country_code || "").toUpperCase();
  return lens.primaryCountryCodes.includes(countryCode);
}
