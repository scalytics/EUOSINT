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
import { loadOverlayDefs, loadOverlay, type OverlayDef, type OverlayId } from "@/lib/map-overlays";
import { detectSpikes } from "@/lib/activity-spikes";
import { getConflictLensById } from "@/lib/conflict-lenses";
import { buildConflictBrief, mergeZoneBriefing } from "@/lib/conflict-briefs";
import { useZoneBriefings } from "@/hooks/useZoneBriefings";

/* ── Region viewports ─────────────────────────────────────────────── */

type MapViewport = { center: [number, number]; zoom: number };

const REGION_VIEWPORTS: Record<string, MapViewport> = {
  Europe: { center: [50, 10], zoom: 4 },
  "North America": { center: [42, -100], zoom: 4 },
  "South America": { center: [-15, -60], zoom: 3 },
  Africa: { center: [4, 20], zoom: 4 },
  Asia: { center: [30, 90], zoom: 4 },
  Oceania: { center: [-28, 140], zoom: 4 },
  Caribbean: { center: [18, -75], zoom: 5 },
  International: { center: [20, 0], zoom: 3 },
  all: { center: [20, 0], zoom: 3 },
};

const LARGE_COUNTRY_CODES = new Set([
  "US", "CA", "BR", "RU", "CN", "IN", "AU", "MX", "AR",
  "KZ", "DZ", "SA", "ID",
]);

function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function dominantCountryLabel(alerts: Alert[]): string {
  const counts = new Map<string, number>();
  for (const alert of alerts) {
    const label = (alert.event_country || alert.source.country || "").trim();
    if (!label) continue;
    counts.set(label, (counts.get(label) ?? 0) + 1);
  }
  let best = "";
  let max = 0;
  for (const [label, n] of counts.entries()) {
    if (n > max) {
      best = label;
      max = n;
    }
  }
  return best || "Unknown";
}

function formatSubcategory(raw: string | undefined): string {
  const value = (raw ?? "").trim();
  if (!value) return "";
  return value
    .split("_")
    .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
    .join(" ");
}

function formatCategory(raw: string): string {
  const value = raw.trim();
  if (!value) return "Unknown";
  return value
    .split("_")
    .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
    .join(" ");
}

/* ── Props ────────────────────────────────────────────────────────── */

interface Props {
  alerts: Alert[];
  historicalAlerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  regionFilter: string;
  onRegionChange: (region: string) => void;
  conflictLensId: string | null;
  visibleNowAlertIds: string[];
  visibleHistoryAlertIds: string[];
  onSelectSourceIdsChange?: (sourceIds: string[]) => void;
  selectedSourceIds?: string[];
}

/* ── Component ────────────────────────────────────────────────────── */

export function GlobeView({
  alerts,
  historicalAlerts,
  selectedId,
  onSelect,
  regionFilter,
  onRegionChange,
  conflictLensId,
  onSelectSourceIdsChange,
  selectedSourceIds = [],
  visibleNowAlertIds,
  visibleHistoryAlertIds,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const clusterRef = useRef<L.MarkerClusterGroup | null>(null);
  const markerLookup = useRef<Map<string, L.CircleMarker>>(new Map());
  const markerAlertLookup = useRef<Map<number, Alert>>(new Map());
  const overlayLayers = useRef<Map<OverlayId, L.LayerGroup>>(new Map());
  const conflictHotspotLayerRef = useRef<L.LayerGroup | null>(null);
  const overlayLoadTokens = useRef<Map<OverlayId, number>>(new Map());
  const activeOverlaysRef = useRef<Set<OverlayId>>(new Set());
  const lastOverlayRegionRef = useRef<string>(regionFilter);
  const clusterListPopupRef = useRef<L.Popup | null>(null);
  const lastNonCountryRegionRef = useRef<string>("all");
  const countryTableReturnRegionRef = useRef<string>("all");
  const countryFocusFromSpikeRef = useRef<string>("");
  const lastConflictLensRef = useRef<string | null>(null);
  const preLensOverlaysRef = useRef<Set<OverlayId> | null>(null);
  const [overlayDefs, setOverlayDefs] = useState<OverlayDef[]>([]);
  const [activeOverlays, setActiveOverlays] = useState<Set<OverlayId>>(new Set());
  const [activeAreaGroupID, setActiveAreaGroupID] = useState<string>("");
  const onSelectRef = useRef(onSelect);
  onSelectRef.current = onSelect;
  const countryFilterCode = useMemo(
    () => (regionFilter.startsWith("country:") ? regionFilter.slice(8).toUpperCase() : ""),
    [regionFilter],
  );
  const isLargeCountryScope = countryFilterCode !== "" && LARGE_COUNTRY_CODES.has(countryFilterCode);
  const { briefings: zoneBriefings } = useZoneBriefings();

  useEffect(() => {
    if (!regionFilter.startsWith("country:")) {
      lastNonCountryRegionRef.current = regionFilter;
    }
  }, [regionFilter]);

  const toggleSource = (sourceId: string) => {
    if (!onSelectSourceIdsChange) return;
    if (selectedSourceIds.includes(sourceId)) {
      onSelectSourceIdsChange(selectedSourceIds.filter((id) => id !== sourceId));
      return;
    }
    onSelectSourceIdsChange([...selectedSourceIds, sourceId]);
  };

  useEffect(() => {
    let cancelled = false;
    loadOverlayDefs().then((defs) => {
      if (!cancelled) {
        setOverlayDefs(defs);
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const sortedOverlayDefs = useMemo(() => {
    const isZones = (label: string) => label.trim().toLowerCase().endsWith("zones");
    const nonZones = overlayDefs.filter((overlay) => !isZones(overlay.label));
    const zones = overlayDefs.filter((overlay) => isZones(overlay.label));
    return [...nonZones, ...zones];
  }, [overlayDefs]);

  useEffect(() => {
    // Drop no-longer-registered active overlays when manifest changes.
    setActiveOverlays((prev) => {
      const allowed = new Set(overlayDefs.map((o) => o.id));
      const next = new Set<OverlayId>();
      for (const id of prev) {
        if (allowed.has(id)) {
          next.add(id);
        } else {
          const layer = overlayLayers.current.get(id);
          if (layer && mapRef.current) {
            mapRef.current.removeLayer(layer);
          }
          overlayLayers.current.delete(id);
        }
      }
      return next;
    });
  }, [overlayDefs]);

  useEffect(() => {
    activeOverlaysRef.current = activeOverlays;
  }, [activeOverlays]);

  useEffect(() => {
    const previousLens = getConflictLensById(lastConflictLensRef.current);
    const nextLens = getConflictLensById(conflictLensId);
    lastConflictLensRef.current = conflictLensId;
    const lensDefaults: OverlayId[] = ["bases"];

    setActiveOverlays((prev) => {
      if (!previousLens && nextLens) {
        preLensOverlaysRef.current = new Set(prev);
      }
      if (previousLens && !nextLens && preLensOverlaysRef.current) {
        const restored = new Set(preLensOverlaysRef.current);
        preLensOverlaysRef.current = null;
        return restored;
      }

      const next = new Set(prev);
      if (previousLens) {
        for (const overlay of previousLens.overlays) {
          next.delete(overlay);
        }
        for (const overlay of lensDefaults) {
          next.delete(overlay);
        }
      }
      if (nextLens) {
        for (const overlay of nextLens.overlays) {
          next.add(overlay);
        }
        for (const overlay of lensDefaults) {
          next.add(overlay);
        }
      } else {
        preLensOverlaysRef.current = null;
      }
      return next;
    });
  }, [conflictLensId]);

  const nextOverlayToken = useCallback((id: OverlayId) => {
    const next = (overlayLoadTokens.current.get(id) ?? 0) + 1;
    overlayLoadTokens.current.set(id, next);
    return next;
  }, []);

  const toggleOverlay = useCallback((id: OverlayId) => {
    setActiveOverlays((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (lastOverlayRegionRef.current !== regionFilter) {
      lastOverlayRegionRef.current = regionFilter;
      for (const [id, layer] of overlayLayers.current.entries()) {
        nextOverlayToken(id);
        map.removeLayer(layer);
        overlayLayers.current.delete(id);
      }
    }

    for (const [id, layer] of overlayLayers.current.entries()) {
      if (!activeOverlays.has(id)) {
        nextOverlayToken(id);
        map.removeLayer(layer);
        overlayLayers.current.delete(id);
      }
    }

    for (const id of activeOverlays) {
      if (overlayLayers.current.has(id)) continue;
      const def = overlayDefs.find((o) => o.id === id);
      if (!def) continue;
      const token = nextOverlayToken(id);
      loadOverlay(map, def, { regionFilter, conflictLensId }).then((layer) => {
        const isLatest = overlayLoadTokens.current.get(id) === token;
        const stillActive = activeOverlaysRef.current.has(id);
        if (!isLatest || !stillActive || !mapRef.current) {
          map.removeLayer(layer);
          return;
        }
        const previous = overlayLayers.current.get(id);
        if (previous && previous !== layer) {
          map.removeLayer(previous);
        }
        overlayLayers.current.set(id, layer);
      });
    }
  }, [activeOverlays, conflictLensId, nextOverlayToken, overlayDefs, regionFilter]);

  const visibleNowIdSet = useMemo(() => new Set(visibleNowAlertIds), [visibleNowAlertIds]);
  const visibleHistoryIdSet = useMemo(() => new Set(visibleHistoryAlertIds), [visibleHistoryAlertIds]);

  const visibleNowAlerts = useMemo(
    () =>
      alerts.filter(
        (a) => visibleNowIdSet.has(a.alert_id) && alertMatchesRegionFilter(a, regionFilter),
      ),
    [alerts, regionFilter, visibleNowIdSet],
  );

  const visibleHistoryAlerts = useMemo(
    () =>
      historicalAlerts.filter(
        (a) => visibleHistoryIdSet.has(a.alert_id) && alertMatchesRegionFilter(a, regionFilter),
      ),
    [historicalAlerts, regionFilter, visibleHistoryIdSet],
  );

  const visibleHistoryAlertsRendered = useMemo(
    () => visibleHistoryAlerts,
    [visibleHistoryAlerts],
  );

  const combinedVisibleAlerts = useMemo(
    () => [...visibleNowAlerts, ...visibleHistoryAlertsRendered],
    [visibleNowAlerts, visibleHistoryAlertsRendered],
  );

  const geocodedVisibleAlerts = useMemo(
    () => combinedVisibleAlerts.filter((a) => !(a.lat === 0 && a.lng === 0)),
    [combinedVisibleAlerts],
  );

  const countryFocusCenter = useMemo(() => {
    if (countryFilterCode === "") return null as [number, number] | null;
    const countryAlerts = [...alerts, ...historicalAlerts].filter((a) => {
      const sourceCode = (a.source.country_code || "").toUpperCase();
      const eventCode = (a.event_country_code || "").toUpperCase();
      return sourceCode === countryFilterCode || eventCode === countryFilterCode;
    });
    const geocoded = countryAlerts.filter((a) => !(a.lat === 0 && a.lng === 0));
    if (geocoded.length === 0) return null;
    const lat = geocoded.reduce((sum, a) => sum + a.lat, 0) / geocoded.length;
    const lng = geocoded.reduce((sum, a) => sum + a.lng, 0) / geocoded.length;
    return [lat, lng] as [number, number];
  }, [alerts, historicalAlerts, countryFilterCode]);

  const areaGroups = useMemo(() => {
    if (countryFilterCode === "") {
      return [] as Array<{
        id: string;
        label: string;
        count: number;
        critical: number;
        high: number;
        lat: number;
        lng: number;
        alerts: Alert[];
      }>;
    }

    if (geocodedVisibleAlerts.length === 0) {
      if (combinedVisibleAlerts.length === 0) {
        return [] as Array<{
          id: string;
          label: string;
          count: number;
          critical: number;
          high: number;
          lat: number;
          lng: number;
          alerts: Alert[];
        }>;
      }
      return [{
        id: `${countryFilterCode}-national`,
        label: "National",
        count: combinedVisibleAlerts.length,
        critical: combinedVisibleAlerts.filter((a) => a.severity === "critical").length,
        high: combinedVisibleAlerts.filter((a) => a.severity === "high").length,
        lat: countryFocusCenter?.[0] ?? 0,
        lng: countryFocusCenter?.[1] ?? 0,
        alerts: combinedVisibleAlerts.sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
      }];
    }

    const avgLat = geocodedVisibleAlerts.reduce((sum, a) => sum + a.lat, 0) / geocodedVisibleAlerts.length;
    const avgLng = geocodedVisibleAlerts.reduce((sum, a) => sum + a.lng, 0) / geocodedVisibleAlerts.length;

    if (!isLargeCountryScope) {
      return [{
        id: `${countryFilterCode}-national`,
        label: "National",
        count: geocodedVisibleAlerts.length,
        critical: geocodedVisibleAlerts.filter((a) => a.severity === "critical").length,
        high: geocodedVisibleAlerts.filter((a) => a.severity === "high").length,
        lat: avgLat,
        lng: avgLng,
        alerts: geocodedVisibleAlerts,
      }];
    }

    const latStep = 1.2;
    const lngStep = 1.2;
    const groups = new Map<string, Alert[]>();
    for (const alert of geocodedVisibleAlerts) {
      const ns = alert.lat > avgLat+latStep ? "North" : alert.lat < avgLat-latStep ? "South" : "Central";
      const ew = alert.lng > avgLng+lngStep ? "East" : alert.lng < avgLng-lngStep ? "West" : "Central";
      let label = "Central";
      if (ns === "Central" && ew !== "Central") label = ew;
      else if (ew === "Central" && ns !== "Central") label = ns;
      else if (ns !== "Central" && ew !== "Central") label = `${ns} ${ew}`;
      const key = label.toLowerCase().replace(/\s+/g, "-");
      const list = groups.get(key) ?? [];
      list.push(alert);
      groups.set(key, list);
    }

    return [...groups.entries()]
      .map(([id, items]) => ({
        id,
        label: id.split("-").map((p) => p.charAt(0).toUpperCase() + p.slice(1)).join(" "),
        count: items.length,
        critical: items.filter((a) => a.severity === "critical").length,
        high: items.filter((a) => a.severity === "high").length,
        lat: items.reduce((sum, a) => sum + a.lat, 0) / items.length,
        lng: items.reduce((sum, a) => sum + a.lng, 0) / items.length,
        alerts: items.sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()),
      }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 8);
  }, [combinedVisibleAlerts, countryFilterCode, countryFocusCenter, geocodedVisibleAlerts, isLargeCountryScope]);

  useEffect(() => {
    if (areaGroups.length === 0) {
      setActiveAreaGroupID("");
      return;
    }
    if (!areaGroups.some((g) => g.id === activeAreaGroupID)) {
      setActiveAreaGroupID(areaGroups[0].id);
    }
  }, [activeAreaGroupID, areaGroups]);

  const activeAreaGroup = useMemo(
    () => areaGroups.find((group) => group.id === activeAreaGroupID) ?? null,
    [activeAreaGroupID, areaGroups],
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
      spiderfyOnMaxZoom: false,
      zoomToBoundsOnClick: false,
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
    const map = mapRef.current;
    if (!cluster || !map) return;

    cluster.clearLayers();
    markerLookup.current.clear();
    markerAlertLookup.current.clear();
    if (countryFilterCode !== "") {
      if (clusterListPopupRef.current) {
        map.closePopup(clusterListPopupRef.current);
        clusterListPopupRef.current = null;
      }
    }

    const isNowAndHistoryMode = visibleNowAlerts.length > 0 && visibleHistoryAlertsRendered.length > 0;
    const markers: L.CircleMarker[] = [];
    for (const alert of visibleHistoryAlertsRendered) {
      // Skip alerts with no resolved location (0,0).
      if (alert.lat === 0 && alert.lng === 0) continue;
      const selected = alert.alert_id === selectedId;
      const text = textHex();
      const marker = L.circleMarker([alert.lat, alert.lng], {
        radius: selected ? (isNowAndHistoryMode ? 9 : 10) : (isNowAndHistoryMode ? 5 : 6),
        fillColor: severityHex(alert.severity),
        color: selected ? `${text}${isNowAndHistoryMode ? "AA" : "CC"}` : `${text}${isNowAndHistoryMode ? "4D" : "66"}`,
        weight: selected ? 2 : 1,
        fillOpacity: isNowAndHistoryMode ? 0.08 : 0.2,
        opacity: isNowAndHistoryMode ? 0.2 : 0.45,
      });

      marker.bindTooltip(
        `<strong>${alert.source.authority_name}</strong><br/>[History] ${alert.title.slice(0, 76)}`,
        { className: "siem-tooltip", direction: "top", offset: L.point(0, -6) },
      );

      marker.on("click", () => onSelectRef.current(alert.alert_id));
      markers.push(marker);
      markerLookup.current.set(alert.alert_id, marker);
      markerAlertLookup.current.set(L.Util.stamp(marker), alert);
    }

    for (const alert of visibleNowAlerts) {
      // Skip alerts with no resolved location (0,0).
      if (alert.lat === 0 && alert.lng === 0) continue;
      const selected = alert.alert_id === selectedId;
      const text = textHex();
      const marker = L.circleMarker([alert.lat, alert.lng], {
        radius: selected ? 11 : 7,
        fillColor: severityHex(alert.severity),
        color: selected ? text : `${text}59`,
        weight: selected ? 2.5 : 1,
        fillOpacity: 0.85,
      });

      marker.bindTooltip(
        `<strong>${alert.source.authority_name}</strong><br/>${alert.title.slice(0, 80)}`,
        { className: "siem-tooltip", direction: "top", offset: L.point(0, -6) },
      );

      marker.on("click", () => onSelectRef.current(alert.alert_id));
      markers.push(marker);
      markerLookup.current.set(alert.alert_id, marker);
      markerAlertLookup.current.set(L.Util.stamp(marker), alert);
    }

    cluster.addLayers(markers);

    const closeClusterList = () => {
      if (clusterListPopupRef.current) {
        map.closePopup(clusterListPopupRef.current);
        clusterListPopupRef.current = null;
      }
    };

    const onClusterClick = (e: L.LeafletEvent & { layer?: L.MarkerCluster }) => {
      const cl = e.layer;
      if (!cl) return;
      closeClusterList();

      const childAlerts: Alert[] = (cl.getAllChildMarkers() as L.Layer[])
        .map((m: L.Layer) => markerAlertLookup.current.get(L.Util.stamp(m)))
        .filter((a): a is Alert => Boolean(a))
        .sort((a: Alert, b: Alert) => {
          const sa = a.severity === "critical" ? 4 : a.severity === "high" ? 3 : a.severity === "medium" ? 2 : a.severity === "low" ? 1 : 0;
          const sb = b.severity === "critical" ? 4 : b.severity === "high" ? 3 : b.severity === "medium" ? 2 : b.severity === "low" ? 1 : 0;
          if (sb !== sa) return sb - sa;
          return new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime();
        });
      if (childAlerts.length === 0) return;

      const countryLabel = escapeHtml(dominantCountryLabel(childAlerts));
      const pageSize = 80;
      let visibleLimit = Math.min(pageSize, childAlerts.length);

      const renderContent = (limit: number): string => {
        const grouped = new Map<string, Alert[]>();
        for (const alert of childAlerts.slice(0, limit)) {
          const key = alert.category || "unknown";
          const list = grouped.get(key) ?? [];
          list.push(alert);
          grouped.set(key, list);
        }
        const sections = [...grouped.entries()]
          .sort((a, b) => b[1].length - a[1].length)
          .map(([category, items]) => {
            const categoryLabel = escapeHtml(formatCategory(category).toUpperCase());
            const rows = items
              .map((alert: Alert, idx: number) => {
                const title = escapeHtml(alert.title);
                const subcategory = escapeHtml(formatSubcategory(alert.subcategory));
                const link = escapeHtml(alert.canonical_url);
                const sev = escapeHtml(alert.severity.toUpperCase());
                const meta = subcategory
                  ? `<div style="font-size:10px;line-height:1.2;color:#94a3b8;margin-top:2px;">${subcategory}</div>`
                  : "";
                const sourceLink = link
                  ? `<a href="${link}" target="_blank" rel="noopener noreferrer" style="display:inline-block;margin-top:4px;font-size:10px;color:#60a5fa;text-decoration:none;">Open source ↗</a>`
                  : "";
                return `<div style="border-bottom:1px solid rgba(148,163,184,.16);padding:6px 0;"><button data-alert-id="${alert.alert_id}" class="cluster-list-row" style="display:block;width:100%;text-align:left;background:transparent;border:0;padding:0;cursor:pointer;font-size:12px;line-height:1.3;color:#f3f4f6;"><div style="display:flex;align-items:center;gap:6px;"><span style="font-size:9px;letter-spacing:.08em;text-transform:uppercase;color:#9ca3af;border:1px solid rgba(148,163,184,.35);padding:1px 4px;border-radius:4px;">${sev}</span><span>${idx + 1}. ${title}</span></div>${meta}</button>${sourceLink}</div>`;
              })
              .join("");
            return `<div style="margin-bottom:8px;"><div style="display:inline-flex;align-items:center;gap:6px;font-size:10px;letter-spacing:.1em;text-transform:uppercase;color:#cbd5e1;border:1px solid rgba(148,163,184,.35);background:rgba(15,23,42,.45);padding:2px 8px;border-radius:999px;margin:4px 0 6px 0;">${categoryLabel}<span style="color:#94a3b8;">(${items.length})</span></div>${rows}</div>`;
          })
          .join("");

        const hasMore = limit < childAlerts.length;
        const remaining = childAlerts.length - limit;
        const loadMore = hasMore
          ? `<button data-load-more="1" style="display:block;width:100%;margin-top:4px;padding:6px 8px;border:1px solid rgba(148,163,184,.3);border-radius:8px;background:rgba(15,23,42,.45);color:#cbd5e1;font-size:11px;cursor:pointer;">Load more alerts (+${remaining})</button>`
          : "";

        return `<div style="min-width:300px;max-width:430px;"><div style="font-size:10px;letter-spacing:.12em;text-transform:uppercase;color:#9ca3af;margin-bottom:6px;">${countryLabel} AREA ALERTS (${childAlerts.length})</div><div style="max-height:300px;overflow:auto;padding-right:2px;">${sections}${loadMore}</div></div>`;
      };

      const popup = L.popup({
        autoClose: false,
        closeOnClick: false,
        closeButton: true,
        className: "siem-cluster-list-popup",
      })
        .setLatLng(cl.getLatLng())
        .setContent(renderContent(visibleLimit))
        .openOn(map);
      clusterListPopupRef.current = popup;

      const bindPopupHandlers = () => {
        const el = popup.getElement();
        if (!el) return;
        el.querySelectorAll<HTMLButtonElement>(".cluster-list-row").forEach((btn) => {
          btn.addEventListener("click", (evt) => {
            evt.preventDefault();
            evt.stopPropagation();
            const id = btn.dataset.alertId;
            if (id) {
              onSelectRef.current(id);
            }
          });
        });
        el.querySelectorAll<HTMLButtonElement>("button[data-load-more='1']").forEach((btn) => {
          btn.addEventListener("click", (evt) => {
            evt.preventDefault();
            evt.stopPropagation();
            visibleLimit = Math.min(childAlerts.length, visibleLimit + pageSize);
            popup.setContent(renderContent(visibleLimit));
            bindPopupHandlers();
          });
        });
      };

      popup.once("add", bindPopupHandlers);
    };

    cluster.on("clusterclick", onClusterClick);
    map.on("click", closeClusterList);
    map.on("zoomstart", closeClusterList);

    return () => {
      cluster.off("clusterclick", onClusterClick);
      map.off("click", closeClusterList);
      map.off("zoomstart", closeClusterList);
      closeClusterList();
    };
  }, [countryFilterCode, visibleNowAlerts, visibleHistoryAlertsRendered, selectedId]);

  /* ── Fly to region on filter change ───────────────────────────── */

  // Track the last region filter so we only fly when the *filter* changes,
  // not when alert data updates (which was causing the zoom-back bug).
  const lastRegionRef = useRef(regionFilter);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    // Only fly when the region filter actually changed.
    if (lastRegionRef.current === regionFilter) return;
    lastRegionRef.current = regionFilter;

    // Country-first view from Activity Spikes: center at country level,
    // avoid pin-driven deep zoom that feels overly precise.
    if (countryFilterCode !== "" && countryFocusFromSpikeRef.current === countryFilterCode) {
      const center = countryFocusCenter;
      if (center) {
        map.flyTo(center, isLargeCountryScope ? 4 : 5, { duration: 0.8 });
      } else {
        map.flyTo(REGION_VIEWPORTS.all.center, 3, { duration: 0.8 });
      }
      return;
    }

    // Country-first view: keep situational zoom, avoid village-level drill-in.
    if (countryFilterCode !== "" && geocodedVisibleAlerts.length > 0) {
      const lats = geocodedVisibleAlerts.map((a) => a.lat);
      const lngs = geocodedVisibleAlerts.map((a) => a.lng);
      if (isLargeCountryScope) {
        const bounds = L.latLngBounds(
          [Math.min(...lats) - 1, Math.min(...lngs) - 1],
          [Math.max(...lats) + 1, Math.max(...lngs) + 1],
        );
        map.flyToBounds(bounds, { duration: 0.8, maxZoom: 5, padding: [40, 40] });
      } else {
        const centerLat = lats.reduce((sum, v) => sum + v, 0) / lats.length;
        const centerLng = lngs.reduce((sum, v) => sum + v, 0) / lngs.length;
        map.flyTo([centerLat, centerLng], 5, { duration: 0.8 });
      }
      return;
    }

    const vp = REGION_VIEWPORTS[regionFilter] ?? REGION_VIEWPORTS.Europe;
    map.flyTo(vp.center, vp.zoom, { duration: 0.8 });
  }, [countryFilterCode, countryFocusCenter, geocodedVisibleAlerts, isLargeCountryScope, regionFilter]);

  useEffect(() => {
    const lens = getConflictLensById(conflictLensId);
    const map = mapRef.current;
    if (!lens || !map) return;
    map.flyTo(lens.viewport.center, lens.viewport.zoom, { duration: 0.8 });
  }, [conflictLensId]);

  /* ── Stats for sidebar ────────────────────────────────────────── */

  const topClusters = useMemo(() => {
    const bins = new Map<string, { sourceId: string; title: string; count: number }>();
    for (const alert of combinedVisibleAlerts) {
      const key = alert.source.source_id;
      const existing = bins.get(key);
      if (existing) {
        existing.count += 1;
      } else {
        bins.set(key, { sourceId: key, title: alert.source.authority_name, count: 1 });
      }
    }
    return [...bins.values()].sort((a, b) => b.count - a.count).slice(0, 6);
  }, [combinedVisibleAlerts]);

  const activitySpikes = useMemo(() => detectSpikes([...alerts, ...historicalAlerts]), [alerts, historicalAlerts]);
  const activeConflictLens = useMemo(() => getConflictLensById(conflictLensId), [conflictLensId]);
  const activeConflictBrief = useMemo(() => {
    const derived = buildConflictBrief([...alerts, ...historicalAlerts], activeConflictLens);
    const override = activeConflictLens
      ? zoneBriefings.find((briefing) => briefing.lensId === activeConflictLens.id)
      : null;
    return mergeZoneBriefing(derived, override);
  }, [activeConflictLens, alerts, historicalAlerts, zoneBriefings]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    const removeLensOverlay = () => {
      if (conflictHotspotLayerRef.current) {
        map.removeLayer(conflictHotspotLayerRef.current);
        conflictHotspotLayerRef.current = null;
      }
    };

    if (!activeConflictLens) {
      removeLensOverlay();
      return;
    }

    removeLensOverlay();

    const hotspotLayer = L.layerGroup();
    activeConflictBrief?.hotspots.forEach((hotspot) => {
      const marker = L.circleMarker([hotspot.lat, hotspot.lng], {
        radius: Math.max(6, Math.min(14, 5 + hotspot.eventCount / 4)),
        color: "#f97316",
        weight: 1,
        fillColor: "#fb923c",
        fillOpacity: 0.28,
      });
      marker.bindTooltip(
        `${escapeHtml(hotspot.label)}<br/>${hotspot.eventCount} events in current brief`,
        { className: "siem-tooltip", direction: "top", offset: L.point(0, -6) },
      );
      hotspotLayer.addLayer(marker);
    });
    hotspotLayer.addTo(map);
    conflictHotspotLayerRef.current = hotspotLayer;

    return () => {
      removeLensOverlay();
    };
  }, [activeConflictBrief, activeConflictLens]);

  /* ── Render ───────────────────────────────────────────────────── */

  return (
    <section className="relative flex h-full flex-col overflow-hidden rounded-[1.6rem] border border-siem-border bg-siem-panel/90 shadow-[0_20px_64px_rgba(0,0,0,0.24)]">
      {/* Header bar */}
      <div className="z-10 flex items-start justify-between p-4">
        <div className="rounded-2xl border border-siem-border bg-siem-panel px-4 py-3">
          <div className="text-xxs uppercase tracking-[0.18em] text-siem-muted">
            {activeConflictLens ? "Conflict lens" : "Theatre"}
          </div>
          <div className="mt-1 text-lg font-semibold text-siem-text">
            {activeConflictLens
              ? `${activeConflictLens.label} lens`
              : regionFilter === "all"
                ? "Global operating picture"
                : regionFilter.startsWith("country:")
                  ? `${regionFilter.slice(8)} operating picture`
                  : `${regionFilter} operating picture`}
          </div>
          <div className="mt-1 text-sm text-siem-muted">
            {activeConflictLens
              ? activeConflictLens.description
              : `${visibleNowAlerts.length + visibleHistoryAlertsRendered.length} visible alerts across official feeds`}
          </div>
          {activeConflictLens && (
            <div className="mt-2 text-2xs uppercase tracking-[0.14em] text-siem-muted">
              {visibleNowAlerts.length + visibleHistoryAlertsRendered.length} visible alerts in current lens
            </div>
          )}
        </div>

        <div className="flex flex-col gap-2">
          <div className="rounded-2xl border border-siem-border bg-siem-panel p-2 space-y-1.5">
          <div className="grid grid-cols-2 gap-1.5 md:grid-cols-5">
            {["Europe", "Africa", "North America", "Asia", "all"].map((region) => (
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
          <div className="grid grid-cols-5 gap-1 md:grid-cols-9">
            {sortedOverlayDefs.map((overlay) => (
              <button
                key={overlay.id}
                type="button"
                onClick={() => toggleOverlay(overlay.id)}
                className={`flex flex-col items-center justify-center gap-0.5 rounded-md border px-1 py-0.5 uppercase leading-tight tracking-[0.12em] text-center transition-colors ${
                  activeOverlays.has(overlay.id)
                    ? "border-siem-accent/40 bg-siem-accent/12"
                    : "border-siem-border bg-siem-panel-strong hover:border-siem-accent/30 hover:bg-siem-accent/8"
                }`}
                style={{ fontSize: "10px", color: overlay.color, opacity: activeOverlays.has(overlay.id) ? 1 : 0.5 }}
              >
                <span>{overlay.label.split(" ")[0]}</span>
                <span>{overlay.label.split(" ").slice(1).join(" ")}</span>
              </button>
            ))}
          </div>
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

          {activeConflictBrief && (
            <div className="absolute bottom-3 left-3 z-[1000] w-[23rem] max-w-[calc(100%-1.5rem)] rounded-xl border border-siem-border/80 bg-siem-panel/95 p-3 text-2xs text-siem-text shadow-[0_12px_28px_rgba(0,0,0,0.35)] backdrop-blur-sm">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <div className="text-2xs uppercase tracking-[0.14em] text-siem-muted">Zone brief</div>
                  <div className="mt-1 text-2xs">{activeConflictBrief.lens.label}</div>
                </div>
              </div>
              <div className="mt-3 grid grid-cols-3 gap-2">
                {activeConflictBrief.metrics.slice(0, 6).map((metric) => (
                  <div key={metric.label} className="rounded border border-siem-border bg-white/5 px-2 py-1.5">
                    <div className="text-[10px] uppercase tracking-[0.12em] text-siem-muted">{metric.label}</div>
                    <div className="mt-1 font-mono text-2xs text-siem-text">{metric.value}</div>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5">
                {activeConflictBrief.topCountries.map((item) => (
                  <span key={item.code} className="inline-flex items-center gap-1 rounded-full border border-siem-border bg-white/5 px-2 py-1 text-2xs">
                    <span>{item.label}</span>
                    <span className="text-siem-muted">{item.count}</span>
                  </span>
                ))}
              </div>
              {activeConflictBrief.recentEvents && activeConflictBrief.recentEvents.length > 0 && (
                <div className="mt-3 space-y-1">
                  <div className="text-[10px] uppercase tracking-[0.12em] text-siem-muted">Latest events</div>
                  {activeConflictBrief.recentEvents.slice(0, 3).map((event, idx) => (
                    <a
                      key={`${event.title}-${event.date}-${idx}`}
                      href={event.url || "#"}
                      target={event.url ? "_blank" : undefined}
                      rel={event.url ? "noreferrer" : undefined}
                      className="block rounded border border-siem-border bg-white/5 px-2 py-1.5 hover:bg-siem-accent/8"
                    >
                      <div className="text-2xs text-siem-text line-clamp-2">{event.title}</div>
                      <div className="mt-0.5 text-2xs text-siem-muted">
                        {event.date ? new Date(event.date).toISOString().slice(0, 10) : "n/a"}
                        {event.location ? ` · ${event.location}` : ""}
                      </div>
                    </a>
                  ))}
                </div>
              )}
            </div>
          )}

          {regionFilter.startsWith("country:") && areaGroups.length > 0 && (
            <div className="absolute left-3 top-3 z-[1000] w-[32rem] max-w-[calc(100%-6rem)] rounded-xl border border-siem-border/80 bg-siem-panel/95 p-3 text-xs text-siem-text shadow-[0_12px_28px_rgba(0,0,0,0.35)] backdrop-blur-sm">
              <div className="flex items-center justify-between gap-2">
                <div className="text-2xs uppercase tracking-[0.14em] text-siem-muted">Country Alarm Table</div>
                <button
                  type="button"
                  onClick={() => onRegionChange(countryTableReturnRegionRef.current || "all")}
                  className="inline-flex h-6 w-6 items-center justify-center rounded-md border border-siem-border bg-siem-panel-strong text-siem-muted hover:border-siem-accent/40 hover:bg-siem-accent/10 hover:text-siem-text transition-colors"
                  aria-label="Close country alarm table"
                  title="Close"
                >
                  ×
                </button>
              </div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {areaGroups.map((group) => (
                  <button
                    key={group.id}
                    type="button"
                    onClick={() => setActiveAreaGroupID(group.id)}
                    className={`rounded-md border px-2 py-1 text-2xs uppercase tracking-[0.12em] transition-colors ${
                      activeAreaGroupID === group.id
                        ? "border-siem-accent/45 bg-siem-accent/16 text-siem-text"
                        : "border-siem-border bg-siem-panel-strong text-siem-muted hover:border-siem-accent/35 hover:bg-siem-accent/8"
                    }`}
                  >
                    {group.label} ({group.count})
                  </button>
                ))}
              </div>
              <div className="mt-2 max-h-56 overflow-y-auto rounded-lg border border-siem-border bg-siem-panel-strong">
                {activeAreaGroup?.alerts.map((alert) => (
                  <button
                    key={alert.alert_id}
                    type="button"
                    onClick={() => onSelect(alert.alert_id)}
                    className="grid w-full grid-cols-[4rem_3.5rem_1fr] gap-2 border-b border-siem-border/60 px-2 py-1.5 text-left last:border-b-0 hover:bg-siem-accent/8"
                  >
                    <span className="text-2xs uppercase tracking-[0.12em] text-siem-muted">{alert.severity}</span>
                    <span className="text-2xs text-siem-muted">{alert.event_country_code || alert.source.country_code}</span>
                    <span className="truncate text-xs text-siem-text">{alert.title}</span>
                  </button>
                ))}
                {!activeAreaGroup && (
                  <div className="px-2 py-2 text-2xs text-siem-muted">No alarms in selected group.</div>
                )}
              </div>
            </div>
          )}
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
                    onClick={() => {
                      countryFocusFromSpikeRef.current = spike.countryCode.toUpperCase();
                      countryTableReturnRegionRef.current = regionFilter.startsWith("country:")
                        ? lastNonCountryRegionRef.current
                        : regionFilter;
                      onRegionChange(`country:${spike.countryCode}`);
                    }}
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
