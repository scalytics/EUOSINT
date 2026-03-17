/*
 * EUOSINT
 * 3D Globe view using globe.gl with Natural Earth textures.
 * Supports the same overlay layers as the 2D Leaflet map.
 */

import { useEffect, useRef, useMemo, useCallback } from "react";
import Globe, { type GlobeInstance } from "globe.gl";
import type { Alert } from "@/types/alert";
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

// Colors matching map-overlays.ts
const overlayColors: Record<string, string> = {
  conflicts: "#ff5d5d",
  cables: "#60a5fa",
  shipping: "#4ccb8d",
  sanctions: "#f87171",
};

export function Globe3D({ alerts, selectedId, onSelect, regionFilter, activeOverlays }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const globeRef = useRef<GlobeInstance | null>(null);
  const onSelectRef = useRef(onSelect);
  onSelectRef.current = onSelect;

  const points = useMemo(
    () =>
      alerts.map((a) => ({
        id: a.alert_id,
        lat: a.lat,
        lng: a.lng,
        severity: a.severity,
        color: severityHex(a.severity),
        title: a.source.authority_name,
        text: a.title.slice(0, 80),
        selected: a.alert_id === selectedId,
      })),
    [alerts, selectedId],
  );

  const handlePointClick = useCallback(
    (point: object) => {
      const p = point as { id: string };
      onSelectRef.current(p.id);
    },
    [],
  );

  /* Initialise globe once */
  useEffect(() => {
    if (!containerRef.current) return;

    const globe = new Globe(containerRef.current)
      .globeImageUrl("//unpkg.com/three-globe/example/img/earth-night.jpg")
      .bumpImageUrl("//unpkg.com/three-globe/example/img/earth-topology.png")
      .backgroundImageUrl("//unpkg.com/three-globe/example/img/night-sky.png")
      .showAtmosphere(true)
      .atmosphereColor("#3a7ecf")
      .atmosphereAltitude(0.15)
      .pointsData([])
      .pointLat("lat")
      .pointLng("lng")
      .pointColor("color")
      .pointAltitude((d: object) => (d as { selected: boolean }).selected ? 0.06 : 0.02)
      .pointRadius((d: object) => (d as { selected: boolean }).selected ? 0.35 : 0.18)
      .pointLabel(
        (d: object) => {
          const p = d as { title: string; text: string };
          return `<div style="background:rgba(15,18,25,0.92);border:1px solid rgba(255,255,255,0.12);border-radius:8px;padding:6px 10px;font-size:11px;color:#e2e8f0;max-width:220px"><strong>${p.title}</strong><br/>${p.text}</div>`;
        },
      )
      .onPointClick(handlePointClick)
      .showGraticules(true)
      .polygonsData([])
      .polygonCapColor((d: object) => (d as PolygonFeature).__color + "22")
      .polygonSideColor((d: object) => (d as PolygonFeature).__color + "11")
      .polygonStrokeColor((d: object) => (d as PolygonFeature).__color)
      .polygonAltitude(0.005)
      .polygonLabel((d: object) => {
        const f = d as PolygonFeature;
        return `<div style="background:rgba(15,18,25,0.92);border:1px solid rgba(255,255,255,0.12);border-radius:8px;padding:6px 10px;font-size:11px;color:#e2e8f0">${f.__label}</div>`;
      })
      .pathsData([])
      .pathColor((d: object) => (d as PathFeature).__color)
      .pathStroke(1.5)
      .pathLabel((d: object) => {
        const f = d as PathFeature;
        return `<div style="background:rgba(15,18,25,0.92);border:1px solid rgba(255,255,255,0.12);border-radius:8px;padding:6px 10px;font-size:11px;color:#e2e8f0">${f.__label}</div>`;
      });

    // Dim scene lights for dark theme
    const scene = globe.scene();
    scene.children.forEach((child: { type?: string; intensity?: number }) => {
      if (child.type === "DirectionalLight") child.intensity = 0.6;
      if (child.type === "AmbientLight") child.intensity = 0.8;
    });

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

  /* Sync points data */
  useEffect(() => {
    globeRef.current?.pointsData(points);
  }, [points]);

  /* Sync overlays */
  useEffect(() => {
    const globe = globeRef.current;
    if (!globe) return;

    const polygons: PolygonFeature[] = [];
    const paths: PathFeature[] = [];

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
            // globe.gl paths: array of [lng, lat] → need [lat, lng, alt]
            paths.push({
              coords: feature.geometry.coordinates.map((c: number[]) => [c[1], c[0]]),
              __color: color,
              __label: label,
            });
          }
          // Points (ports, bases, nuclear) rendered as additional globe points
          // could be added here but alert points already cover the map
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
    });
  }, [activeOverlays]);

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
