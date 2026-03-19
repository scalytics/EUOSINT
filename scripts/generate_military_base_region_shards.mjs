#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

const root = process.cwd();
const inputPath = path.join(root, "public/geo/military-bases.geojson");
const outDir = path.join(root, "public/geo");

const BOUNDS = {
  "North America": { latMin: 7, latMax: 84, lngMin: -170, lngMax: -50 },
  Europe: { latMin: 34, latMax: 72, lngMin: -12, lngMax: 45 },
  Africa: { latMin: -36, latMax: 38, lngMin: -18, lngMax: 52 },
  Asia: { latMin: -11, latMax: 78, lngMin: 40, lngMax: 180 },
};

function inBounds(lat, lng, b) {
  return lat >= b.latMin && lat <= b.latMax && lng >= b.lngMin && lng <= b.lngMax;
}

function collectLonLat(coords, out) {
  if (!Array.isArray(coords)) return;
  if (coords.length >= 2 && typeof coords[0] === "number" && typeof coords[1] === "number") {
    out.push([coords[0], coords[1]]);
    return;
  }
  for (const c of coords) collectLonLat(c, out);
}

function centerLatLng(geometry) {
  if (!geometry || !geometry.type) return null;
  if (geometry.type === "Point" && Array.isArray(geometry.coordinates) && geometry.coordinates.length >= 2) {
    const [lon, lat] = geometry.coordinates;
    if (typeof lon === "number" && typeof lat === "number") {
      return { lat, lng: lon };
    }
    return null;
  }
  const pts = [];
  collectLonLat(geometry.coordinates, pts);
  if (pts.length === 0) return null;
  let minLon = pts[0][0], maxLon = pts[0][0], minLat = pts[0][1], maxLat = pts[0][1];
  for (const [lon, lat] of pts) {
    if (lon < minLon) minLon = lon;
    if (lon > maxLon) maxLon = lon;
    if (lat < minLat) minLat = lat;
    if (lat > maxLat) maxLat = lat;
  }
  return { lat: (minLat + maxLat) / 2, lng: (minLon + maxLon) / 2 };
}

function regionForFeature(feature) {
  const center = centerLatLng(feature?.geometry);
  if (!center) return null;
  if (inBounds(center.lat, center.lng, BOUNDS["North America"])) return "North America";
  if (inBounds(center.lat, center.lng, BOUNDS.Europe)) return "Europe";
  if (inBounds(center.lat, center.lng, BOUNDS.Africa)) return "Africa";
  if (inBounds(center.lat, center.lng, BOUNDS.Asia)) return "Asia";
  return null;
}

const raw = fs.readFileSync(inputPath, "utf8");
const doc = JSON.parse(raw);
const features = Array.isArray(doc.features) ? doc.features : [];

const buckets = {
  Europe: [],
  Africa: [],
  "North America": [],
  Asia: [],
};

for (const feature of features) {
  const region = regionForFeature(feature);
  if (region && buckets[region]) {
    buckets[region].push(feature);
  }
}

const outputs = [
  ["europe", buckets.Europe],
  ["africa", buckets.Africa],
  ["north-america", buckets["North America"]],
  ["asia", buckets.Asia],
];

for (const [suffix, regionFeatures] of outputs) {
  const outPath = path.join(outDir, `military-bases.${suffix}.geojson`);
  const payload = { type: "FeatureCollection", features: regionFeatures };
  fs.writeFileSync(outPath, `${JSON.stringify(payload, null, 2)}\n`);
  console.log(`${path.basename(outPath)} => ${regionFeatures.length}`);
}
