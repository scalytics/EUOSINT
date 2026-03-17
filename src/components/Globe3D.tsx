/*
 * EUOSINT
 * 3D Globe view using globe.gl with Natural Earth textures.
 * Supports the same overlay layers as the 2D Leaflet map:
 *   - Polygons: conflict zones, sanctions
 *   - Paths: undersea cables, shipping lanes
 *   - HTML markers: military bases, nuclear sites, strategic ports
 *   - Clustered alert points via supercluster
 */

import { useEffect, useRef, useMemo, useCallback } from "react";
import Globe, { type GlobeInstance } from "globe.gl";
import Supercluster from "supercluster";
import type { Alert, Severity } from "@/types/alert";
import { severityHex } from "@/lib/theme";
import { OVERLAYS, type OverlayId } from "@/lib/map-overlays";

const REGION_POV: Record<string, { lat: number; lng: number; altitude: number }> = {
  Europe: { lat: 50, lng: 10, altitude: 1.8 },
  "North America": { lat: 42, lng: -100, altitude: 2.0 },
  "South America": { lat: -15, lng: -60, altitude: 2.2 },
  Africa: { lat: 2, lng: 20, altitude: 2.2 },
  Asia: { lat: 34, lng: 100, altitude: 2.2 },
  Oceania: { lat: -28, lng: 140, altitude: 2.0 },
  Caribbean: { lat: 18, lng: -75, altitude: 1.5 },
  International: { lat: 20, lng: 0, altitude: 2.5 },
  all: { lat: 20, lng: 0, altitude: 2.5 },
};

/* Approximate mapping from globe.gl altitude → Leaflet-style zoom level.
 * globe.gl altitude is distance from globe surface in Earth-radii units.
 * Lower altitude = closer = higher zoom.  */
function altitudeToZoom(altitude: number): number {
  // Empirically: alt 3.0 ≈ zoom 2, alt 1.8 ≈ zoom 3, alt 0.8 ≈ zoom 5, alt 0.3 ≈ zoom 7
  return Math.round(Math.max(1, Math.min(14, 6.5 - Math.log2(altitude + 0.1))));
}

interface Props {
  alerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  regionFilter: string;
  activeOverlays: Set<OverlayId>;
}

interface PolygonFeature {
  geometry: { type: string; coordinates: unknown };
  properties: Record<string, string>;
  __color: string;
  __label: string;
}

interface PathFeature {
  coords: [number, number][];
  __color: string;
  __label: string;
}

interface HtmlMarker {
  lat: number;
  lng: number;
  label: string;
  color: string;
  icon: string;
  size: number;
}

interface ClusterPoint {
  lat: number;
  lng: number;
  color: string;
  label: string;
  count: number;
  isCluster: boolean;
  id: string;
  radius: number;
  altitude: number;
}

type HtmlElementDatum = HtmlMarker | ClusterPoint;

type ClusterFeature =
  | {
      geometry: { coordinates: [number, number] };
      properties: { cluster: true; point_count: number; cluster_id: number };
    }
  | {
      geometry: { coordinates: [number, number] };
      properties: { id: string; severity: Severity; title: string; text: string };
    };

// Overlay colors matching map-overlays.ts
const overlayColors: Record<string, string> = {
  conflicts: "#ff5d5d",
  cables: "#60a5fa",
  shipping: "#4ccb8d",
  sanctions: "#f87171",
  ports: "#f29d4b",
  bases: "#a78bfa",
  nuclear: "#facc15",
};

// Icons for point-based overlays
const baseTypeIcons: Record<string, string> = {
  air: "\u2708",
  naval: "\u2693",
  army: "\u2694",
  joint: "\u2605",
  intelligence: "\u25C9",
};
const portTypeIcons: Record<string, string> = {
  container: "\u2693",
  canal: "\u26F5",
  military: "\u2694",
  strategic: "\u25C6",
};

function buildMarkerEl(marker: HtmlMarker): HTMLDivElement {
  const el = document.createElement("div");
  el.style.cssText = `
    width:${marker.size}px;height:${marker.size}px;border-radius:50%;
    background:${marker.color};border:1.5px solid rgba(255,255,255,0.5);
    display:flex;align-items:center;justify-content:center;
    font-size:${marker.size * 0.55}px;line-height:1;cursor:pointer;
    box-shadow:0 0 6px ${marker.color}88;pointer-events:auto;
  `;
  el.textContent = marker.icon;
  el.title = marker.label;
  return el;
}

function buildClusterEl(point: ClusterPoint): HTMLDivElement {
  const el = document.createElement("div");
  if (point.isCluster) {
    const size = point.count >= 100 ? 36 : point.count >= 30 ? 30 : 24;
    const bg = point.count >= 100
      ? "rgba(239,68,68,0.28)"
      : point.count >= 30
        ? "rgba(245,158,11,0.22)"
        : "rgba(15,18,25,0.88)";
    const border = point.count >= 100
      ? "rgba(239,68,68,0.55)"
      : "rgba(245,158,11,0.45)";
    el.style.cssText = `
      width:${size}px;height:${size}px;border-radius:50%;
      background:${bg};border:2px solid ${border};
      display:flex;align-items:center;justify-content:center;
      font-family:'Roboto Mono',monospace;font-weight:600;
      font-size:${size * 0.35}px;color:#e6edf5;line-height:1;
      cursor:pointer;pointer-events:auto;
      box-shadow:0 0 8px rgba(0,0,0,0.4);
    `;
    el.textContent = String(point.count);
    el.title = `${point.count} alerts`;
  } else {
    const size = point.id === "__selected" ? 14 : 8;
    el.style.cssText = `
      width:${size}px;height:${size}px;border-radius:50%;
      background:${point.color};
      border:1.5px solid rgba(255,255,255,0.45);
      cursor:pointer;pointer-events:auto;
      box-shadow:0 0 4px ${point.color}88;
    `;
    el.title = point.label;
  }
  return el;
}

const TOOLTIP_STYLE = "background:rgba(15,18,25,0.92);border:1px solid rgba(255,255,255,0.12);border-radius:8px;padding:6px 10px;font-size:11px;color:#e2e8f0;max-width:220px";

// Min / max camera distances (Earth radii). Prevents scrolling into oblivion.
const MIN_ALTITUDE = 0.25; // ~ zoom 8, close enough to see individual points
const MAX_ALTITUDE = 4.5;  // ~ zoom 1, sees the whole globe

export function Globe3D({ alerts, selectedId, onSelect, regionFilter, activeOverlays }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const globeRef = useRef<GlobeInstance | null>(null);
  const onSelectRef = useRef(onSelect);
  onSelectRef.current = onSelect;
  const altitudeRef = useRef(2.5);

  // Build supercluster index from alerts
  const clusterIndex = useMemo(() => {
    const index = new Supercluster({
      radius: 45,
      maxZoom: 14,
      minZoom: 0,
    });
    const features = alerts.map((a) => ({
      type: "Feature" as const,
      properties: {
        id: a.alert_id,
        severity: a.severity,
        title: a.source.authority_name,
        text: a.title.slice(0, 80),
      },
      geometry: {
        type: "Point" as const,
        coordinates: [a.lng, a.lat] as [number, number],
      },
    }));
    index.load(features);
    return index;
  }, [alerts]);

  const isOverlayMarker = (d: unknown): d is HtmlMarker =>
    typeof d === "object" &&
    d !== null &&
    "icon" in d &&
    "size" in d &&
    !("isCluster" in d);

  // Compute clustered points for the current altitude
  const computeClusteredPoints = useCallback(
    (altitude: number): ClusterPoint[] => {
      const zoom = altitudeToZoom(altitude);
      const clusters = clusterIndex.getClusters([-180, -85, 180, 85], zoom) as unknown as ClusterFeature[];
      return clusters.map((c: ClusterFeature) => {
        const [lng, lat] = c.geometry.coordinates;
        if ("cluster" in c.properties) {
          const count = c.properties.point_count;
          return {
            lat,
            lng,
            color: "#e6edf5",
            label: `${count} alerts`,
            count,
            isCluster: true,
            id: `cluster-${c.properties.cluster_id}`,
            radius: 0.3,
            altitude: 0.015,
          };
        }
        const props = c.properties;
        const selected = props.id === selectedId;
        return {
          lat,
          lng,
          color: severityHex(props.severity),
          label: `${props.title}\n${props.text}`,
          count: 1,
          isCluster: false,
          id: selected ? "__selected" : props.id,
          radius: selected ? 0.35 : 0.18,
          altitude: selected ? 0.025 : 0.01,
        };
      });
    },
    [clusterIndex, selectedId],
  );

  const handleClusterClick = useCallback(
    (point: object) => {
      const p = point as ClusterPoint;
      if (p.isCluster) {
        // Zoom into the cluster
        const globe = globeRef.current;
        if (globe) {
          const newAlt = Math.max(MIN_ALTITUDE, altitudeRef.current * 0.5);
          globe.pointOfView({ lat: p.lat, lng: p.lng, altitude: newAlt }, 600);
        }
      } else {
        const id = p.id === "__selected" ? selectedId : p.id;
        if (id) onSelectRef.current(id);
      }
    },
    [selectedId],
  );

  /* Initialise globe once */
  useEffect(() => {
    if (!containerRef.current) return;

    const base = import.meta.env.BASE_URL ?? "/";
    const globe = new Globe(containerRef.current)
      .globeImageUrl(`${base}textures/earth-topo-bathy.jpg`)
      .backgroundImageUrl(`${base}textures/night-sky.png`)
      .showAtmosphere(true)
      .atmosphereColor("#4466cc")
      .atmosphereAltitude(0.18)
      // Alert points rendered via HTML elements (clustered)
      .pointsData([])
      .showGraticules(true)
      // Polygon layer (conflict zones, sanctions)
      .polygonsData([])
      .polygonCapColor((d: object) => (d as PolygonFeature).__color + "22")
      .polygonSideColor((d: object) => (d as PolygonFeature).__color + "11")
      .polygonStrokeColor((d: object) => (d as PolygonFeature).__color)
      .polygonAltitude(0.005)
      .polygonLabel((d: object) => {
        const f = d as PolygonFeature;
        return `<div style="${TOOLTIP_STYLE}">${f.__label}</div>`;
      })
      // Path layer (cables, shipping lanes)
      .pathsData([])
      .pathColor((d: object) => (d as PathFeature).__color)
      .pathStroke(1.5)
      .pathLabel((d: object) => {
        const f = d as PathFeature;
        return `<div style="${TOOLTIP_STYLE}">${f.__label}</div>`;
      })
      // HTML marker layer (bases, nuclear, ports + cluster markers)
      .htmlElementsData([])
      .htmlLat((d: object) => (d as { lat: number }).lat)
      .htmlLng((d: object) => (d as { lng: number }).lng)
      .htmlAltitude((d: object) => {
        const m = d as { altitude?: number };
        return m.altitude ?? 0.012;
      })
      .htmlElement((d: object) => {
        if ("isCluster" in (d as object)) {
          return buildClusterEl(d as ClusterPoint);
        }
        return buildMarkerEl(d as HtmlMarker);
      });

    // Dim scene lights for dark theme
    const scene = globe.scene();
    scene.children.forEach((child: { type?: string; intensity?: number }) => {
      if (child.type === "DirectionalLight") child.intensity = 0.6;
      if (child.type === "AmbientLight") child.intensity = 0.8;
    });

    // Constrain camera zoom range — prevent scrolling into oblivion
    const controls = globe.controls() as {
      minDistance?: number;
      maxDistance?: number;
      addEventListener?: (event: string, cb: () => void) => void;
    };
    // globe.gl uses Three.js units where Earth radius = 100
    controls.minDistance = 100 + MIN_ALTITUDE * 100; // closest
    controls.maxDistance = 100 + MAX_ALTITUDE * 100; // farthest

    // Track altitude changes for cluster re-computation
    if (controls.addEventListener) {
      controls.addEventListener("change", () => {
        const pov = globe.pointOfView();
        if (Math.abs(pov.altitude - altitudeRef.current) > 0.1) {
          altitudeRef.current = pov.altitude;
          // Re-render will be triggered by the effect watching altitudeRef
        }
      });
    }

    globeRef.current = globe;

    const ro = new ResizeObserver(() => {
      if (!containerRef.current) return;
      const { width, height } = containerRef.current.getBoundingClientRect();
      globe.width(width);
      globe.height(height);
    });
    ro.observe(containerRef.current);

    return () => {
      ro.disconnect();
      globe._destructor();
      globeRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /* Sync clustered alert points */
  useEffect(() => {
    const globe = globeRef.current;
    if (!globe) return;

    const clusterPoints = computeClusteredPoints(altitudeRef.current);

    // Merge cluster points with overlay HTML markers
    const existing = (globe.htmlElementsData() as unknown[]).filter(isOverlayMarker);
    globe.htmlElementsData([...existing, ...clusterPoints]);
  }, [computeClusteredPoints]);

  /* Re-cluster on zoom change — poll altitude via animation frame */
  useEffect(() => {
    const globe = globeRef.current;
    if (!globe) return;
    let lastZoom = altitudeToZoom(altitudeRef.current);
    let raf: number;
    const tick = () => {
      const pov = globe.pointOfView();
      altitudeRef.current = pov.altitude;
      const zoom = altitudeToZoom(pov.altitude);
      if (zoom !== lastZoom) {
        lastZoom = zoom;
        const clusterPoints = computeClusteredPoints(pov.altitude);
        // Keep overlay markers, replace cluster points
        const overlayMarkers = (globe.htmlElementsData() as unknown[]).filter(isOverlayMarker);
        globe.htmlElementsData([...overlayMarkers, ...clusterPoints]);
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [computeClusteredPoints]);

  /* Sync overlays */
  useEffect(() => {
    const globe = globeRef.current;
    if (!globe) return;

    const polygons: PolygonFeature[] = [];
    const paths: PathFeature[] = [];
    const htmlMarkers: HtmlMarker[] = [];

    const fetches = Array.from(activeOverlays).map(async (id) => {
      const def = OVERLAYS.find((o) => o.id === id);
      if (!def) return;
      try {
        const resp = await fetch(def.url);
        const geojson = await resp.json();
        const color = overlayColors[id] ?? def.color;

        for (const feature of geojson.features) {
          const gtype = feature.geometry?.type;
          const props = feature.properties ?? {};
          const label = `<strong>${props.name ?? ""}</strong>${props.type ? `<br/>${props.type}` : ""}`;

          if (gtype === "Polygon" || gtype === "MultiPolygon") {
            polygons.push({
              geometry: feature.geometry,
              properties: props,
              __color: color,
              __label: label,
            });
          } else if (gtype === "LineString") {
            paths.push({
              coords: feature.geometry.coordinates.map((c: number[]) => [c[1], c[0]]),
              __color: color,
              __label: label,
            });
          } else if (gtype === "Point") {
            const [lng, lat] = feature.geometry.coordinates as [number, number];
            let icon = "\u25CF";
            let size = 10;
            if (id === "bases") {
              icon = baseTypeIcons[props.type] ?? "\u2605";
              size = 12;
            } else if (id === "nuclear") {
              icon = "\u2622";
              size = 13;
            } else if (id === "ports") {
              icon = portTypeIcons[props.type] ?? "\u2693";
              size = 11;
            }
            htmlMarkers.push({ lat, lng, label: props.name ?? "", color, icon, size });
          }
        }
      } catch {
        // overlay fetch failed silently
      }
    });

    Promise.all(fetches).then(() => {
      globe
        .polygonsData(polygons)
        .polygonGeoJsonGeometry("geometry")
        .pathsData(paths)
        .pathPoints("coords")
        .pathPointLat((p: unknown) => (p as number[])[0])
        .pathPointLng((p: unknown) => (p as number[])[1]);

      // Merge overlay markers with current cluster points
      const clusterPoints = computeClusteredPoints(altitudeRef.current);
      globe.htmlElementsData([...htmlMarkers, ...clusterPoints]);
    });
  }, [activeOverlays, computeClusteredPoints]);

  /* Fly to region */
  useEffect(() => {
    const globe = globeRef.current;
    if (!globe) return;

    if (regionFilter.startsWith("country:") && alerts.length > 0) {
      const avgLat = alerts.reduce((s, a) => s + a.lat, 0) / alerts.length;
      const avgLng = alerts.reduce((s, a) => s + a.lng, 0) / alerts.length;
      globe.pointOfView({ lat: avgLat, lng: avgLng, altitude: 1.5 }, 800);
      return;
    }

    const pov = REGION_POV[regionFilter] ?? REGION_POV.Europe;
    globe.pointOfView(pov, 800);
  }, [regionFilter, alerts]);

  return (
    <div
      ref={containerRef}
      className="absolute inset-0 overflow-hidden rounded-[1.4rem] border border-siem-border"
    />
  );
}
