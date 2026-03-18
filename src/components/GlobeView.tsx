/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import "leaflet.markercluster";
import "leaflet.markercluster/dist/MarkerCluster.css";
import type { Alert } from "@/types/alert";
import { alertMatchesRegionFilter } from "@/lib/regions";
import { severityHex, textHex } from "@/lib/theme";
import { OVERLAYS, loadOverlay, type OverlayId } from "@/lib/map-overlays";
import { detectSpikes } from "@/lib/activity-spikes";

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
  International: { center: [20, 0], zoom: 3 },
  all: { center: [20, 0], zoom: 3 },
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
  const overlayLayers = useRef<Map<OverlayId, L.LayerGroup>>(new Map());
  const [activeOverlays, setActiveOverlays] = useState<Set<OverlayId>>(new Set());
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

  const toggleOverlay = useCallback((id: OverlayId) => {
    setActiveOverlays((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
        const layer = overlayLayers.current.get(id);
        if (layer && mapRef.current) {
          mapRef.current.removeLayer(layer);
          overlayLayers.current.delete(id);
        }
      } else {
        next.add(id);
        const map = mapRef.current;
        if (map) {
          const def = OVERLAYS.find((o) => o.id === id);
          if (def) {
            loadOverlay(map, def).then((layer) => {
              overlayLayers.current.set(id, layer);
            });
          }
        }
      }
      return next;
    });
  }, []);

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
      minZoom: 3,
      maxBounds: L.latLngBounds([-85, -180], [85, 180]),
      maxBoundsViscosity: 1.0,
      zoomControl: false,
      attributionControl: false,
    });

    L.tileLayer(
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
      { maxZoom: 18, subdomains: "abcd", noWrap: true },
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

    // Leaflet needs a correctly-sized container. On force-reload the
    // layout may not yet be computed when the map initialises, giving it
    // 0×0 dimensions. A ResizeObserver fixes this by calling
    // invalidateSize() whenever the container gets its real size.
    const ro = new ResizeObserver(() => {
      map.invalidateSize();
    });
    ro.observe(containerRef.current);

    const markers = markerLookup.current;
    return () => {
      ro.disconnect();
      map.remove();
      mapRef.current = null;
      clusterRef.current = null;
      markers.clear();
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
      // Skip alerts with no resolved location (0,0).
      if (alert.lat === 0 && alert.lng === 0) continue;
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

  const activitySpikes = useMemo(() => detectSpikes(alerts), [alerts]);

  /* ── Render ───────────────────────────────────────────────────── */

  return (
    <section className="relative flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_20px_64px_rgba(0,0,0,0.24)]">
      {/* Header bar */}
      <div className="z-10 flex items-start justify-between p-4">
        <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
          <div className="text-xxs uppercase tracking-[0.18em] text-siem-muted">Theatre</div>
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

        <div className="rounded-2xl border border-siem-border bg-siem-panel p-2 space-y-1.5">
          <div className="grid grid-cols-2 gap-1.5 md:grid-cols-4">
            {["Europe", "all", "Asia", "North America"].map((region) => (
              <button
                key={region}
                type="button"
                onClick={() => onRegionChange(region)}
                className={`rounded-lg border px-2.5 py-1.5 text-2xs uppercase tracking-[0.18em] transition-colors ${
                  regionFilter === region
                    ? "border-siem-accent bg-siem-accent/16 text-siem-text"
                    : "border-siem-border bg-siem-panel-strong text-siem-muted hover:border-siem-accent/40 hover:bg-siem-accent/8"
                }`}
              >
                {region === "all" ? "Global" : region}
              </button>
            ))}
          </div>
          <div className="grid grid-cols-4 gap-1 md:grid-cols-7">
            {OVERLAYS.map((overlay) => (
              <button
                key={overlay.id}
                type="button"
                onClick={() => toggleOverlay(overlay.id)}
                className={`flex items-center justify-center gap-1 rounded-md border px-1 py-0.5 text-3xs uppercase tracking-[0.12em] transition-colors ${
                  activeOverlays.has(overlay.id)
                    ? "border-siem-accent/40 bg-siem-accent/12 text-siem-text"
                    : "border-siem-border bg-siem-panel-strong text-siem-muted hover:border-siem-accent/30 hover:bg-siem-accent/8"
                }`}
              >
                <span
                  className="h-1.5 w-1.5 rounded-full shrink-0"
                  style={{ backgroundColor: overlay.color, opacity: activeOverlays.has(overlay.id) ? 1 : 0.4 }}
                />
                {overlay.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Map + sidebar */}
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_16rem] gap-0">
        <div className="relative m-4 mt-0 min-h-0">
          <div ref={containerRef} className="absolute inset-0 overflow-hidden rounded-[1.4rem] border border-siem-border" />

          <div className="pointer-events-none absolute left-1/2 top-[-5px] z-[1000] -translate-x-1/2">
            <div className="flex items-center gap-2 rounded-full border border-siem-border/80 bg-siem-panel/92 px-3 py-1.5 text-2xs uppercase tracking-[0.14em] text-siem-muted shadow-[0_10px_24px_rgba(0,0,0,0.24)] backdrop-blur-sm">
              {(
                [
                  ["Critical", "bg-siem-critical"],
                  ["High", "bg-siem-high"],
                  ["Medium", "bg-siem-medium"],
                  ["Low", "bg-siem-low"],
                  ["Info", "bg-siem-info"],
                ] as const
              ).map(([label, bgClass]) => (
                <span key={label} className="inline-flex items-center gap-1.5">
                  <span className={`h-2 w-2 rounded-full ${bgClass}`} />
                  <span>{label}</span>
                </span>
              ))}
            </div>
          </div>
        </div>

        <aside className="m-4 ml-0 mt-0 flex flex-col gap-3 overflow-y-auto">
          <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
            <div className="text-xxs uppercase tracking-[0.18em] text-siem-muted">Hot sectors</div>
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
                  <div className="mt-1 text-xxs uppercase tracking-[0.16em] text-siem-muted">
                    {cluster.count} alerts in sector
                  </div>
                </button>
              ))}
            </div>
          </div>
          {activitySpikes.length > 0 && (
            <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
              <div className="text-xxs uppercase tracking-[0.18em] text-siem-muted">Activity spikes</div>
              <div className="mt-3 space-y-1.5">
                {activitySpikes.slice(0, 5).map((spike) => (
                  <button
                    key={spike.countryCode}
                    type="button"
                    onClick={() => onRegionChange(`country:${spike.countryCode}`)}
                    className="w-full text-left rounded-xl border border-siem-border bg-siem-panel-strong px-3 py-2 hover:border-siem-accent/40 hover:bg-siem-accent/8 transition-colors"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-xs text-siem-text truncate">{spike.country}</span>
                      <span
                        className={`shrink-0 rounded px-1.5 py-0.5 text-3xs font-bold uppercase tracking-wider border ${
                          spike.level === "surge"
                            ? "bg-red-500/15 text-red-300 border-red-500/30"
                            : "bg-amber-500/15 text-amber-300 border-amber-500/30"
                        }`}
                      >
                        {spike.level === "surge" ? "Surge" : "Elevated"}
                      </span>
                    </div>
                    <div className="mt-1 text-2xs text-siem-muted font-mono">
                      {spike.last24h} alerts / 24h — {spike.ratio}x baseline
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </aside>
      </div>
    </section>
  );
}
