import { useEffect, useRef, useState, useCallback } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import "leaflet.markercluster";
import "leaflet.markercluster/dist/MarkerCluster.css";
import { Layers, Check } from "lucide-react";
import type { Alert } from "@/types/alert";
import { severityHex } from "@/lib/theme";
import {
  DEFAULT_OVERLAYS,
  loadOverlay,
  type OverlayDef,
  type OverlayId,
} from "@/lib/map-overlays";

const REGION_VIEWPORTS: Record<string, { center: [number, number]; zoom: number }> = {
  Europe: { center: [50, 10], zoom: 4 },
  "Middle East": { center: [28, 46], zoom: 5 },
  "North America": { center: [42, -100], zoom: 4 },
  "South America": { center: [-15, -60], zoom: 3 },
  Africa: { center: [4, 20], zoom: 4 },
  "Asia-Pacific": { center: [15, 105], zoom: 4 },
  Caribbean: { center: [18, -75], zoom: 5 },
  all: { center: [20, 0], zoom: 3 },
};

interface Props {
  alerts: Alert[];
  regionFilter: string;
  onSelectAlert: (alertId: string) => void;
}

export function MobileMapView({ alerts, regionFilter, onSelectAlert }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const clusterRef = useRef<L.MarkerClusterGroup | null>(null);
  const overlayLayers = useRef<Map<OverlayId, L.LayerGroup>>(new Map());
  const [layerPickerOpen, setLayerPickerOpen] = useState(false);
  const [activeOverlays, setActiveOverlays] = useState<Set<OverlayId>>(new Set());
  const overlayDefs = DEFAULT_OVERLAYS;

  const toggleOverlay = useCallback((id: OverlayId) => {
    setActiveOverlays((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // Initialize map once
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    const vp = REGION_VIEWPORTS[regionFilter] ?? REGION_VIEWPORTS.all;
    const map = L.map(containerRef.current, {
      center: vp.center,
      zoom: vp.zoom,
      zoomControl: false,
      attributionControl: false,
    });

    // Overlay pane above markers
    const overlayPane = map.createPane("overlayPane");
    overlayPane.style.zIndex = "650";

    L.tileLayer(
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
      { maxZoom: 18, subdomains: "abcd" },
    ).addTo(map);

    L.control.zoom({ position: "bottomright" }).addTo(map);

    const cluster = L.markerClusterGroup({
      maxClusterRadius: 50,
      spiderfyOnMaxZoom: false,
      zoomToBoundsOnClick: true,
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

    const ro = new ResizeObserver(() => map.invalidateSize());
    ro.observe(containerRef.current);

    return () => {
      ro.disconnect();
      map.remove();
      mapRef.current = null;
      clusterRef.current = null;
      overlayLayers.current.clear();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fly to region on filter change
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const vp = REGION_VIEWPORTS[regionFilter] ?? REGION_VIEWPORTS.all;
    map.flyTo(vp.center, vp.zoom, { duration: 0.6 });
  }, [regionFilter]);

  // Manage overlay layers
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    // Remove deactivated overlays
    for (const [id, layer] of overlayLayers.current.entries()) {
      if (!activeOverlays.has(id)) {
        map.removeLayer(layer);
        overlayLayers.current.delete(id);
      }
    }

    // Add newly activated overlays
    for (const id of activeOverlays) {
      if (overlayLayers.current.has(id)) continue;
      const def = overlayDefs.find((o) => o.id === id);
      if (!def) continue;
      loadOverlay(map, def, { regionFilter }).then((layer) => {
        if (!mapRef.current || !activeOverlays.has(id)) {
          map.removeLayer(layer);
          return;
        }
        const prev = overlayLayers.current.get(id);
        if (prev) map.removeLayer(prev);
        overlayLayers.current.set(id, layer);
      });
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeOverlays, regionFilter]);

  // Update alert markers
  useEffect(() => {
    const cluster = clusterRef.current;
    if (!cluster) return;

    cluster.clearLayers();

    for (const alert of alerts) {
      if (alert.lat === 0 && alert.lng === 0) continue;
      const marker = L.circleMarker([alert.lat, alert.lng], {
        radius: 6,
        fillColor: severityHex(alert.severity),
        color: "#ffffff40",
        weight: 1,
        fillOpacity: 0.85,
      });
      marker.bindTooltip(
        `<strong>${alert.source.authority_name}</strong><br/>${alert.title.slice(0, 60)}`,
        { className: "siem-tooltip", direction: "top" },
      );
      marker.on("click", () => onSelectAlert(alert.alert_id));
      cluster.addLayer(marker);
    }
  }, [alerts, onSelectAlert]);

  return (
    <div className="relative h-full">
      <div ref={containerRef} className="mobile-map" />

      {/* Layers FAB */}
      <button
        className="mobile-layers-fab"
        onClick={() => setLayerPickerOpen(true)}
      >
        <Layers size={20} />
        {activeOverlays.size > 0 && (
          <span className="mobile-layers-badge">{activeOverlays.size}</span>
        )}
      </button>

      {/* Overlay picker sheet */}
      {layerPickerOpen && (
        <>
          <div
            className="mobile-sheet-backdrop"
            onClick={() => setLayerPickerOpen(false)}
          />
          <div className="mobile-picker-sheet">
            <div className="mobile-sheet-handle" />
            <div className="mobile-picker-title">Map Layers</div>
            {overlayDefs.map((def: OverlayDef) => (
              <button
                key={def.id}
                className={`mobile-picker-item ${activeOverlays.has(def.id) ? "active" : ""}`}
                onClick={() => toggleOverlay(def.id)}
              >
                <span className="flex items-center gap-2">
                  <span
                    className="inline-block w-3 h-3 rounded-full flex-shrink-0"
                    style={{ background: def.color }}
                  />
                  {def.label}
                </span>
                {activeOverlays.has(def.id) && (
                  <Check size={16} className="text-sky-400" />
                )}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
