/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import * as THREE from "three";
import * as topojson from "topojson-client";
import type { Topology } from "topojson-specification";
import type { Alert } from "@/types/alert";
import { categoryLabels, severityColors } from "@/lib/severity";
import { alertMatchesRegionFilter, latLngToRegion } from "@/lib/regions";

const WORLD_TOPO_URL =
  "https://cdn.jsdelivr.net/npm/world-atlas@2/countries-110m.json";
const US_STATES_GEOJSON_URL =
  "https://raw.githubusercontent.com/PublicaMundi/MappingAPI/master/data/geojson/us-states.json";
const AUTO_ROTATE_SPEED = (Math.PI * 2) / 3600; // ~1 full rotation per hour
const AUTO_RESUME_EASE = 1.6;
const MOMENTUM_FRICTION = 2.6;
const ZOOM_MIN = 1.03;
const ZOOM_MAX = 4.6;
const ZOOM_STEP = 0.06;
const PINCH_ZOOM_SENSITIVITY = 0.0027;
const PINCH_ZOOM_STEP_CAP = 0.16;
const ZOOM_EASE = 7.5;
const INITIAL_TILT_X = 0.06;
const DRAG_SENSITIVITY_MIN = 0.0016;
const DRAG_SENSITIVITY_MAX = 0.005;
const HOTSPOT_RADIUS_KM = 260;
const HOTSPOT_MIN_ALERTS = 2;
const HOTSPOT_PIXEL_RADIUS = 22;
const CLICK_DRAG_THRESHOLD_PX = 4;
const REGION_CENTROIDS: Record<string, { lat: number; lng: number; scale: number }> = {
  "North America": { lat: 45, lng: -100, scale: 1.42 },
  "South America": { lat: -15, lng: -60, scale: 1.2 },
  Europe: { lat: 52, lng: 15, scale: 0.96 },
  Africa: { lat: 5, lng: 20, scale: 1.28 },
  Asia: { lat: 34, lng: 95, scale: 1.62 },
  Oceania: { lat: -22, lng: 140, scale: 1.08 },
  Caribbean: { lat: 20, lng: -86, scale: 0.9 },
  International: { lat: 20, lng: 0, scale: 1.8 },
};

interface Props {
  alerts: Alert[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  regionFilter: string;
  onRegionChange: (region: string) => void;
  visibleAlertIds: string[];
}

function haversineKm(aLat: number, aLng: number, bLat: number, bLng: number): number {
  const toRad = (d: number) => (d * Math.PI) / 180;
  const dLat = toRad(bLat - aLat);
  const dLng = toRad(bLng - aLng);
  const lat1 = toRad(aLat);
  const lat2 = toRad(bLat);
  const sinDLat = Math.sin(dLat / 2);
  const sinDLng = Math.sin(dLng / 2);
  const x =
    sinDLat * sinDLat +
    Math.cos(lat1) * Math.cos(lat2) * sinDLng * sinDLng;
  return 6371 * 2 * Math.atan2(Math.sqrt(x), Math.sqrt(1 - x));
}

function latLngToVector3(
  lat: number,
  lng: number,
  radius: number
): THREE.Vector3 {
  const phi = (90 - lat) * (Math.PI / 180);
  const theta = (lng + 180) * (Math.PI / 180);
  return new THREE.Vector3(
    -radius * Math.sin(phi) * Math.cos(theta),
    radius * Math.cos(phi),
    radius * Math.sin(phi) * Math.sin(theta)
  );
}

function vector3ToLatLng(vec: THREE.Vector3): { lat: number; lng: number } {
  const v = vec.clone().normalize();
  const phi = Math.acos(v.y);
  const theta = Math.atan2(v.z, v.x);
  const lat = 90 - (phi * 180) / Math.PI;
  // Negate longitude to match latLngToVector3's inverted x-axis (x = -r*sin*cos)
  const lng = -(((theta * 180) / Math.PI + 540) % 360 - 180);
  return { lat, lng };
}

function createGlowTexture(): THREE.CanvasTexture {
  const size = 256;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    const fallback = new THREE.CanvasTexture(canvas);
    fallback.needsUpdate = true;
    return fallback;
  }
  const image = ctx.createImageData(size, size);
  const data = image.data;
  const center = size / 2;
  const maxR = size / 2;
  for (let y = 0; y < size; y += 1) {
    for (let x = 0; x < size; x += 1) {
      const dx = x - center;
      const dy = y - center;
      const r = Math.sqrt(dx * dx + dy * dy) / maxR;
      const idx = (y * size + x) * 4;

      // Gaussian-like continuous fade (no hard stop bands).
      const alpha =
        r >= 1
          ? 0
          : Math.exp(-3.8 * r * r) * Math.pow(1 - r, 0.75);
      const a = Math.max(0, Math.min(255, Math.round(alpha * 255)));
      data[idx] = 255;
      data[idx + 1] = 255;
      data[idx + 2] = 255;
      data[idx + 3] = a;
    }
  }
  ctx.putImageData(image, 0, 0);

  const texture = new THREE.CanvasTexture(canvas);
  texture.minFilter = THREE.LinearFilter;
  texture.magFilter = THREE.LinearFilter;
  texture.generateMipmaps = false;
  texture.needsUpdate = true;
  return texture;
}

/** Convert a GeoJSON MultiLineString / Polygon ring coords into THREE.Line segments */
function coordsToLine(
  coords: number[][],
  radius: number,
  material: THREE.LineBasicMaterial,
  group: THREE.Group
) {
  const points: THREE.Vector3[] = [];
  for (const [lng, lat] of coords) {
    points.push(latLngToVector3(lat, lng, radius));
  }
  if (points.length < 2) return;
  const geometry = new THREE.BufferGeometry().setFromPoints(points);
  const line = new THREE.Line(geometry, material);
  group.add(line);
}

function drawGeoJson(
  geojson: GeoJSON.FeatureCollection,
  radius: number,
  material: THREE.LineBasicMaterial,
  group: THREE.Group
) {
  for (const feature of geojson.features) {
    const geom = feature.geometry;
    if (geom.type === "Polygon") {
      for (const ring of geom.coordinates) {
        coordsToLine(ring, radius, material, group);
      }
    } else if (geom.type === "MultiPolygon") {
      for (const polygon of geom.coordinates) {
        for (const ring of polygon) {
          coordsToLine(ring, radius, material, group);
        }
      }
    }
  }
}

function createSurfaceTexture(landGeo: GeoJSON.FeatureCollection): THREE.CanvasTexture {
  const width = 2048;
  const height = 1024;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    const fallback = new THREE.CanvasTexture(canvas);
    fallback.needsUpdate = true;
    return fallback;
  }

  // Water is slightly lighter black-blue; land remains darker.
  ctx.fillStyle = "#122033";
  ctx.fillRect(0, 0, width, height);
  ctx.fillStyle = "#070d18";

  const project = (lng: number, lat: number) => ({
    x: ((lng + 180) / 360) * width,
    y: ((90 - lat) / 180) * height,
  });

  for (const feature of landGeo.features) {
    const geom = feature.geometry;
    if (!geom) continue;
    const polygons =
      geom.type === "Polygon"
        ? [geom.coordinates]
        : geom.type === "MultiPolygon"
        ? geom.coordinates
        : [];
    if (polygons.length === 0) continue;

    ctx.beginPath();
    for (const polygon of polygons) {
      for (const ring of polygon) {
        if (ring.length === 0) continue;
        const start = project(ring[0][0], ring[0][1]);
        ctx.moveTo(start.x, start.y);
        for (let i = 1; i < ring.length; i += 1) {
          const p = project(ring[i][0], ring[i][1]);
          ctx.lineTo(p.x, p.y);
        }
        ctx.closePath();
      }
    }
    ctx.fill("evenodd");
  }

  const texture = new THREE.CanvasTexture(canvas);
  texture.minFilter = THREE.LinearFilter;
  texture.magFilter = THREE.LinearFilter;
  texture.generateMipmaps = false;
  texture.needsUpdate = true;
  return texture;
}

export function GlobeView({
  alerts,
  selectedId,
  onSelect,
  regionFilter,
  onRegionChange,
  visibleAlertIds,
}: Props) {
  const [hotspot, setHotspot] = useState<{ lat: number; lng: number } | null>(null);
  const [hotspotAlertIds, setHotspotAlertIds] = useState<string[]>([]);
  const [hotspotTypeFilter, setHotspotTypeFilter] = useState<string>("all");
  const [hotspotAgencyFilter, setHotspotAgencyFilter] = useState<string>("all");
  const visibleIdSet = useMemo(() => new Set(visibleAlertIds), [visibleAlertIds]);
  const alertsById = useMemo(
    () => new Map(alerts.map((a) => [a.alert_id, a])),
    [alerts]
  );
  const closeHotspot = () => {
    setHotspot(null);
    setHotspotAlertIds([]);
  };
  const hotspotAlerts = useMemo(() => {
    if (!hotspot || hotspotAlertIds.length === 0) return [];
    return hotspotAlertIds
      .map((id) => alertsById.get(id))
      .filter((a): a is Alert => Boolean(a))
      .filter((a) => visibleIdSet.has(a.alert_id))
      .sort(
        (a, b) => new Date(b.first_seen).getTime() - new Date(a.first_seen).getTime()
      );
  }, [alertsById, hotspot, hotspotAlertIds, visibleIdSet]);
  const hotspotTypeOptions = useMemo(
    () => [...new Set(hotspotAlerts.map((a) => a.category))],
    [hotspotAlerts]
  );
  const hotspotAgencyOptions = useMemo(
    () => [...new Set(hotspotAlerts.map((a) => a.source.authority_name))].sort(),
    [hotspotAlerts]
  );
  const hotspotFiltered = useMemo(() => {
    return hotspotAlerts.filter((a) => {
      const typeOk = hotspotTypeFilter === "all" || a.category === hotspotTypeFilter;
      const agencyOk =
        hotspotAgencyFilter === "all" || a.source.authority_name === hotspotAgencyFilter;
      return typeOk && agencyOk;
    });
  }, [hotspotAlerts, hotspotAgencyFilter, hotspotTypeFilter]);
  const regions = useMemo(() => {
    const counts = new Map<string, number>();
    alerts.forEach((a) => {
      const r = a.source.region;
      counts.set(r, (counts.get(r) ?? 0) + 1);
    });
    return [...counts.entries()].sort((a, b) => b[1] - a[1]);
  }, [alerts]);
  const containerRef = useRef<HTMLDivElement>(null);
  const rendererRef = useRef<THREE.WebGLRenderer | null>(null);
  const globeGroupRef = useRef<THREE.Group | null>(null);
  const oceanRef = useRef<THREE.Mesh | null>(null);
  const regionHighlightRef = useRef<THREE.Mesh | null>(null);
  const pointMeshesRef = useRef<Map<string, THREE.Mesh>>(new Map());
  const frameRef = useRef<number>(0);
  const mouseRef = useRef({
    isDown: false,
    prevX: 0,
    prevY: 0,
    prevT: 0,
    dragDistance: 0,
    moved: false,
  });
  const zoomRef = useRef({ current: 3.2, target: 3.2 });
  const rotationRef = useRef({
    autoRotate: true,
    x: INITIAL_TILT_X,
    y: 0,
    velX: 0,
    velY: 0,
    interacting: false,
    ambientBlend: 1,
  });
  const regionFilterRef = useRef(regionFilter);
  const regionsRef = useRef<Set<string>>(new Set(regions.map(([r]) => r)));
  const selectedIdRef = useRef<string | null>(selectedId);
  const visibleAlertIdsRef = useRef<Set<string>>(new Set(visibleAlertIds));
  const adjustZoom = (delta: number) => {
    const next = zoomRef.current.target + delta;
    zoomRef.current.target = Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, next));
  };

  useEffect(() => {
    regionFilterRef.current = regionFilter;
  }, [regionFilter]);

  useEffect(() => {
    regionsRef.current = new Set(regions.map(([r]) => r));
  }, [regions]);

  useEffect(() => {
    selectedIdRef.current = selectedId;
  }, [selectedId]);

  useEffect(() => {
    visibleAlertIdsRef.current = new Set(visibleAlertIds);
  }, [visibleAlertIds]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    // --- Scene ---
    const scene = new THREE.Scene();

    // --- Camera ---
    const camera = new THREE.PerspectiveCamera(
      45,
      container.clientWidth / container.clientHeight,
      0.005,
      1000
    );
    camera.position.z = zoomRef.current.current;

    // --- Renderer ---
    const renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(container.clientWidth, container.clientHeight);
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.setClearColor(0x0a0e17, 1);
    container.appendChild(renderer.domElement);
    rendererRef.current = renderer;

    // --- Globe group (everything rotates together) ---
    const globeGroup = new THREE.Group();
    globeGroupRef.current = globeGroup;
    scene.add(globeGroup);

    // --- Ocean sphere ---
    const oceanGeo = new THREE.SphereGeometry(0.995, 64, 64);
    const oceanMat = new THREE.MeshPhongMaterial({
      color: 0xffffff,
      emissive: 0x050a14,
      shininess: 5,
    });
    const oceanMesh = new THREE.Mesh(oceanGeo, oceanMat);
    globeGroup.add(oceanMesh);
    oceanRef.current = oceanMesh;

    // --- Atmosphere glow ---
    const glowGeo = new THREE.SphereGeometry(1.04, 64, 64);
    const glowMat = new THREE.ShaderMaterial({
      vertexShader: `
        varying vec3 vNormal;
        void main() {
          vNormal = normalize(normalMatrix * normal);
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }
      `,
      fragmentShader: `
        varying vec3 vNormal;
        void main() {
          float intensity = pow(max(0.0, 0.45 - dot(vNormal, vec3(0.0, 0.0, 1.0))), 2.0);
          gl_FragColor = vec4(0.12, 0.32, 0.62, intensity * 0.14);
        }
      `,
      blending: THREE.AdditiveBlending,
      side: THREE.BackSide,
      transparent: true,
    });
    globeGroup.add(new THREE.Mesh(glowGeo, glowMat));

    // --- Graticule (lat/lng grid) ---
    const gratMat = new THREE.LineBasicMaterial({
      color: 0x112244,
      transparent: true,
      opacity: 0.12,
    });
    // Latitude lines every 30 degrees
    for (let lat = -60; lat <= 60; lat += 30) {
      const pts: THREE.Vector3[] = [];
      for (let lng = -180; lng <= 180; lng += 2) {
        pts.push(latLngToVector3(lat, lng, 1.0));
      }
      const g = new THREE.BufferGeometry().setFromPoints(pts);
      globeGroup.add(new THREE.Line(g, gratMat));
    }
    // Longitude lines every 30 degrees
    for (let lng = -180; lng < 180; lng += 30) {
      const pts: THREE.Vector3[] = [];
      for (let lat = -90; lat <= 90; lat += 2) {
        pts.push(latLngToVector3(lat, lng, 1.0));
      }
      const g = new THREE.BufferGeometry().setFromPoints(pts);
      globeGroup.add(new THREE.Line(g, gratMat));
    }

    // --- Lights ---
    scene.add(new THREE.AmbientLight(0x445566, 2));
    const pLight = new THREE.PointLight(0x3b82f6, 3, 12);
    pLight.position.set(3, 2, 4);
    scene.add(pLight);
    const pLight2 = new THREE.PointLight(0x1e40af, 1.5, 12);
    pLight2.position.set(-3, -1, 3);
    scene.add(pLight2);

    // --- Load world map (country boundaries) ---
    let surfaceTexture: THREE.CanvasTexture | null = null;
    fetch(WORLD_TOPO_URL)
      .then((r) => r.json())
      .then((topoData: Topology) => {
        const countries = topojson.feature(
          topoData,
          topoData.objects.countries
        ) as unknown as GeoJSON.FeatureCollection;

        // Country border lines
        const borderMat = new THREE.LineBasicMaterial({
          color: 0x1a6aaa,
          transparent: true,
          opacity: 0.55,
        });
        drawGeoJson(countries, 1.001, borderMat, globeGroup);

        // Coastline mesh (from the same topology)
        if (topoData.objects.land) {
          const land = topojson.feature(
            topoData,
            topoData.objects.land
          ) as unknown as GeoJSON.FeatureCollection;
          surfaceTexture = createSurfaceTexture(land);
          oceanMat.map = surfaceTexture;
          oceanMat.needsUpdate = true;
          const coastMat = new THREE.LineBasicMaterial({
            color: 0x2488cc,
            transparent: true,
            opacity: 0.7,
          });
          drawGeoJson(land, 1.002, coastMat, globeGroup);
        }
      })
      .catch((err) => console.warn("Failed to load world map:", err));

    // --- U.S. state boundaries (visible mainly when zoomed in) ---
    const usStateGroup = new THREE.Group();
    globeGroup.add(usStateGroup);
    const usStateMat = new THREE.LineBasicMaterial({
      color: 0x8eb7cf,
      transparent: true,
      opacity: 0,
    });
    fetch(US_STATES_GEOJSON_URL)
      .then((r) => r.json())
      .then((geo: GeoJSON.FeatureCollection) => {
        drawGeoJson(geo, 1.0032, usStateMat, usStateGroup);
      })
      .catch((err) => console.warn("Failed to load US states:", err));

    // --- Alert points ---
    const pointMeshes = new Map<string, THREE.Mesh>();
    const glowTexture = createGlowTexture();
    const glowMeshes: THREE.Mesh[] = [];
    const allDotMeshes: {
      mesh: THREE.Mesh;
      severity: string;
      alertId: string;
      baseGlow: number;
      baseDotOpacity: number;
    }[] = [];
    alerts.forEach((alert, idx) => {
      const pos = latLngToVector3(alert.lat, alert.lng, 1.02);
      const isInformational = alert.severity === "info" || alert.category === "informational";
      const size =
        alert.severity === "critical"
          ? 0.018
          : alert.severity === "high"
          ? 0.014
          : isInformational
          ? 0.009
          : 0.011;
      const baseDotOpacity = isInformational ? 0.46 : 0.95;
      const baseCoreOpacity = isInformational ? 0.42 : 0.85;
      const baseGlowOpacity = isInformational ? 0.06 : 0.2;
      const color = new THREE.Color(severityColors[alert.severity]);
      const glowColor = color.clone().lerp(new THREE.Color(0xfff3cc), 0.78);

      // Core dot
      const dotGeo = new THREE.SphereGeometry(size, 16, 16);
      const dotMat = new THREE.MeshPhongMaterial({
        color,
        emissive: color.clone().multiplyScalar(0.5),
        shininess: 85,
        specular: new THREE.Color(0xffffff),
        transparent: true,
        opacity: baseDotOpacity,
        depthWrite: false,
      });
      const dotMesh = new THREE.Mesh(dotGeo, dotMat);
      dotMesh.position.copy(pos);
      dotMesh.userData = { alertId: alert.alert_id, region: alert.source.region };
      globeGroup.add(dotMesh);
      pointMeshes.set(alert.alert_id, dotMesh);

      // Bright inner center for a crisp, modern marker look
      const coreGeo = new THREE.SphereGeometry(size * 0.42, 10, 10);
      const coreMat = new THREE.MeshBasicMaterial({
        color: 0xffffff,
        transparent: true,
        opacity: baseCoreOpacity,
      });
      const coreMesh = new THREE.Mesh(coreGeo, coreMat);
      dotMesh.add(coreMesh);

      // Surface-hugging glow (tangent to globe) to avoid "dome" billboard look
      const glowM = new THREE.MeshBasicMaterial({
        map: glowTexture,
        color: glowColor,
        transparent: true,
        opacity: baseGlowOpacity,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
        side: THREE.DoubleSide,
      });
      const gm = new THREE.Mesh(new THREE.PlaneGeometry(1, 1), glowM);
      gm.position.copy(pos);
      const baseGlow = size * 6;
      gm.scale.set(baseGlow, baseGlow, 1);
      const normal = pos.clone().normalize();
      gm.quaternion.setFromUnitVectors(new THREE.Vector3(0, 0, 1), normal);
      gm.userData = {
        phase: idx * 0.8,
        region: alert.source.region,
        baseGlow,
        baseGlowOpacity,
        alertId: alert.alert_id,
      };
      globeGroup.add(gm);
      glowMeshes.push(gm);
      allDotMeshes.push({
        mesh: dotMesh,
        severity: alert.severity,
        alertId: alert.alert_id,
        baseGlow,
        baseDotOpacity,
      });
    });
    pointMeshesRef.current = pointMeshes;

    // --- Region spotlight (continent highlight on filter selection) ---
    const regionHighlightMat = new THREE.MeshBasicMaterial({
      map: glowTexture,
      color: new THREE.Color(0x66c4ff),
      transparent: true,
      opacity: 0.32,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
      side: THREE.DoubleSide,
    });
    const regionHighlight = new THREE.Mesh(
      new THREE.PlaneGeometry(1, 1),
      regionHighlightMat
    );
    regionHighlight.visible = false;
    globeGroup.add(regionHighlight);
    regionHighlightRef.current = regionHighlight;

    // --- Initial tilt (persisted) ---
    globeGroup.rotation.x = rotationRef.current.x;
    globeGroup.rotation.y = rotationRef.current.y;

    // --- Animate ---
    let t = 0;
    let last = performance.now();
    const animate = () => {
      frameRef.current = requestAnimationFrame(animate);
      const now = performance.now();
      const delta = (now - last) / 1000;
      last = now;
      t += delta;
      if (!rotationRef.current.interacting) {
        const damping = Math.exp(-MOMENTUM_FRICTION * delta);
        rotationRef.current.velX *= damping;
        rotationRef.current.velY *= damping;
        rotationRef.current.ambientBlend +=
          (1 - rotationRef.current.ambientBlend) * Math.min(1, delta * AUTO_RESUME_EASE);
        const ambient = AUTO_ROTATE_SPEED * rotationRef.current.ambientBlend;
        globeGroup.rotation.x += rotationRef.current.velX * delta;
        globeGroup.rotation.y += (rotationRef.current.velY + ambient) * delta;
      }
      globeGroup.rotation.x = Math.max(-1.2, Math.min(1.2, globeGroup.rotation.x));
      const zoomEase = Math.min(1, delta * ZOOM_EASE);
      zoomRef.current.current +=
        (zoomRef.current.target - zoomRef.current.current) * zoomEase;
      camera.position.z = zoomRef.current.current;
      rotationRef.current.x = globeGroup.rotation.x;
      rotationRef.current.y = globeGroup.rotation.y;

      const activeRegion = regionFilterRef.current;
      const selected = selectedIdRef.current;
      const zoomLevel = zoomRef.current.current;
      const zoomNorm = Math.max(
        0,
        Math.min(1, (zoomLevel - ZOOM_MIN) / (ZOOM_MAX - ZOOM_MIN))
      );
      const dotScaleBase = 0.42 + zoomNorm * 0.68; // smaller when zoomed in
      const glowScaleBase = 0.22 + Math.pow(zoomNorm, 1.4) * 0.78; // stronger shrink when zoomed in
      const glowOpacityScale = 0.35 + zoomNorm * 0.65;

      // Keep surface glows static so they read as city lights, not VFX
      // When "all" or a specific region is active, show dots based on region
      // match alone — don't let AlertFeed's internal filters hide globe dots.
      const showAll = activeRegion === "all";
      glowMeshes.forEach((gm) => {
        const alertId = (gm.userData as { alertId?: string }).alertId;
        const alert = alertId ? alertsById.get(alertId) : null;
        const isRegionVisible =
          showAll || !alert || alertMatchesRegionFilter(alert, activeRegion);
        gm.visible = isRegionVisible;
        if (!gm.visible) return;
        const { baseGlow } = gm.userData as {
          baseGlow: number;
          baseGlowOpacity?: number;
        };
        gm.scale.set(baseGlow * glowScaleBase, baseGlow * glowScaleBase, 1);
        const base = (gm.userData as { baseGlowOpacity?: number }).baseGlowOpacity ?? 0.13;
        (gm.material as THREE.MeshBasicMaterial).opacity = base * glowOpacityScale;
      });

      // Keep dots stable (no visible pulsing)
      allDotMeshes.forEach((entry) => {
        const alert = alertsById.get(entry.alertId);
        const isRegionVisible =
          showAll || !alert || alertMatchesRegionFilter(alert, activeRegion);
        entry.mesh.visible = isRegionVisible;
        if (!entry.mesh.visible) return;
        const selectedBoost = entry.alertId === selected ? 1.9 : 1;
        const dotScale = dotScaleBase * selectedBoost;
        entry.mesh.scale.set(dotScale, dotScale, dotScale);
        const mat = entry.mesh.material as THREE.MeshPhongMaterial;
        mat.opacity = selectedBoost > 1 ? 1 : entry.baseDotOpacity;
      });

      const regionHighlightMesh = regionHighlightRef.current;
      if (regionHighlightMesh) {
        const target = REGION_CENTROIDS[activeRegion];
        if (!target || activeRegion === "all") {
          regionHighlightMesh.visible = false;
        } else {
          const base = latLngToVector3(target.lat, target.lng, 1.012);
          regionHighlightMesh.visible = true;
          regionHighlightMesh.position.copy(base);
          const normal = base.clone().normalize();
          regionHighlightMesh.quaternion.setFromUnitVectors(
            new THREE.Vector3(0, 0, 1),
            normal
          );
          const pulse = 1 + Math.sin(t * 1.2) * 0.04;
          const scale = target.scale * pulse;
          regionHighlightMesh.scale.set(scale, scale, 1);
        }
      }

      // Fade in US state lines as user zooms closer to North America details.
      const stateFade = Math.max(0, Math.min(1, (2.45 - zoomLevel) / 0.9));
      usStateGroup.visible = stateFade > 0.01;
      usStateMat.opacity = 0.82 * stateFade;

      renderer.render(scene, camera);
    };
    animate();

    // --- Resize ---
    const onResize = () => {
      const w = container.clientWidth;
      const h = container.clientHeight;
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
      renderer.setSize(w, h);
    };
    const resizeObs = new ResizeObserver(onResize);
    resizeObs.observe(container);

    // --- Drag to rotate (pointer-first, touch fallback for older browsers) ---
    let activePointerId: number | null = null;
    const touchPointers = new Map<number, { x: number; y: number }>();
    let pinchDistance = 0;
    let fallbackPinchDistance = 0;
    const getDistance = (a: { x: number; y: number }, b: { x: number; y: number }) =>
      Math.hypot(b.x - a.x, b.y - a.y);
    const applyPinchDelta = (deltaPx: number) => {
      const pinchStep = Math.max(
        -PINCH_ZOOM_STEP_CAP,
        Math.min(PINCH_ZOOM_STEP_CAP, deltaPx * PINCH_ZOOM_SENSITIVITY)
      );
      // Finger spread (distance up) should zoom in, pinch (distance down) should zoom out.
      adjustZoom(-pinchStep);
      mouseRef.current.moved = true;
    };
    const stopInteracting = () => {
      mouseRef.current.isDown = false;
      rotationRef.current.interacting = false;
    };
    const beginDrag = (clientX: number, clientY: number) => {
      mouseRef.current = {
        isDown: true,
        prevX: clientX,
        prevY: clientY,
        prevT: performance.now(),
        dragDistance: 0,
        moved: false,
      };
      rotationRef.current.interacting = true;
      rotationRef.current.ambientBlend = 0;
    };
    const onDown = (e: PointerEvent) => {
      if (e.pointerType === "mouse" && e.button !== 0) return;
      if (e.pointerType === "touch") {
        touchPointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
        if (touchPointers.size >= 2) {
          const [a, b] = Array.from(touchPointers.values());
          pinchDistance = getDistance(a, b);
          activePointerId = null;
          stopInteracting();
          e.preventDefault();
          return;
        }
      }
      activePointerId = e.pointerId;
      beginDrag(e.clientX, e.clientY);
      // Pointer capture is useful for mouse drag continuity, but can interfere
      // with multi-touch pinch tracking on some touch browsers.
      if (e.pointerType !== "touch") {
        try {
          (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
        } catch {
          // Ignore capture errors on unsupported environments.
        }
      }
    };
    const applyDrag = (clientX: number, clientY: number) => {
      if (!mouseRef.current.isDown) return;
      const zoomNorm = Math.max(
        0,
        Math.min(1, (zoomRef.current.current - ZOOM_MIN) / (ZOOM_MAX - ZOOM_MIN))
      );
      const dragSensitivity =
        DRAG_SENSITIVITY_MIN +
        zoomNorm * (DRAG_SENSITIVITY_MAX - DRAG_SENSITIVITY_MIN);
      const dx = clientX - mouseRef.current.prevX;
      const dy = clientY - mouseRef.current.prevY;
      const now = performance.now();
      const dt = Math.max(0.001, (now - mouseRef.current.prevT) / 1000);
      const rotY = dx * dragSensitivity;
      const rotX = dy * dragSensitivity;
      globeGroup.rotation.y += rotY;
      globeGroup.rotation.x += rotX;
      globeGroup.rotation.x = Math.max(
        -1.2,
        Math.min(1.2, globeGroup.rotation.x)
      );
      const step = Math.hypot(dx, dy);
      mouseRef.current.dragDistance += step;
      if (mouseRef.current.dragDistance > CLICK_DRAG_THRESHOLD_PX) {
        mouseRef.current.moved = true;
      }
      const vY = rotY / dt;
      const vX = rotX / dt;
      rotationRef.current.velY = rotationRef.current.velY * 0.6 + vY * 0.4;
      rotationRef.current.velX = rotationRef.current.velX * 0.6 + vX * 0.4;
      mouseRef.current.prevX = clientX;
      mouseRef.current.prevY = clientY;
      mouseRef.current.prevT = now;
    };
    const onMove = (e: PointerEvent) => {
      if (e.pointerType === "touch") {
        if (touchPointers.has(e.pointerId)) {
          touchPointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
        }
        if (touchPointers.size >= 2) {
          const [a, b] = Array.from(touchPointers.values());
          const nextDistance = getDistance(a, b);
          if (pinchDistance > 0) {
            applyPinchDelta(nextDistance - pinchDistance);
          }
          pinchDistance = nextDistance;
          activePointerId = null;
          stopInteracting();
          e.preventDefault();
          return;
        }
        pinchDistance = 0;
        if (activePointerId !== null && e.pointerId !== activePointerId) return;
        if (mouseRef.current.isDown) {
          e.preventDefault();
        }
      } else if (activePointerId !== null && e.pointerId !== activePointerId) {
        return;
      }
      applyDrag(e.clientX, e.clientY);
    };
    const onUp = (e: PointerEvent) => {
      if (e.pointerType === "touch") {
        touchPointers.delete(e.pointerId);
        if (touchPointers.size < 2) {
          pinchDistance = 0;
        }
        if (activePointerId === e.pointerId) {
          activePointerId = null;
          stopInteracting();
        }
        if (touchPointers.size === 1 && activePointerId === null) {
          const [id, p] = Array.from(touchPointers.entries())[0];
          activePointerId = id;
          beginDrag(p.x, p.y);
          return;
        }
        if (touchPointers.size === 0) {
          stopInteracting();
        }
        return;
      }
      activePointerId = null;
      stopInteracting();
    };
    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length >= 2) {
        const t1 = e.touches[0];
        const t2 = e.touches[1];
        fallbackPinchDistance = Math.hypot(
          t2.clientX - t1.clientX,
          t2.clientY - t1.clientY
        );
        stopInteracting();
        e.preventDefault();
        return;
      }
      const t = e.touches[0];
      if (!t) return;
      e.preventDefault();
      beginDrag(t.clientX, t.clientY);
    };
    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length >= 2) {
        const t1 = e.touches[0];
        const t2 = e.touches[1];
        const nextDistance = Math.hypot(
          t2.clientX - t1.clientX,
          t2.clientY - t1.clientY
        );
        if (fallbackPinchDistance > 0) {
          applyPinchDelta(nextDistance - fallbackPinchDistance);
        }
        fallbackPinchDistance = nextDistance;
        stopInteracting();
        e.preventDefault();
        return;
      }
      if (!mouseRef.current.isDown) return;
      const t = e.touches[0];
      if (!t) return;
      e.preventDefault();
      applyDrag(t.clientX, t.clientY);
    };
    const onTouchEnd = (e: TouchEvent) => {
      if (e.touches.length >= 2) return;
      fallbackPinchDistance = 0;
      if (e.touches.length === 1) {
        const t = e.touches[0];
        beginDrag(t.clientX, t.clientY);
        return;
      }
      activePointerId = null;
      stopInteracting();
    };
    const onGesture = (e: Event) => {
      e.preventDefault();
    };
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const deltaScale =
        e.deltaMode === WheelEvent.DOM_DELTA_LINE
          ? 16
          : e.deltaMode === WheelEvent.DOM_DELTA_PAGE
          ? window.innerHeight
          : 1;
      const normalized = e.deltaY * deltaScale;
      if (normalized === 0) return;
      const dir = normalized > 0 ? 1 : -1;
      const baseIntensity = Math.max(0.4, Math.min(1.45, Math.abs(normalized) / 120));
      const zoomNorm = Math.max(
        0,
        Math.min(1, (zoomRef.current.current - ZOOM_MIN) / (ZOOM_MAX - ZOOM_MIN))
      );
      // Bell-curve + far-distance boost:
      // high sensitivity when far out, progressively finer near the globe.
      const sigma = 0.28;
      const center = 0.82;
      const bell = Math.exp(-((zoomNorm - center) ** 2) / (2 * sigma * sigma));
      const farBoost = Math.pow(zoomNorm, 2.35);
      const zoomSensitivity = 0.18 + bell * 1.45 + farBoost * 1.75;
      adjustZoom(dir * ZOOM_STEP * baseIntensity * zoomSensitivity);
    };

    // --- Click raycasting ---
    const raycaster = new THREE.Raycaster();
    const mv = new THREE.Vector2();
    const onClk = (e: MouseEvent) => {
      if (mouseRef.current.moved) {
        mouseRef.current.moved = false;
        return;
      }
      const rect = container.getBoundingClientRect();
      const clickPx = {
        x: e.clientX - rect.left,
        y: e.clientY - rect.top,
      };
      const visibleSet = visibleAlertIdsRef.current;
      const findNearbyVisibleAlertsAt = (lat: number, lng: number) =>
        alerts
          .filter((a) => visibleSet.has(a.alert_id))
          .filter((a) => haversineKm(a.lat, a.lng, lat, lng) <= HOTSPOT_RADIUS_KM);
      const findPixelClusterAt = (px: { x: number; y: number }, radiusPx: number) => {
        const r2 = radiusPx * radiusPx;
        return alerts.filter((a) => {
          if (!visibleSet.has(a.alert_id)) return false;
          const mesh = pointMeshes.get(a.alert_id);
          if (!mesh || !mesh.visible) return false;
          const p = mesh.getWorldPosition(new THREE.Vector3()).project(camera);
          if (p.z < -1 || p.z > 1) return false;
          const sx = ((p.x + 1) / 2) * rect.width;
          const sy = ((1 - p.y) / 2) * rect.height;
          const dx = sx - px.x;
          const dy = sy - px.y;
          return dx * dx + dy * dy <= r2;
        });
      };
      const openHotspotFromCluster = (cluster: Alert[]) => {
        if (cluster.length < HOTSPOT_MIN_ALERTS) return false;
        const uniqueIds = [...new Set(cluster.map((a) => a.alert_id))];
        if (uniqueIds.length < HOTSPOT_MIN_ALERTS) return false;
        const center = cluster.reduce(
          (acc, a) => ({ lat: acc.lat + a.lat, lng: acc.lng + a.lng }),
          { lat: 0, lng: 0 }
        );
        setHotspot({
          lat: center.lat / cluster.length,
          lng: center.lng / cluster.length,
        });
        setHotspotAlertIds(uniqueIds);
        setHotspotTypeFilter("all");
        setHotspotAgencyFilter("all");
        return true;
      };
      mv.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      mv.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
      raycaster.setFromCamera(mv, camera);
      const hits = raycaster.intersectObjects(Array.from(pointMeshes.values()));
      if (hits.length > 0) {
        const id = hits[0].object.userData.alertId;
        if (id) {
          onSelect(id);
          const baseAlert = alertsById.get(id);
          if (baseAlert) {
            const overlapped = findPixelClusterAt(clickPx, HOTSPOT_PIXEL_RADIUS * 1.3);
            const nearby = findNearbyVisibleAlertsAt(baseAlert.lat, baseAlert.lng);
            if (!openHotspotFromCluster(overlapped) && !openHotspotFromCluster(nearby)) {
              closeHotspot();
            }
          }
        }
        return;
      }
      const ocean = oceanRef.current;
      if (!ocean) return;
      const globeHits = raycaster.intersectObject(ocean);
      if (globeHits.length === 0) return;
      const hitPoint = globeHits[0].point;
      // Transform from world space to globe-local space to account for rotation
      const globeGroup = globeGroupRef.current;
      const localPoint = globeGroup
        ? globeGroup.worldToLocal(hitPoint.clone())
        : hitPoint;
      const { lat, lng } = vector3ToLatLng(localPoint);
      const overlapped = findPixelClusterAt(clickPx, HOTSPOT_PIXEL_RADIUS * 1.35);
      const nearby = findNearbyVisibleAlertsAt(lat, lng);
      if (!openHotspotFromCluster(overlapped) && !openHotspotFromCluster(nearby)) {
        closeHotspot();
      }
      // Resolve clicked point to a data-backed region
      const detectedRegion = latLngToRegion(lat, lng);
      if (!detectedRegion) {
        // Clicked ocean — reset to show all alerts
        if (regionFilterRef.current !== "all") onRegionChange("all");
        return;
      }
      // Only select regions that actually have alerts in the current dataset
      const dataRegions = regionsRef.current;
      const region = dataRegions.has(detectedRegion) ? detectedRegion : null;
      if (!region) return;
      const currentRegion = regionFilterRef.current;
      onRegionChange(currentRegion === region ? "all" : region);
    };

    const el = renderer.domElement;
    const hasPointerSupport = typeof PointerEvent !== "undefined";
    el.style.touchAction = "none";
    container.style.touchAction = "none";
    el.addEventListener("pointerdown", onDown);
    window.addEventListener("pointermove", onMove, { passive: false });
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);
    if (!hasPointerSupport) {
      el.addEventListener("touchstart", onTouchStart, { passive: false });
      window.addEventListener("touchmove", onTouchMove, { passive: false });
      window.addEventListener("touchend", onTouchEnd, { passive: true });
      window.addEventListener("touchcancel", onTouchEnd, { passive: true });
    }
    el.addEventListener("click", onClk);
    el.addEventListener("wheel", onWheel, { passive: false });
    container.addEventListener("wheel", onWheel, { passive: false });
    el.addEventListener("gesturestart", onGesture, { passive: false });
    el.addEventListener("gesturechange", onGesture, { passive: false });
    el.addEventListener("gestureend", onGesture, { passive: false });
    container.addEventListener("gesturestart", onGesture, { passive: false });
    container.addEventListener("gesturechange", onGesture, { passive: false });
    container.addEventListener("gestureend", onGesture, { passive: false });

    return () => {
      cancelAnimationFrame(frameRef.current);
      resizeObs.disconnect();
      el.removeEventListener("pointerdown", onDown);
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
      if (!hasPointerSupport) {
        el.removeEventListener("touchstart", onTouchStart);
        window.removeEventListener("touchmove", onTouchMove);
        window.removeEventListener("touchend", onTouchEnd);
        window.removeEventListener("touchcancel", onTouchEnd);
      }
      el.removeEventListener("click", onClk);
      el.removeEventListener("wheel", onWheel);
      container.removeEventListener("wheel", onWheel);
      el.removeEventListener("gesturestart", onGesture);
      el.removeEventListener("gesturechange", onGesture);
      el.removeEventListener("gestureend", onGesture);
      container.removeEventListener("gesturestart", onGesture);
      container.removeEventListener("gesturechange", onGesture);
      container.removeEventListener("gestureend", onGesture);
      container.removeChild(el);
      renderer.dispose();
      glowTexture.dispose();
      surfaceTexture?.dispose();
      usStateMat.dispose();
      regionHighlightMat.dispose();
    };
  }, [alerts, onSelect]);

  // --- Highlight selected point ---
  useEffect(() => {
    pointMeshesRef.current.forEach((mesh, id) => {
      const alert = alerts.find((a) => a.alert_id === id);
      if (!alert) return;
      const mat = mesh.material as THREE.MeshPhongMaterial;
      if (id === selectedId) {
        mat.color.set(0xffffff);
        mesh.scale.set(2.5, 2.5, 2.5);
      } else {
        mat.color.set(severityColors[alert.severity]);
        mesh.scale.set(1, 1, 1);
      }
    });
  }, [selectedId, alerts]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full relative cursor-grab active:cursor-grabbing"
    >
      {hotspot && hotspotAlerts.length >= HOTSPOT_MIN_ALERTS && (
        <div className="absolute top-2 left-2 md:top-4 md:left-4 w-[calc(100%-1rem)] md:w-[290px] max-h-[58%] md:max-h-[52%] rounded-xl border border-siem-accent/25 bg-siem-panel/58 backdrop-blur-md shadow-lg shadow-black/35 overflow-hidden">
          <div className="px-3 py-2 border-b border-siem-border/80 flex items-center justify-between">
            <span className="text-[10px] uppercase tracking-widest text-siem-accent font-bold">
              Dense Alert Zone
            </span>
            <div className="flex items-center gap-2">
              <span className="text-[10px] text-siem-muted font-mono">
                {hotspotFiltered.length}/{hotspotAlerts.length}
              </span>
              <button
                onClick={closeHotspot}
                className="text-[10px] px-1.5 py-0.5 rounded border border-siem-border text-siem-muted hover:bg-siem-accent/10 hover:text-siem-accent transition-colors"
              >
                Close
              </button>
            </div>
          </div>
          <div className="px-3 py-2 space-y-1.5 border-b border-siem-border/70">
            <select
              value={hotspotTypeFilter}
              onChange={(e) => setHotspotTypeFilter(e.target.value)}
              className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-2 py-1 text-[11px] text-siem-text"
            >
              <option value="all">All Types</option>
              {hotspotTypeOptions.map((type) => (
                <option key={type} value={type}>
                  {categoryLabels[type]}
                </option>
              ))}
            </select>
            <select
              value={hotspotAgencyFilter}
              onChange={(e) => setHotspotAgencyFilter(e.target.value)}
              className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-2 py-1 text-[11px] text-siem-text"
            >
              <option value="all">All Agencies</option>
              {hotspotAgencyOptions.map((agency) => (
                <option key={agency} value={agency}>
                  {agency}
                </option>
              ))}
            </select>
          </div>
          <div className="max-h-[250px] overflow-y-auto p-2 space-y-1.5">
            {hotspotFiltered.map((alert) => (
              <button
                key={alert.alert_id}
                onClick={() => onSelect(alert.alert_id)}
                className="w-full text-left rounded-md border border-siem-border bg-white/5 hover:bg-siem-accent/10 px-2 py-1.5 transition-colors"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-[10px] text-siem-accent truncate">
                    {alert.source.authority_name}
                  </span>
                  <span className="text-[10px] text-siem-muted">{alert.severity}</span>
                </div>
                <p className="text-[11px] text-siem-text line-clamp-2 leading-snug mt-0.5">
                  {alert.title}
                </p>
              </button>
            ))}
          </div>
        </div>
      )}
      {/* Legend */}
      <div className="hidden sm:flex absolute bottom-4 left-4 items-center gap-4 bg-siem-panel/80 backdrop-blur-sm px-3 py-2 rounded-lg border border-siem-border">
        {(
          [
            ["critical", "bg-red-500"],
            ["high", "bg-orange-500"],
            ["medium", "bg-yellow-500"],
            ["low", "bg-green-500"],
            ["info", "bg-cyan-500"],
          ] as const
        ).map(([sev, bg]) => (
          <div
            key={sev}
            className="flex items-center gap-1.5 text-[10px] uppercase tracking-wider text-siem-muted"
          >
            <div className={`w-2 h-2 rounded-full ${bg}`} />
            {sev}
          </div>
        ))}
      </div>
      <div className="hidden md:block absolute top-4 left-1/2 -translate-x-1/2 text-[10px] text-siem-muted/50 uppercase tracking-widest pointer-events-none">
        Drag to rotate &middot; Scroll to zoom &middot; Click a continent to filter
      </div>
      <div className="absolute bottom-3 right-3 md:bottom-4 md:right-4 flex items-center gap-1.5 bg-siem-panel/80 backdrop-blur-sm px-2 py-2 rounded-lg border border-siem-border">
        <button
          onClick={() => adjustZoom(-ZOOM_STEP * 1.2)}
          className="w-8 h-8 md:w-7 md:h-7 rounded border border-siem-border bg-white/5 text-siem-text hover:bg-siem-accent/10 hover:text-siem-accent transition-colors text-sm font-bold"
          aria-label="Zoom in"
          title="Zoom in"
        >
          +
        </button>
        <button
          onClick={() => adjustZoom(ZOOM_STEP * 1.2)}
          className="w-8 h-8 md:w-7 md:h-7 rounded border border-siem-border bg-white/5 text-siem-text hover:bg-siem-accent/10 hover:text-siem-accent transition-colors text-sm font-bold"
          aria-label="Zoom out"
          title="Zoom out"
        >
          -
        </button>
      </div>
      <div className="hidden md:flex absolute top-4 right-4 items-center gap-1.5 bg-siem-panel/80 backdrop-blur-sm px-2.5 py-2 rounded-lg border border-siem-border">
        <button
          onClick={() => onRegionChange("all")}
          className={`px-2 py-1 text-[10px] uppercase tracking-wider rounded border transition-colors ${
            regionFilter === "all"
              ? "bg-siem-accent/20 text-siem-accent border-siem-accent/40"
              : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
          }`}
        >
          All
        </button>
        {regions.map(([region]) => (
          <button
            key={region}
            onClick={() => onRegionChange(region)}
            className={`px-2 py-1 text-[10px] uppercase tracking-wider rounded border transition-colors ${
              regionFilter === region
                ? "bg-siem-accent/20 text-siem-accent border-siem-accent/40"
                : "bg-white/5 text-siem-muted border-siem-border hover:bg-siem-accent/10 hover:text-siem-accent"
            }`}
          >
            {region}
          </button>
        ))}
      </div>
      <div className="md:hidden absolute top-3 right-3 w-[150px] bg-siem-panel/85 backdrop-blur-sm px-2 py-1.5 rounded-lg border border-siem-border">
        <select
          value={regionFilter}
          onChange={(e) => onRegionChange(e.target.value)}
          className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-2 py-1 text-[10px] text-siem-text"
        >
          <option value="all">All Regions</option>
          {regions.map(([region]) => (
            <option key={region} value={region}>
              {region}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}
