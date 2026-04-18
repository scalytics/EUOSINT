#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, "..");

const inputPath = path.join(root, "public/geo/military-bases.geojson");
const outputPath = inputPath;
const airportsCSVURL = process.env.OURAIRPORTS_CSV_URL || "https://ourairports.com/data/airports.csv";

function csvParse(text) {
  const rows = [];
  let row = [];
  let field = "";
  let inQuotes = false;

  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i];
    const next = text[i + 1];

    if (inQuotes) {
      if (ch === '"' && next === '"') {
        field += '"';
        i += 1;
      } else if (ch === '"') {
        inQuotes = false;
      } else {
        field += ch;
      }
      continue;
    }

    if (ch === '"') {
      inQuotes = true;
      continue;
    }

    if (ch === ',') {
      row.push(field);
      field = "";
      continue;
    }

    if (ch === '\n') {
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
      continue;
    }

    if (ch === '\r') {
      continue;
    }

    field += ch;
  }

  if (field.length > 0 || row.length > 0) {
    row.push(field);
    rows.push(row);
  }

  return rows;
}

function normalizeName(name) {
  return (name || "")
    .toLowerCase()
    .replace(/[^a-z0-9 ]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function regionForLatLng(lat, lng) {
  if (Number.isNaN(lat) || Number.isNaN(lng)) return "all";
  if (lat >= 34 && lat <= 72 && lng >= -11 && lng <= 40) return "Europe";
  if (lat >= -35 && lat <= 38 && lng >= -20 && lng <= 52) return "Africa";
  if (lat >= 5 && lat <= 83 && lng >= -170 && lng <= -52) return "North America";
  if (lat >= -10 && lat <= 78 && lng >= 40 && lng <= 180) return "Asia";
  return "all";
}

const MILITARY_NAME_PATTERNS = [
  /\bair\s*base\b/i,
  /\bair\s*force\s*base\b/i,
  /\barmy\s*air(field|base)\b/i,
  /\bnaval\s*(air\s*)?station\b/i,
  /\bnaval\s*base\b/i,
  /\bmilitary\b/i,
  /\bgarrison\b/i,
  /\bbase\s*aerienne\b/i,
  /\bairbase\b/i,
  /\braf\s+[a-z0-9]/i,
  /\bafb\b/i,
];

const MILITARY_KEYWORD_PATTERNS = [/\bmilitary\b/i, /\bair\s*force\b/i, /\bnavy\b/i, /\barmy\b/i];

if (!fs.existsSync(inputPath)) {
  throw new Error(`Input not found: ${inputPath}`);
}
const existing = JSON.parse(fs.readFileSync(inputPath, "utf8"));
const response = await fetch(airportsCSVURL, { headers: { "user-agent": "kafSIEM/1.0" } });
if (!response.ok) {
  throw new Error(`Fetch failed (${response.status}) from ${airportsCSVURL}`);
}
const csvText = await response.text();
const rows = csvParse(csvText);
if (rows.length < 2) {
  throw new Error("airports CSV is empty");
}

const headers = rows[0].map((h) => h.trim());
const idx = Object.fromEntries(headers.map((h, i) => [h, i]));
for (const required of ["name", "latitude_deg", "longitude_deg", "iso_country", "continent", "keywords", "type"]) {
  if (!(required in idx)) {
    throw new Error(`CSV missing required column: ${required}`);
  }
}

const outFeatures = Array.isArray(existing.features) ? [...existing.features] : [];
const seen = new Set();
for (const f of outFeatures) {
  const p = f?.properties || {};
  const g = f?.geometry || {};
  const c = Array.isArray(g.coordinates) ? g.coordinates : [];
  const lng = Number(c[0]);
  const lat = Number(c[1]);
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) continue;
  const country = String(p.country || p.country_code || p.countryName || "").trim().toUpperCase();
  const name = normalizeName(String(p.name || p.siteName || p.featureName || ""));
  const key = `${country}|${name}|${lat.toFixed(3)}|${lng.toFixed(3)}`;
  seen.add(key);
}

let added = 0;
let scanned = 0;
for (let i = 1; i < rows.length; i += 1) {
  const row = rows[i];
  if (!row || row.length === 0) continue;

  const name = String(row[idx.name] || "").trim();
  const country = String(row[idx.iso_country] || "").trim().toUpperCase();
  const continent = String(row[idx.continent] || "").trim().toUpperCase();
  const keywords = String(row[idx.keywords] || "").trim();
  const type = String(row[idx.type] || "").trim();
  const lat = Number(row[idx.latitude_deg]);
  const lng = Number(row[idx.longitude_deg]);

  if (!name || !country || !Number.isFinite(lat) || !Number.isFinite(lng)) continue;
  if (!["EU", "AS", "AF"].includes(continent)) continue;

  const nameHit = MILITARY_NAME_PATTERNS.some((re) => re.test(name));
  const kwHit = MILITARY_KEYWORD_PATTERNS.some((re) => re.test(keywords));
  if (!nameHit && !kwHit) continue;

  scanned += 1;
  const normName = normalizeName(name);
  const key = `${country}|${normName}|${lat.toFixed(3)}|${lng.toFixed(3)}`;
  if (seen.has(key)) continue;
  seen.add(key);

  const region = regionForLatLng(lat, lng);
  outFeatures.push({
    type: "Feature",
    geometry: { type: "Point", coordinates: [lng, lat] },
    properties: {
      name,
      country,
      type: "air_base",
      operator: "Unknown",
      source: "ourairports",
      confidence: 0.58,
      inferred_from: "name_or_keywords",
      airport_type: type,
      region_hint: region,
    },
  });
  added += 1;
}

const doc = { type: "FeatureCollection", features: outFeatures };
fs.writeFileSync(outputPath, `${JSON.stringify(doc, null, 2)}\n`, "utf8");

console.log(`existing=${existing.features?.length || 0}`);
console.log(`candidate_hits=${scanned}`);
console.log(`added=${added}`);
console.log(`total=${outFeatures.length}`);
