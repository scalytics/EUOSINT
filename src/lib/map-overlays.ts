import L from "leaflet";
import { latLngToRegion } from "@/lib/regions";

export type OverlayId = string;

export interface OverlayDef {
  id: OverlayId;
  label: string;
  color: string;
  url: string;
}

type GeoJSONLike = Record<string, unknown>;
type GeoJSONFeature = { properties?: Record<string, unknown> };
type GeoJSONFeatureItem = { geometry?: { type?: string; coordinates?: unknown }; properties?: Record<string, unknown> };

export interface OverlayLoadOptions {
  regionFilter?: string;
  conflictLensId?: string | null;
  onConflictCountrySelect?: (countryCode: string, countryLabel: string, lensId?: string) => void;
}

const OVERLAY_FALLBACK_URLS: Record<string, string> = {
  conflicts: "/geo/conflict-zones.seed.geojson",
  terrorism: "/geo/terrorism-zones.seed.geojson",
  bases: "/geo/military-bases.geojson",
};

const LENS_REGION_FILTERS: Record<string, string> = {
  gaza: "Asia",
  sudan: "Africa",
  ukraine: "Europe",
  "red-sea": "Africa",
  sahel: "Africa",
  "drc-east": "Africa",
};

const BASES_REGION_URLS: Record<string, string> = {
  Europe: "/geo/military-bases.europe.geojson",
  Africa: "/geo/military-bases.africa.geojson",
  "North America": "/geo/military-bases.north-america.geojson",
  Asia: "/geo/military-bases.asia.geojson",
  all: "/geo/military-bases.all.geojson",
};

function normalizeOverlayRegion(regionFilter: string): string {
  if (BASES_REGION_URLS[regionFilter]) return regionFilter;
  const value = regionFilter.trim().toLowerCase();
  if (value.includes("europe")) return "Europe";
  if (value.includes("africa")) return "Africa";
  if (value.includes("asia") || value.includes("middle east")) return "Asia";
  if (value.includes("north america") || value.includes("caribbean")) return "North America";
  if (value.includes("south america") || value.includes("latin america")) return "all";
  if (value.includes("oceania")) return "all";
  return "all";
}

export const DEFAULT_OVERLAYS: OverlayDef[] = [
  { id: "conflicts", label: "Conflict Zones", color: "#ff5d5d", url: "/geo/conflict-zones.geojson" },
  { id: "cables", label: "Undersea Cables", color: "#60a5fa", url: "/geo/submarine-cables.geojson" },
  { id: "shipping", label: "Shipping Lanes", color: "#4ccb8d", url: "/geo/shipping-lanes.geojson" },
  { id: "ports", label: "Strategic Ports", color: "#f29d4b", url: "/geo/strategic-ports.geojson" },
  { id: "bases", label: "Military Bases", color: "#a78bfa", url: "/geo/military-bases.geojson" },
  { id: "nuclear", label: "Nuclear Sites", color: "#facc15", url: "/geo/nuclear-sites.geojson" },
  { id: "sanctions", label: "Sanctions Zones", color: "#f87171", url: "/geo/sanctions-zones.geojson" },
  { id: "piracy", label: "Piracy Zones", color: "#38bdf8", url: "/geo/piracy-zones.geojson" },
  { id: "terrorism", label: "Terror Zones", color: "#e879f9", url: "/geo/terrorism-zones.geojson" },
];

const EMPTY_FEATURE_COLLECTION: GeoJSONLike = { type: "FeatureCollection", features: [] };

function hasGeoJSONFeatures(value: GeoJSONLike | null): value is GeoJSONLike {
  if (!value) return false;
  const features = (value as { features?: unknown[] }).features;
  return Array.isArray(features) && features.length > 0;
}

async function fetchGeoJSON(url: string): Promise<GeoJSONLike | null> {
  const resp = await fetch(url);
  if (!resp.ok) {
    return null;
  }
  const contentType = (resp.headers.get("content-type") || "").toLowerCase();
  if (!contentType.includes("json")) {
    return null;
  }
  try {
    return (await resp.json()) as GeoJSONLike;
  } catch {
    return null;
  }
}

export async function loadOverlayDefs(): Promise<OverlayDef[]> {
  try {
    const resp = await fetch("/geo/overlays.json", { cache: "no-cache" });
    if (!resp.ok) {
      return DEFAULT_OVERLAYS;
    }
    const raw = (await resp.json()) as { overlays?: OverlayDef[] };
    const overlays = Array.isArray(raw?.overlays) ? raw.overlays : [];
    const normalized = overlays
      .filter((o) => typeof o?.id === "string" && typeof o?.url === "string")
      .map((o) => ({
        id: o.id.trim(),
        label: (o.label ?? o.id).trim(),
        color: (o.color ?? "#94a3b8").trim(),
        url: o.id.trim() === "conflicts" ? "/geo/conflict-zones.geojson" : o.url.trim(),
      }))
      .filter((o) => o.id !== "" && o.url !== "");
    return normalized.length > 0 ? normalized : DEFAULT_OVERLAYS;
  } catch {
    return DEFAULT_OVERLAYS;
  }
}

const conflictTypeColors: Record<string, string> = {
  active_war: "#ff3333",
  active_conflict: "#ff6b3d",
  insurgency: "#e8913a",
  high_tension: "#c9c23a",
  frozen_conflict: "#7a8899",
  low_intensity: "#b8863a",
};

const portTypeIcons: Record<string, string> = {
  container: "\u2693",  // anchor
  canal: "\u26F5",      // sailboat
  military: "\u2694",   // swords
  strategic: "\u25C6",  // diamond
};

const baseTypeIcons: Record<string, string> = {
  air: "\u2708",        // airplane
  naval: "\u2693",      // anchor
  army: "\u2694",       // swords
  joint: "\u2605",      // star
  intelligence: "\u25C9", // fisheye
};

function pickString(props: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = props[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }
  return "";
}

function collectLonLat(coords: unknown, out: Array<[number, number]>): void {
  if (!Array.isArray(coords)) return;
  if (coords.length >= 2 && typeof coords[0] === "number" && typeof coords[1] === "number") {
    out.push([coords[0], coords[1]]);
    return;
  }
  for (const item of coords) {
    collectLonLat(item, out);
  }
}

function geometryCenterLatLng(geometry: { type?: string; coordinates?: unknown } | undefined): L.LatLng | null {
  if (!geometry || !geometry.type) return null;
  if (geometry.type === "Point" && Array.isArray(geometry.coordinates) && geometry.coordinates.length >= 2) {
    const lon = geometry.coordinates[0];
    const lat = geometry.coordinates[1];
    if (typeof lon === "number" && typeof lat === "number") {
      return L.latLng(lat, lon);
    }
    return null;
  }
  const points: Array<[number, number]> = [];
  collectLonLat(geometry.coordinates, points);
  if (points.length === 0) return null;
  let minLon = points[0][0];
  let maxLon = points[0][0];
  let minLat = points[0][1];
  let maxLat = points[0][1];
  for (const [lon, lat] of points) {
    if (lon < minLon) minLon = lon;
    if (lon > maxLon) maxLon = lon;
    if (lat < minLat) minLat = lat;
    if (lat > maxLat) maxLat = lat;
  }
  return L.latLng((minLat + maxLat) / 2, (minLon + maxLon) / 2);
}

function inferBaseType(name: string, description: string, component: string, isJoint: boolean): string {
  if (isJoint) return "joint";
  const text = `${name} ${description} ${component}`.toLowerCase();
  if (text.includes("intel")) return "intelligence";
  if (text.includes("air") || text.includes("afb") || text.includes("air force")) return "air";
  if (text.includes("naval") || text.includes("navy") || text.includes("shipyard")) return "naval";
  if (text.includes("army") || text.includes("fort") || text.includes("camp")) return "army";
  return "joint";
}

const sanctionTypeColors: Record<string, string> = {
  comprehensive: "#f87171",
  sectoral: "#fb923c",
  targeted: "#fbbf24",
};

const piracyTypeColors: Record<string, string> = {
  high_risk: "#ef4444",
  elevated: "#f59e0b",
  moderate: "#38bdf8",
};

const terrorismTypeColors: Record<string, string> = {
  active: "#e879f9",
  degraded: "#a78bfa",
  elevated: "#f472b6",
};

export async function loadOverlay(
  map: L.Map,
  def: OverlayDef,
  options: OverlayLoadOptions = {},
): Promise<L.LayerGroup> {
  const regionFilter = options.regionFilter ?? "all";
  const normalizedRegion = normalizeOverlayRegion(regionFilter);
  const conflictLensId = (options.conflictLensId ?? "").trim();
  const onConflictCountrySelect = options.onConflictCountrySelect;
  let url = def.id === "bases"
    ? (BASES_REGION_URLS[normalizedRegion] ?? BASES_REGION_URLS.all)
    : def.url;
  if (conflictLensId !== "") {
    if (def.id === "conflicts") {
      url = `/geo/conflict-zones.${conflictLensId}.geojson`;
    } else if (def.id === "terrorism") {
      url = `/geo/terrorism-zones.${conflictLensId}.geojson`;
    }
  }
  let geojson = await fetchGeoJSON(url);
  if (!geojson) {
    const fallbackURL = OVERLAY_FALLBACK_URLS[def.id];
    geojson = fallbackURL ? await fetchGeoJSON(fallbackURL) : null;
  }
  if (!geojson) {
    geojson = EMPTY_FEATURE_COLLECTION;
  }
  geojson = filterConflictOverlayFeatures(geojson, def.id, conflictLensId);
  geojson = filterGeoJSONByRegion(geojson, def.id, regionFilter, conflictLensId);
  const group = L.layerGroup();

  if (def.id === "conflicts") {
    L.geoJSON(geojson as any, {
      style: (feature) => {
        const type = feature?.properties?.type ?? "active_conflict";
        const countryRole = feature?.properties?.country_role ?? "primary";
        const isContext = countryRole === "context";
        return {
          color: conflictTypeColors[type] ?? "#ff5d5d",
          fillColor: conflictTypeColors[type] ?? "#ff5d5d",
          fillOpacity: isContext ? 0.04 : 0.12,
          weight: isContext ? 1 : 1.5,
          dashArray: isContext ? "4,3" : (type === "frozen_conflict" || type === "high_tension" ? "6,4" : undefined),
        };
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        const role = p.country_role === "context" ? "involved country" : "primary conflict country";
        const countryCode = pickString(p, "country_code", "countryCode", "ISO_A2", "iso2").toUpperCase();
        const countryLabel = pickString(p, "country_label", "country_label", "name", "NAME", "ADMIN", "country");
        layer.bindTooltip(
          `<strong>${p.name}</strong><br/>${p.type?.replace(/_/g, " ")} since ${p.since}<br/>${role}`,
          { className: "siem-tooltip", direction: "top" },
        );
        if (onConflictCountrySelect && countryCode) {
          layer.on("click", () => {
            const lensId = pickString(p, "lens_id", "lensId").toLowerCase() || undefined;
            onConflictCountrySelect(countryCode, countryLabel || countryCode, lensId);
          });
        }
      },
    }).addTo(group);
  } else if (def.id === "ports") {
    L.geoJSON(geojson as any, {
      pointToLayer: (_feature, latlng) => {
        const type = _feature.properties?.type ?? "container";
        return L.circleMarker(latlng, {
          radius: type === "military" ? 5 : 4,
          fillColor: def.color,
          color: `${def.color}99`,
          weight: 1,
          fillOpacity: 0.8,
        });
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        const icon = portTypeIcons[p.type] ?? "\u2693";
        layer.bindTooltip(
          `${icon} <strong>${p.name}</strong> (${p.country})<br/>${p.type}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "bases") {
    const features = Array.isArray((geojson as { features?: unknown[] }).features)
      ? ((geojson as { features: Array<{ geometry?: { type?: string; coordinates?: unknown }; properties?: Record<string, unknown> }> }).features)
      : [];
    for (const feature of features) {
      const latlng = geometryCenterLatLng(feature.geometry);
      if (!latlng) continue;
      const props = feature.properties ?? {};
      const name = pickString(props, "name", "featureName", "siteName") || "Military Base";
      const country = pickString(props, "country", "countryName", "stateNameCode") || "US";
      const operator = pickString(props, "operator", "siteReportingComponent") || "DoD";
      const description = pickString(props, "featureDescription");
      const component = pickString(props, "siteReportingComponent");
      const isJoint = props.isJointBase === true || props.isJointBase === 1 || props.isJointBase === "1";
      const type = pickString(props, "type") || inferBaseType(name, description, component, isJoint);
      const icon = baseTypeIcons[type] ?? "\u2605";

      const marker = L.circleMarker(latlng, {
        radius: 4,
        fillColor: def.color,
        color: `${def.color}99`,
        weight: 1,
        fillOpacity: 0.85,
      });
      marker.bindTooltip(
        `${icon} <strong>${name}</strong> (${country})<br/>${operator} — ${type}`,
        { className: "siem-tooltip", direction: "top" },
      );
      marker.addTo(group);
    }
  } else if (def.id === "nuclear") {
    L.geoJSON(geojson as any, {
      pointToLayer: (_feature, latlng) => {
        return L.circleMarker(latlng, {
          radius: 5,
          fillColor: def.color,
          color: `${def.color}cc`,
          weight: 1.5,
          fillOpacity: 0.9,
        });
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        const statusTag = p.status ? ` [${p.status}]` : "";
        const cap = p.capacity ? `<br/>${p.capacity}` : "";
        layer.bindTooltip(
          `\u2622 <strong>${p.name}</strong> (${p.country})<br/>${p.type?.replace(/_/g, " ")}${statusTag}${cap}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "sanctions") {
    L.geoJSON(geojson as any, {
      style: (feature) => {
        const type = feature?.properties?.type ?? "comprehensive";
        return {
          color: sanctionTypeColors[type] ?? "#f87171",
          fillColor: sanctionTypeColors[type] ?? "#f87171",
          fillOpacity: 0.08,
          weight: 1.5,
          dashArray: type === "targeted" ? "6,4" : undefined,
        };
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        layer.bindTooltip(
          `<strong>${p.name}</strong><br/>${p.regime} — ${p.type} since ${p.since}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "piracy") {
    L.geoJSON(geojson as any, {
      style: (feature) => {
        const type = feature?.properties?.type ?? "elevated";
        return {
          color: piracyTypeColors[type] ?? "#38bdf8",
          fillColor: piracyTypeColors[type] ?? "#38bdf8",
          fillOpacity: 0.10,
          weight: 1.5,
          dashArray: type === "moderate" ? "6,4" : undefined,
        };
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        layer.bindTooltip(
          `\u2620 <strong>${p.name}</strong><br/>${p.type?.replace(/_/g, " ")} — ${p.threat}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "terrorism") {
    L.geoJSON(geojson as any, {
      style: (feature) => {
        const type = feature?.properties?.type ?? "active";
        return {
          color: terrorismTypeColors[type] ?? "#e879f9",
          fillColor: terrorismTypeColors[type] ?? "#e879f9",
          fillOpacity: 0.10,
          weight: 1.5,
          dashArray: type === "degraded" ? "6,4" : undefined,
        };
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        layer.bindTooltip(
          `\u26A0 <strong>${p.name}</strong><br/>${p.type} — ${p.threat}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "cables") {
    L.geoJSON(geojson as any, {
      style: (feature) => ({
        color: feature?.properties?.color ?? def.color,
        weight: 1.5,
        opacity: 0.5,
        dashArray: "4,3",
      }),
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        layer.bindTooltip(
          `<strong>${p.name}</strong>`,
          { className: "siem-tooltip", sticky: true },
        );
      },
    }).addTo(group);
  } else {
    // Shipping lanes (Major / Middle)
    const laneWeight: Record<string, number> = { Major: 2, Middle: 1.4 };
    const laneOpacity: Record<string, number> = { Major: 0.5, Middle: 0.35 };
    L.geoJSON(geojson as any, {
      style: (feature) => {
        const type = feature?.properties?.Type ?? "Major";
        return {
          color: def.color,
          weight: laneWeight[type] ?? 1.4,
          opacity: laneOpacity[type] ?? 0.35,
          dashArray: "8,5",
        };
      },
      onEachFeature: (feature, layer) => {
        const type = feature?.properties?.Type ?? "Shipping Lane";
        layer.bindTooltip(
          `<strong>${type} shipping lane</strong>`,
          { className: "siem-tooltip", sticky: true },
        );
      },
    }).addTo(group);
  }

  group.addTo(map);
  return group;
}

function normalizedStatus(value: unknown): string {
  if (typeof value !== "string") return "";
  return value.trim().toLowerCase();
}

function normalizedLensID(value: unknown): string {
  if (typeof value !== "string") return "";
  return value.trim().toLowerCase();
}

function filterConflictOverlayFeatures(geojson: GeoJSONLike, overlayID: string, conflictLensId: string): GeoJSONLike {
  if (overlayID !== "conflicts" && overlayID !== "terrorism") return geojson;
  const features = (geojson as { features?: unknown[] }).features;
  if (!Array.isArray(features)) return geojson;

  const wantedLensID = conflictLensId.toLowerCase();
  const filtered = features.filter((feature) => {
    const props = (feature as GeoJSONFeature)?.properties ?? {};
    const featureLensID = normalizedLensID(props.lens_id ?? props.lensId);
    const status = normalizedStatus(props.status);
    if (wantedLensID !== "") {
      return featureLensID === wantedLensID;
    }
    return status !== "inactive";
  });
  return { ...geojson, features: filtered };
}

function isRegionScopedOverlay(id: string): boolean {
  return id === "conflicts" || id === "terrorism" || id === "sanctions" || id === "piracy" || id === "nuclear" || id === "bases";
}

function normalizeCountryCode(value: unknown): string {
  if (typeof value !== "string") return "";
  return value.trim().toUpperCase();
}

function featureCountryCode(feature: GeoJSONFeatureItem): string {
  const p = feature.properties ?? {};
  return (
    normalizeCountryCode(p.country_code) ||
    normalizeCountryCode(p.countryCode) ||
    normalizeCountryCode(p.ISO_A2) ||
    normalizeCountryCode(p.iso2)
  );
}

function featureRegion(feature: GeoJSONFeatureItem, overlayID: string): string | null {
  const p = feature.properties ?? {};
  if (overlayID === "conflicts" || overlayID === "terrorism") {
    const lensID = normalizedLensID(p.lens_id ?? p.lensId);
    if (lensID !== "" && LENS_REGION_FILTERS[lensID]) {
      return LENS_REGION_FILTERS[lensID];
    }
  }
  const center = geometryCenterLatLng(feature.geometry);
  if (!center) return null;
  return latLngToRegion(center.lat, center.lng);
}

function filterGeoJSONByRegion(geojson: GeoJSONLike, overlayID: string, regionFilter: string, conflictLensId: string): GeoJSONLike {
  if (!isRegionScopedOverlay(overlayID)) return geojson;
  if (conflictLensId !== "") return geojson;
  if (regionFilter === "all") return geojson;
  const features = (geojson as { features?: unknown[] }).features;
  if (!Array.isArray(features)) return geojson;

  const filtered = features.filter((raw) => {
    const feature = raw as GeoJSONFeatureItem;
    if (regionFilter.startsWith("country:")) {
      const selectedCode = regionFilter.slice(8).toUpperCase();
      return featureCountryCode(feature) === selectedCode;
    }
    const region = featureRegion(feature, overlayID);
    return region === regionFilter;
  });
  return { ...geojson, features: filtered };
}
