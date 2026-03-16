/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useRef } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import "leaflet.markercluster";
import "leaflet.markercluster/dist/MarkerCluster.css";
import type { Alert } from "@/types/alert";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { severityHex, textHex } from "@/lib/theme";

/* ── Region viewports ─────────────────────────────────────────────── */

type MapViewport = { center: [number, number]; zoom: number };

const REGION_VIEWPORTS: Record<string, MapViewport> = {
  Europe: { center: [50, 10], zoom: 4 },
  "North America": { center: [42, -100], zoom: 4 },
  "South America": { center: [-15, -60], zoom: 3 },
  Africa: { center: [2, 20], zoom: 3 },
  Asia: { center: [34, 100], zoom: 3 },
  Oceania: { center: [-28, 140], zoom: 4 },
  Caribbean: { center: [18, -75], zoom: 5 },
  International: { center: [20, 0], zoom: 2 },
  all: { center: [20, 0], zoom: 2 },
};

/* ── Props ────────────────────────────────────────────────────────── */

interface Props {
  alerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  regionFilter: string;
  onRegionChange: (region: string) => void;
  visibleAlertIds: string[];
  onSelectSourceIdsChange?: (sourceIds: string[]) => void;
  selectedSourceIds?: string[];
}

/* ── Component ────────────────────────────────────────────────────── */

export function GlobeView({
  alerts,
  selectedId,
  onSelect,
  regionFilter,
  onRegionChange,
  onSelectSourceIdsChange,
  selectedSourceIds = [],
  visibleAlertIds,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const clusterRef = useRef<L.MarkerClusterGroup | null>(null);
  const markerLookup = useRef<Map<string, L.CircleMarker>>(new Map());
  const onSelectRef = useRef(onSelect);
  onSelectRef.current = onSelect;

  const toggleSource = (sourceId: string) => {
    if (!onSelectSourceIdsChange) return;
    if (selectedSourceIds.includes(sourceId)) {
      onSelectSourceIdsChange(selectedSourceIds.filter((id) => id !== sourceId));
      return;
    }
    onSelectSourceIdsChange([...selectedSourceIds, sourceId]);
  };

  const visibleIdSet = useMemo(() => new Set(visibleAlertIds), [visibleAlertIds]);

  const visibleAlerts = useMemo(
    () =>
      alerts.filter(
        (a) => visibleIdSet.has(a.alert_id) && alertMatchesRegionFilter(a, regionFilter),
      ),
    [alerts, regionFilter, visibleIdSet],
  );

  /* ── Initialise Leaflet once ──────────────────────────────────── */

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    const vp = REGION_VIEWPORTS[regionFilter] ?? REGION_VIEWPORTS.Europe;

    const map = L.map(containerRef.current, {
      center: vp.center,
      zoom: vp.zoom,
      zoomControl: false,
      attributionControl: false,
    });

    L.tileLayer(
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
      { maxZoom: 18, subdomains: "abcd" },
    ).addTo(map);

    L.control.zoom({ position: "bottomright" }).addTo(map);

    L.control
      .attribution({ position: "bottomleft", prefix: false })
      .addAttribution(
        '&copy; <a href="https://carto.com/attributions">CARTO</a> &copy; <a href="https://www.openstreetmap.org/copyright">OSM</a>',
      )
      .addTo(map);

    const cluster = L.markerClusterGroup({
      maxClusterRadius: 45,
      spiderfyOnMaxZoom: true,
      showCoverageOnHover: false,
      iconCreateFunction(c) {
        const count = c.getChildCount();
        let size: "small" | "medium" | "large" = "small";
        if (count >= 100) size = "large";
        else if (count >= 30) size = "medium";
        return L.divIcon({
          html: `<span>${count}</span>`,
          className: `siem-cluster siem-cluster-${size}`,
          iconSize: L.point(36, 36),
        });
      },
    });

    map.addLayer(cluster);
    mapRef.current = map;
    clusterRef.current = cluster;

    return () => {
      map.remove();
      mapRef.current = null;
      clusterRef.current = null;
      markerLookup.current.clear();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /* ── Sync markers with visible alerts ─────────────────────────── */

  useEffect(() => {
    const cluster = clusterRef.current;
    if (!cluster) return;

    cluster.clearLayers();
    markerLookup.current.clear();

    const markers: L.CircleMarker[] = [];
    for (const alert of visibleAlerts) {
      const selected = alert.alert_id === selectedId;
      const text = textHex();
      const marker = L.circleMarker([alert.lat, alert.lng], {
        radius: selected ? 8 : 5,
        fillColor: severityHex(alert.severity),
        color: selected ? text : `${text}59`,
        weight: selected ? 2 : 0.6,
        fillOpacity: 0.85,
      });

      marker.bindTooltip(
        `<strong>${alert.source.authority_name}</strong><br/>${alert.title.slice(0, 80)}`,
        { className: "siem-tooltip", direction: "top", offset: L.point(0, -6) },
      );

      marker.on("click", () => onSelectRef.current(alert.alert_id));
      markers.push(marker);
      markerLookup.current.set(alert.alert_id, marker);
    }

    cluster.addLayers(markers);
  }, [visibleAlerts, selectedId]);

  /* ── Fly to region on filter change ───────────────────────────── */

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    // For country filters, fit map bounds to visible markers.
    if (regionFilter.startsWith("country:") && visibleAlerts.length > 0) {
      const lats = visibleAlerts.map((a) => a.lat);
      const lngs = visibleAlerts.map((a) => a.lng);
      const bounds = L.latLngBounds(
        [Math.min(...lats) - 1, Math.min(...lngs) - 1],
        [Math.max(...lats) + 1, Math.max(...lngs) + 1]
      );
      map.flyToBounds(bounds, { duration: 0.8, maxZoom: 7, padding: [30, 30] });
      return;
    }

    const vp = REGION_VIEWPORTS[regionFilter] ?? REGION_VIEWPORTS.Europe;
    map.flyTo(vp.center, vp.zoom, { duration: 0.8 });
  }, [regionFilter, visibleAlerts]);

  /* ── Stats for sidebar ────────────────────────────────────────── */

  const topClusters = useMemo(() => {
    const bins = new Map<string, { sourceId: string; title: string; count: number }>();
    for (const alert of visibleAlerts) {
      const key = alert.source.source_id;
      const existing = bins.get(key);
      if (existing) {
        existing.count += 1;
      } else {
        bins.set(key, { sourceId: key, title: alert.source.authority_name, count: 1 });
      }
    }
    return [...bins.values()].sort((a, b) => b.count - a.count).slice(0, 6);
  }, [visibleAlerts]);

  /* ── Render ───────────────────────────────────────────────────── */

  return (
    <section className="relative flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_20px_64px_rgba(0,0,0,0.24)]">
      {/* Header bar */}
      <div className="z-10 flex items-start justify-between p-4">
        <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
          <div className="text-[11px] uppercase tracking-[0.18em] text-siem-muted">Theatre</div>
          <div className="mt-1 text-lg font-semibold text-siem-text">
            {regionFilter === "all"
              ? "Global operating picture"
              : regionFilter.startsWith("country:")
                ? `${regionFilter.slice(8)} operating picture`
                : `${regionFilter} operating picture`}
          </div>
          <div className="mt-1 text-sm text-siem-muted">
            {visibleAlerts.length} visible alerts across official feeds
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2 rounded-2xl border border-siem-border bg-siem-panel p-2 md:grid-cols-4">
          {["Europe", "all", "Asia", "North America"].map((region) => (
            <button
              key={region}
              type="button"
              onClick={() => onRegionChange(region)}
              className={`rounded-xl border px-3 py-2 text-[11px] uppercase tracking-[0.18em] transition-colors ${
                regionFilter === region
                  ? "border-siem-accent bg-siem-accent/16 text-siem-text"
                  : "border-siem-border bg-siem-panel-strong text-siem-muted hover:border-siem-accent/40 hover:bg-siem-accent/8"
              }`}
            >
              {region === "all" ? "Global" : region}
            </button>
          ))}
        </div>
      </div>

      {/* Map + sidebar */}
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_16rem] gap-0">
        <div ref={containerRef} className="relative m-4 mt-0 overflow-hidden rounded-[1.4rem] border border-siem-border" />

        <aside className="m-4 ml-0 mt-0 flex flex-col gap-3 overflow-y-auto">
          <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
            <div className="text-[11px] uppercase tracking-[0.18em] text-siem-muted">Hot sectors</div>
            <div className="mt-3 space-y-2">
              {topClusters.map((cluster) => (
                <button
                  key={cluster.sourceId}
                  type="button"
                  onClick={() => toggleSource(cluster.sourceId)}
                  className={`w-full text-left rounded-xl border px-3 py-2 transition-colors cursor-pointer ${
                    selectedSourceIds.includes(cluster.sourceId)
                      ? "border-siem-accent bg-siem-accent/14"
                      : "border-siem-border bg-siem-panel-strong hover:border-siem-accent/40 hover:bg-siem-accent/8"
                  }`}
                >
                  <div className="text-sm text-siem-text">{cluster.title}</div>
                  <div className="mt-1 text-[11px] uppercase tracking-[0.16em] text-siem-muted">
                    {cluster.count} alerts in sector
                  </div>
                </button>
              ))}
            </div>
          </div>
          <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
            <div className="text-[11px] uppercase tracking-[0.18em] text-siem-muted">Legend</div>
            <div className="mt-3 space-y-2 text-sm text-siem-text">
              {(
                [
                  ["Critical", "bg-siem-critical"],
                  ["High", "bg-siem-high"],
                  ["Medium", "bg-siem-medium"],
                  ["Low", "bg-siem-low"],
                  ["Info", "bg-siem-info"],
                ] as const
              ).map(([label, bgClass]) => (
                <div key={label} className="flex items-center gap-2">
                  <span className={`h-2.5 w-2.5 rounded-full ${bgClass}`} />
                  <span>{label}</span>
                </div>
              ))}
            </div>
          </div>
        </aside>
      </div>
    </section>
  );
}
