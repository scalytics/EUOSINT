import { useEffect, useRef } from "react";
import type { GeoJsonObject } from "geojson";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import type { FeatureCollection, MapLayer } from "@/agentops/lib/api-client/types";

interface Props {
  bbox: string;
  layers: MapLayer[];
  features: FeatureCollection;
  selectedTypes: string[];
  onBBoxChange: (bbox: string) => void;
  onTypesChange: (types: string[]) => void;
}

const DEFAULT_CENTER: L.LatLngExpression = [35.8997, 14.5146];

function bboxFromBounds(bounds: L.LatLngBounds): string {
  const southWest = bounds.getSouthWest();
  const northEast = bounds.getNorthEast();
  return [southWest.lng, southWest.lat, northEast.lng, northEast.lat].map((value) => value.toFixed(5)).join(",");
}

export function RuntimeMap({ bbox, layers, features, selectedTypes, onBBoxChange, onTypesChange }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<L.Map | null>(null);
  const featureLayerRef = useRef<L.GeoJSON | null>(null);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    const map = L.map(containerRef.current, {
      zoomControl: true,
      attributionControl: true,
    }).setView(DEFAULT_CENTER, 7);
    L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
      attribution: "&copy; OpenStreetMap contributors",
      maxZoom: 19,
    }).addTo(map);
    map.on("moveend", () => onBBoxChange(bboxFromBounds(map.getBounds())));
    featureLayerRef.current = L.geoJSON(undefined, {
      style: () => ({
        color: "#38bdf8",
        weight: 2,
        fillOpacity: 0.16,
      }),
      pointToLayer: (_feature, latlng) =>
        L.circleMarker(latlng, {
          radius: 6,
          color: "#f59e0b",
          weight: 1.5,
          fillColor: "#f97316",
          fillOpacity: 0.85,
        }),
      onEachFeature: (feature, layer) => {
        const props = (feature.properties ?? {}) as Record<string, unknown>;
        const lines = [String(props.display_name ?? props.id ?? "entity"), String(props.type ?? "")].filter(Boolean);
        if (lines.length > 0) {
          layer.bindPopup(lines.join("<br/>"));
        }
      },
    }).addTo(map);
    mapRef.current = map;
    onBBoxChange(bboxFromBounds(map.getBounds()));
    return () => {
      featureLayerRef.current?.remove();
      map.remove();
      mapRef.current = null;
      featureLayerRef.current = null;
    };
  }, [onBBoxChange]);

  useEffect(() => {
    if (!featureLayerRef.current) return;
    featureLayerRef.current.clearLayers();
    featureLayerRef.current.addData(features as unknown as GeoJsonObject);
  }, [features]);

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap gap-2 text-[11px] text-siem-muted">
        <button
          type="button"
          onClick={() => onTypesChange([])}
          className={`rounded-full border px-3 py-1 uppercase tracking-[0.18em] ${selectedTypes.length === 0 ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border"}`}
        >
          all
        </button>
        {layers.map((layer) => {
          const active = selectedTypes.includes(layer.id);
          return (
            <button
              key={layer.id}
              type="button"
              onClick={() => onTypesChange(active ? selectedTypes.filter((item) => item !== layer.id) : [...selectedTypes, layer.id])}
              className={`rounded-full border px-3 py-1 uppercase tracking-[0.18em] ${active ? "border-siem-accent/50 bg-siem-accent/14 text-siem-text" : "border-siem-border"}`}
            >
              {layer.name}
            </button>
          );
        })}
      </div>
      <div className="rounded-2xl border border-siem-border bg-siem-panel/55 p-2">
        <div ref={containerRef} className="h-[320px] w-full overflow-hidden rounded-[18px]" />
      </div>
      <div className="flex items-center justify-between gap-3 text-xs text-siem-muted">
        <span>bbox {bbox}</span>
        <span>{features.features.length} features</span>
      </div>
    </div>
  );
}
