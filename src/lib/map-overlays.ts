import L from "leaflet";
import type { GeoJsonObject } from "geojson";
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
  gaza: "Middle East",
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
  "Middle East": "/geo/military-bases.asia.geojson",
  "Asia-Pacific": "/geo/military-bases.asia.geojson",
  all: "/geo/military-bases.all.geojson",
};

function normalizeOverlayRegion(regionFilter: string): string {
  if (BASES_REGION_URLS[regionFilter]) return regionFilter;
  const value = regionFilter.trim().toLowerCase();
  if (value.includes("europe")) return "Europe";
  if (value.includes("africa")) return "Africa";
  if (value.includes("middle east")) return "Middle East";
  if (value.includes("asia") || value.includes("oceania")) return "Asia-Pacific";
  if (value.includes("north america") || value.includes("caribbean")) return "North America";
  if (value.includes("south america") || value.includes("latin america")) return "all";
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
  { id: "views-risk", label: "Conflict Risk", color: "#06b6d4", url: "/views-risk.json" },
];

const EMPTY_FEATURE_COLLECTION: GeoJSONLike = { type: "FeatureCollection", features: [] };

function toGeoJsonObject(value: GeoJSONLike): GeoJsonObject {
  return value as unknown as GeoJsonObject;
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
  const leafletGeoJSON = toGeoJsonObject(geojson);
  const group = L.layerGroup();

  if (def.id === "conflicts") {
    L.geoJSON(leafletGeoJSON, {
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
    L.geoJSON(leafletGeoJSON, {
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
    L.geoJSON(leafletGeoJSON, {
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
    L.geoJSON(leafletGeoJSON, {
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
    L.geoJSON(leafletGeoJSON, {
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
  } else if (def.id === "views-risk") {
    // VIEWS/PRIO conflict risk layer — country markers + grid heatmap
    const viewsGroup = L.layerGroup();
    try {
      const [countryResp, gridResp] = await Promise.all([
        fetch("/views-risk.json").then((r) => r.ok ? r.json() : null).catch(() => null),
        fetch("/geo/views-risk-grid.json").then((r) => r.ok ? r.json() : null).catch(() => null),
      ]);

      // Grid-level heatmap circles (sub-country detail)
      if (gridResp?.cells && Array.isArray(gridResp.cells)) {
        for (const cell of gridResp.cells as Array<{ lat: number; lng: number; sb_mean: number; ns_mean: number; os_mean: number; sb_dich: number; ns_dich: number; os_dich: number }>) {
          const total = cell.sb_mean + cell.ns_mean + cell.os_mean;
          if (total < 0.5) continue;
          const intensity = Math.min(1, total / 50);
          const radius = 4 + intensity * 8;
          const color = total > 20 ? "#ef4444" : total > 5 ? "#f97316" : total > 1 ? "#eab308" : "#06b6d4";
          const marker = L.circleMarker([cell.lat, cell.lng], {
            radius,
            fillColor: color,
            color: `${color}66`,
            weight: 0.5,
            fillOpacity: 0.25 + intensity * 0.3,
          });
          marker.bindTooltip(
            `<div style="min-width:160px"><strong>\u26A0 VIEWS Grid Risk</strong><br/>`
            + `<span style="color:#94a3b8">State-based:</span> ${cell.sb_mean.toFixed(1)} fatalities/mo (${Math.round(cell.sb_dich * 100)}%)<br/>`
            + `<span style="color:#94a3b8">Non-state:</span> ${cell.ns_mean.toFixed(1)} fatalities/mo (${Math.round(cell.ns_dich * 100)}%)<br/>`
            + `<span style="color:#94a3b8">One-sided:</span> ${cell.os_mean.toFixed(1)} fatalities/mo (${Math.round(cell.os_dich * 100)}%)<br/>`
            + `<span style="color:#94a3b8">Combined:</span> <strong>${total.toFixed(1)}</strong> fatalities/mo<br/>`
            + `<span style="color:#64748b;font-size:9px">PRIO/VIEWS Early Warning System</span></div>`,
            { className: "siem-tooltip", direction: "top" },
          );
          marker.addTo(viewsGroup);
        }
      }

      // Country-level markers (larger, labeled)
      if (countryResp?.countries && Array.isArray(countryResp.countries)) {
        const countryCenters: Record<string, [number, number]> = {
          UKR: [48.4, 31.2], SDN: [15.5, 32.5], SOM: [5.2, 46.2], PAK: [30.4, 69.3],
          ETH: [9.1, 40.5], MMR: [19.8, 96.2], BFA: [12.3, -1.6], ISR: [31.0, 34.9],
          SYR: [35.0, 38.5], NGA: [9.1, 7.5], YEM: [15.6, 48.5], COD: [-4.0, 21.8],
          MLI: [17.6, -4.0], RUS: [55.8, 37.6], NER: [17.6, 8.1], AFG: [33.9, 67.7],
          MOZ: [-18.7, 35.5], IRQ: [33.2, 43.7], TCD: [15.5, 18.7], CMR: [7.4, 12.4],
          COL: [4.6, -74.3], IND: [20.6, 79.0], TUR: [39.9, 32.9], EGY: [26.8, 30.8],
          PSE: [31.9, 35.2], LBY: [26.3, 17.2], SSD: [6.9, 31.3], KEN: [0.0, 38.0],
          PHL: [12.9, 121.8], THA: [15.9, 100.9], MEX: [23.6, -102.6], BRA: [-14.2, -51.9],
          ZAF: [-30.6, 22.9], SAU: [23.9, 45.1], IRN: [32.4, 53.7], LBN: [33.9, 35.5],
          MYS: [4.2, 101.9], IDN: [-0.8, 113.9], TZA: [-6.4, 34.9], UGA: [-1.4, 32.3],
          HTI: [19.0, -72.1], CAF: [6.6, 20.9], GIN: [9.9, -12.0], SEN: [14.5, -14.5],
          CIV: [7.5, -5.5], GHA: [7.9, -1.0], ZWE: [-19.0, 29.2], AGO: [-11.2, 17.9],
        };
        for (const c of countryResp.countries as Array<{ iso: string; name: string; sb_mean: number; ns_mean: number; os_mean: number; sb_dich: number; ns_dich: number; os_dich: number }>) {
          const total = c.sb_mean + c.ns_mean + c.os_mean;
          if (total < 1) continue;
          const center = countryCenters[c.iso];
          if (!center) continue;
          const color = total > 100 ? "#dc2626" : total > 50 ? "#ef4444" : total > 10 ? "#f97316" : total > 5 ? "#eab308" : "#06b6d4";
          const radius = total > 100 ? 14 : total > 50 ? 11 : total > 10 ? 9 : 7;
          const marker = L.circleMarker(center, {
            radius,
            fillColor: color,
            color: "#fff",
            weight: 1.5,
            fillOpacity: 0.85,
          });
          marker.bindPopup(
            `<div style="min-width:200px;font-family:ui-monospace,monospace;font-size:11px;line-height:1.6">`
            + `<div style="font-size:13px;font-weight:700;margin-bottom:6px">\u26A0 ${c.name} (${c.iso})</div>`
            + `<table style="width:100%;border-collapse:collapse">`
            + `<tr style="border-bottom:1px solid #334155"><td style="color:#94a3b8;padding:2px 8px 2px 0">State-based</td><td style="text-align:right">${c.sb_mean.toFixed(1)}/mo</td><td style="text-align:right;color:#64748b">${Math.round(c.sb_dich * 100)}%</td></tr>`
            + `<tr style="border-bottom:1px solid #334155"><td style="color:#94a3b8;padding:2px 8px 2px 0">Non-state</td><td style="text-align:right">${c.ns_mean.toFixed(1)}/mo</td><td style="text-align:right;color:#64748b">${Math.round(c.ns_dich * 100)}%</td></tr>`
            + `<tr style="border-bottom:1px solid #334155"><td style="color:#94a3b8;padding:2px 8px 2px 0">One-sided</td><td style="text-align:right">${c.os_mean.toFixed(1)}/mo</td><td style="text-align:right;color:#64748b">${Math.round(c.os_dich * 100)}%</td></tr>`
            + `<tr><td style="color:#f8fafc;font-weight:700;padding:4px 8px 2px 0">Combined</td><td style="text-align:right;font-weight:700;color:#f8fafc">${total.toFixed(1)}/mo</td><td></td></tr>`
            + `</table>`
            + `<div style="margin-top:8px;color:#64748b;font-size:9px">Predicted fatalities/month &middot; PRIO/VIEWS Early Warning System<br/>Run: ${countryResp.run ?? "unknown"}</div>`
            + `</div>`,
            { className: "siem-popup", maxWidth: 280 },
          );
          marker.addTo(viewsGroup);
        }
      }
    } catch {
      // Non-fatal — layer just shows empty
    }
    viewsGroup.addTo(group);
  } else if (def.id === "terrorism") {
    L.geoJSON(leafletGeoJSON, {
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
    L.geoJSON(leafletGeoJSON, {
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
    L.geoJSON(leafletGeoJSON, {
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
