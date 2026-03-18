import L from "leaflet";

export type OverlayId = "cables" | "shipping" | "ports" | "conflicts" | "bases" | "nuclear" | "sanctions" | "piracy" | "terrorism";

export interface OverlayDef {
  id: OverlayId;
  label: string;
  color: string;
  url: string;
}

export const OVERLAYS: OverlayDef[] = [
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
): Promise<L.LayerGroup> {
  const resp = await fetch(def.url);
  const geojson = await resp.json();
  const group = L.layerGroup();

  if (def.id === "conflicts") {
    L.geoJSON(geojson, {
      style: (feature) => {
        const type = feature?.properties?.type ?? "active_conflict";
        return {
          color: conflictTypeColors[type] ?? "#ff5d5d",
          fillColor: conflictTypeColors[type] ?? "#ff5d5d",
          fillOpacity: 0.12,
          weight: 1.5,
          dashArray: type === "frozen_conflict" || type === "high_tension" ? "6,4" : undefined,
        };
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        layer.bindTooltip(
          `<strong>${p.name}</strong><br/>${p.type?.replace(/_/g, " ")} since ${p.since}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "ports") {
    L.geoJSON(geojson, {
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
    L.geoJSON(geojson, {
      pointToLayer: (_feature, latlng) => {
        return L.circleMarker(latlng, {
          radius: 4,
          fillColor: def.color,
          color: `${def.color}99`,
          weight: 1,
          fillOpacity: 0.85,
        });
      },
      onEachFeature: (feature, layer) => {
        const p = feature.properties;
        const icon = baseTypeIcons[p.type] ?? "\u2605";
        layer.bindTooltip(
          `${icon} <strong>${p.name}</strong> (${p.country})<br/>${p.operator} — ${p.type}`,
          { className: "siem-tooltip", direction: "top" },
        );
      },
    }).addTo(group);
  } else if (def.id === "nuclear") {
    L.geoJSON(geojson, {
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
    L.geoJSON(geojson, {
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
    L.geoJSON(geojson, {
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
    L.geoJSON(geojson, {
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
    L.geoJSON(geojson, {
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
    L.geoJSON(geojson, {
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
