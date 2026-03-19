#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, "..");

const curatedPath = path.join(root, "public/geo/military-bases.geojson");
const fullPath = path.join(root, "public/geo/military-bases.all.geojson");

const sourcePath = fs.existsSync(fullPath) ? fullPath : curatedPath;
if (!fs.existsSync(sourcePath)) {
  throw new Error(`missing source dataset: ${sourcePath}`);
}

const sourceDoc = JSON.parse(fs.readFileSync(sourcePath, "utf8"));
if (!Array.isArray(sourceDoc.features)) {
  throw new Error(`invalid geojson features: ${sourcePath}`);
}

if (!fs.existsSync(fullPath)) {
  fs.writeFileSync(fullPath, `${JSON.stringify(sourceDoc, null, 2)}\n`, "utf8");
}

const STRATEGIC_PATTERNS = [
  /\bjoint\s+base\b/i,
  /\bnaval\s+base\b/i,
  /\bnaval\s+station\b/i,
  /\bfleet\s+base\b/i,
  /\bheadquarters\b/i,
  /\bhq\b/i,
  /\bcommand\b/i,
  /\bstrategic\b/i,
  /\bmissile\b/i,
  /\bspace\s+force\b/i,
  /\bair\s+defen[cs]e\b/i,
  /\brocket\s+force\b/i,
  /\bgarrison\b/i,
];

const AIRFIELD_PATTERNS = [
  /\bair\s*force\s*base\b/i,
  /\bair\s*base\b/i,
  /\bairbase\b/i,
  /\bairfield\b/i,
  /\baerodrome\b/i,
  /\bmilitary\s+airport\b/i,
  /\bairport\b/i,
  /\bair\s*station\b/i,
  /\bnaval\s+air\s+station\b/i,
  /\bafb\b/i,
  /\bramstein\b/i,
];

const LOW_SIGNAL_PATTERNS = [
  /\btraining\b/i,
  /\breserve\b/i,
  /\brecruit(ing)?\b/i,
  /\bcemetery\b/i,
  /\barmory\b/i,
  /\bannex\b/i,
  /\bdepot\b/i,
  /\bschool\b/i,
  /\bacademy\b/i,
  /\bbarracks\b/i,
  /\bclinic\b/i,
  /\bhospital\b/i,
  /\bhousing\b/i,
  /\bwarehouse\b/i,
  /\bstorage\b/i,
  /\bmunitions\s+storage\b/i,
  /\btest\s+site\b/i,
  /\bexercise\s+area\b/i,
  /\brange\b/i,
];

const OPERATOR_PATTERNS = [
  /\bair\s*force\b/i,
  /\bnavy\b/i,
  /\barmy\b/i,
  /\bdefen[cs]e\b/i,
  /\bministry\b/i,
  /\barmed\s+forces\b/i,
];

const USDOT_MAJOR_PREFIXES = [/^fort\b/i, /^joint\b/i, /^naval\b/i, /^marine\b/i, /^nas\b/i, /^nolf\b/i, /^air\b/i, /^army\b/i];
const USDOT_NOISE_PATTERNS = [/\bng\b/i, /\bnational guard\b/i, /\breserve forces\b/i];

const MAJOR_ALLOWLIST = [
  "ramstein",
  "incirlik",
  "aviano",
  "kadena",
  "diego garcia",
  "camp lemo",
  "camp lemonnier",
  "al udeid",
  "raf akrotiri",
  "rzeszow",
];

function clean(v) {
  return String(v || "").trim();
}

function norm(v) {
  return clean(v)
    .toLowerCase()
    .replace(/[^a-z0-9 ]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function value(props, ...keys) {
  for (const key of keys) {
    if (props[key] !== undefined && props[key] !== null && `${props[key]}`.trim() !== "") {
      return props[key];
    }
  }
  return "";
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
    const [lng, lat] = geometry.coordinates;
    if (typeof lng === "number" && typeof lat === "number") {
      return { lat, lng };
    }
    return null;
  }
  const pts = [];
  collectLonLat(geometry.coordinates, pts);
  if (pts.length === 0) return null;
  let minLng = pts[0][0];
  let maxLng = pts[0][0];
  let minLat = pts[0][1];
  let maxLat = pts[0][1];
  for (const [lng, lat] of pts) {
    if (lng < minLng) minLng = lng;
    if (lng > maxLng) maxLng = lng;
    if (lat < minLat) minLat = lat;
    if (lat > maxLat) maxLat = lat;
  }
  return { lat: (minLat + maxLat) / 2, lng: (minLng + maxLng) / 2 };
}

function classify(feature) {
  const props = feature?.properties || {};
  const name = clean(value(props, "name", "siteName", "featureName", "full_name"));
  const operator = clean(value(props, "operator", "siteReportingComponent"));
  const description = clean(value(props, "featureDescription", "notes", "type", "airport_type"));
  const source = clean(value(props, "source"));
  const isUSDOT = !source || source === "usdot";
  const airportType = clean(value(props, "airport_type")).toLowerCase();

  const text = `${name} ${operator} ${description}`;
  let score = 0;

  const major = STRATEGIC_PATTERNS.some((re) => re.test(text));
  const airfield = AIRFIELD_PATTERNS.some((re) => re.test(text));
  const lowSignal = LOW_SIGNAL_PATTERNS.some((re) => re.test(text));
  const operatorHit = OPERATOR_PATTERNS.some((re) => re.test(operator));
  const allowlisted = MAJOR_ALLOWLIST.some((token) => norm(name).includes(token));

  if (major) score += 6;
  if (airfield) score += 4;
  if (operatorHit) score += 2;
  if (allowlisted) score += 6;
  if (props.isJointBase === true || clean(props.isJointBase) === "Y") score += 3;
  if (isUSDOT && USDOT_MAJOR_PREFIXES.some((re) => re.test(name))) score += 5;
  if (isUSDOT && USDOT_NOISE_PATTERNS.some((re) => re.test(`${name} ${description}`))) score -= 4;

  if (airportType === "large_airport" || airportType === "medium_airport") score += 2;
  if (airportType === "small_airport" || airportType === "closed") score += 1;
  if (airportType === "heliport") score -= 3;

  if (lowSignal && !major && !allowlisted) score -= 3;

  if (!major && !airfield && !allowlisted && !(isUSDOT && score >= 5)) {
    return { keep: false, score, tier: "" };
  }

  if (source === "ourairports" && airportType === "heliport" && !major && !allowlisted) {
    return { keep: false, score, tier: "" };
  }

  const tier = score >= 8 || allowlisted || (major && !lowSignal) ? "major" : "operational_airfield";
  if (score < 3) {
    return { keep: false, score, tier: "" };
  }
  return { keep: true, score, tier };
}

const dedupe = new Set();
const curatedFeatures = [];
let dropped = 0;
for (const feature of sourceDoc.features) {
  const center = centerLatLng(feature?.geometry);
  if (!center) {
    dropped += 1;
    continue;
  }
  const { lat, lng } = center;

  const props = feature?.properties || {};
  const name = clean(value(props, "name", "siteName", "featureName", "full_name"));
  const country = clean(value(props, "country", "country_code", "countryName", "iso_country")).toUpperCase();
  const key = `${country}|${norm(name)}|${lat.toFixed(3)}|${lng.toFixed(3)}`;

  const verdict = classify(feature);
  if (!verdict.keep) {
    dropped += 1;
    continue;
  }

  if (dedupe.has(key)) {
    dropped += 1;
    continue;
  }
  dedupe.add(key);

  const nextProps = {
    ...props,
    importance_score: verdict.score,
    importance_tier: verdict.tier,
  };

  curatedFeatures.push({
    ...feature,
    geometry: { type: "Point", coordinates: [lng, lat] },
    properties: nextProps,
  });
}

const out = {
  type: "FeatureCollection",
  features: curatedFeatures,
};

fs.writeFileSync(curatedPath, `${JSON.stringify(out, null, 2)}\n`, "utf8");

const byTier = {};
for (const f of curatedFeatures) {
  const tier = f?.properties?.importance_tier || "unknown";
  byTier[tier] = (byTier[tier] || 0) + 1;
}

console.log(`source=${sourceDoc.features.length}`);
console.log(`curated=${curatedFeatures.length}`);
console.log(`dropped=${dropped}`);
console.log(`tiers=${JSON.stringify(byTier)}`);
