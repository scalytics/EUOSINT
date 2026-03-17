/*
 * EUOSINT
 * 3D Globe view using globe.gl with Natural Earth textures.
 */

import { useEffect, useRef, useMemo, useCallback } from "react";
import Globe from "globe.gl";
import type { Alert } from "@/types/alert";
import { severityHex } from "@/lib/theme";

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
}

export function Globe3D({ alerts, selectedId, onSelect, regionFilter }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const globeRef = useRef<ReturnType<typeof Globe> | null>(null);
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

    const globe = Globe()(containerRef.current)
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
      .showGraticules(true);

    // Style graticules to be subtle
    const globeMaterial = globe.globeMaterial() as { color?: { set: (c: string) => void } };
    if (globeMaterial?.color?.set) {
      globeMaterial.color.set("#0d1117");
    }

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
      globe._destructor?.();
      globeRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /* Sync points data */
  useEffect(() => {
    globeRef.current?.pointsData(points);
  }, [points]);

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
