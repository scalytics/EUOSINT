/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import crypto from "node:crypto";

const MAX_PER_SOURCE = Number.parseInt(process.env.MAX_PER_SOURCE ?? "20", 10);
const OUTPUT_PATH = process.env.OUTPUT_PATH ?? "public/alerts.json";
const STATE_OUTPUT_PATH = process.env.STATE_OUTPUT_PATH ?? "public/alerts-state.json";
const FILTERED_OUTPUT_PATH =
  process.env.FILTERED_OUTPUT_PATH ?? "public/alerts-filtered.json";
const SOURCE_HEALTH_OUTPUT_PATH =
  process.env.SOURCE_HEALTH_OUTPUT_PATH ?? "public/source-health.json";
const SOURCE_REGISTRY_PATH =
  process.env.SOURCE_REGISTRY_PATH ?? "registry/source_registry.json";
const MAX_AGE_DAYS = Number.parseInt(process.env.MAX_AGE_DAYS ?? "180", 10);
const REMOVED_RETENTION_DAYS = Number.parseInt(
  process.env.REMOVED_RETENTION_DAYS ?? "14",
  10
);
const INCIDENT_RELEVANCE_THRESHOLD = Number.parseFloat(
  process.env.INCIDENT_RELEVANCE_THRESHOLD ?? "0.42"
);
const MISSING_PERSON_RELEVANCE_THRESHOLD = Number.parseFloat(
  process.env.MISSING_PERSON_RELEVANCE_THRESHOLD ?? "0"
);
const FAIL_ON_CRITICAL_SOURCE_GAP =
  process.env.FAIL_ON_CRITICAL_SOURCE_GAP === "1";
const CRITICAL_SOURCE_PREFIXES = (process.env.CRITICAL_SOURCE_PREFIXES ??
  "cisa")
  .split(",")
  .map((value) => value.trim())
  .filter(Boolean);
const WATCH =
  process.argv.includes("--watch") || process.env.WATCH === "1";
const INTERVAL_MS = Number.parseInt(process.env.INTERVAL_MS ?? "900000", 10);
let externalSourcesCache = null;

const US_STATE_CENTROIDS = {
  alabama: [32.806671, -86.79113],
  alaska: [61.370716, -152.404419],
  arizona: [33.729759, -111.431221],
  arkansas: [34.969704, -92.373123],
  california: [36.116203, -119.681564],
  colorado: [39.059811, -105.311104],
  connecticut: [41.597782, -72.755371],
  delaware: [39.318523, -75.507141],
  florida: [27.766279, -81.686783],
  georgia: [33.040619, -83.643074],
  hawaii: [21.094318, -157.498337],
  idaho: [44.240459, -114.478828],
  illinois: [40.349457, -88.986137],
  indiana: [39.849426, -86.258278],
  iowa: [42.011539, -93.210526],
  kansas: [38.5266, -96.726486],
  kentucky: [37.66814, -84.670067],
  louisiana: [31.169546, -91.867805],
  maine: [44.693947, -69.381927],
  maryland: [39.063946, -76.802101],
  massachusetts: [42.230171, -71.530106],
  michigan: [43.326618, -84.536095],
  minnesota: [45.694454, -93.900192],
  mississippi: [32.741646, -89.678696],
  missouri: [38.456085, -92.288368],
  montana: [46.921925, -110.454353],
  nebraska: [41.12537, -98.268082],
  nevada: [38.313515, -117.055374],
  "new hampshire": [43.452492, -71.563896],
  "new jersey": [40.298904, -74.521011],
  "new mexico": [34.840515, -106.248482],
  "new york": [42.165726, -74.948051],
  "north carolina": [35.630066, -79.806419],
  "north dakota": [47.528912, -99.784012],
  ohio: [40.388783, -82.764915],
  oklahoma: [35.565342, -96.928917],
  oregon: [44.572021, -122.070938],
  pennsylvania: [40.590752, -77.209755],
  "rhode island": [41.680893, -71.51178],
  "south carolina": [33.856892, -80.945007],
  "south dakota": [44.299782, -99.438828],
  tennessee: [35.747845, -86.692345],
  texas: [31.054487, -97.563461],
  utah: [40.150032, -111.862434],
  vermont: [44.045876, -72.710686],
  virginia: [37.769337, -78.169968],
  washington: [47.400902, -121.490494],
  "west virginia": [38.491226, -80.954453],
  wisconsin: [44.268543, -89.616508],
  wyoming: [42.755966, -107.30249],
  "district of columbia": [38.9072, -77.0369],
  "washington dc": [38.9072, -77.0369],
};

const US_STATE_ABBR_TO_NAME = {
  AL: "alabama",
  AK: "alaska",
  AZ: "arizona",
  AR: "arkansas",
  CA: "california",
  CO: "colorado",
  CT: "connecticut",
  DE: "delaware",
  FL: "florida",
  GA: "georgia",
  HI: "hawaii",
  ID: "idaho",
  IL: "illinois",
  IN: "indiana",
  IA: "iowa",
  KS: "kansas",
  KY: "kentucky",
  LA: "louisiana",
  ME: "maine",
  MD: "maryland",
  MA: "massachusetts",
  MI: "michigan",
  MN: "minnesota",
  MS: "mississippi",
  MO: "missouri",
  MT: "montana",
  NE: "nebraska",
  NV: "nevada",
  NH: "new hampshire",
  NJ: "new jersey",
  NM: "new mexico",
  NY: "new york",
  NC: "north carolina",
  ND: "north dakota",
  OH: "ohio",
  OK: "oklahoma",
  OR: "oregon",
  PA: "pennsylvania",
  RI: "rhode island",
  SC: "south carolina",
  SD: "south dakota",
  TN: "tennessee",
  TX: "texas",
  UT: "utah",
  VT: "vermont",
  VA: "virginia",
  WA: "washington",
  WV: "west virginia",
  WI: "wisconsin",
  WY: "wyoming",
  DC: "district of columbia",
};

const US_STATE_ALT_TOKENS = {
  fla: "florida",
  calif: "california",
  penn: "pennsylvania",
  penna: "pennsylvania",
  wisc: "wisconsin",
  minn: "minnesota",
  colo: "colorado",
  ariz: "arizona",
  mich: "michigan",
  mass: "massachusetts",
  conn: "connecticut",
  ill: "illinois",
  tex: "texas",
  wash: "washington",
  ore: "oregon",
  okla: "oklahoma",
  "n mex": "new mexico",
  "n dak": "north dakota",
  "s dak": "south dakota",
  "n car": "north carolina",
  "s car": "south carolina",
  "w va": "west virginia",
};

const COUNTRY_CENTROIDS = {
  "south africa": [-30.5595, 22.9375],
  egypt: [26.8206, 30.8025],
  nigeria: [9.082, 8.6753],
  kenya: [-0.0236, 37.9062],
  tanzania: [-6.369, 34.8888],
  madagascar: [-18.7669, 46.8691],
  uganda: [1.3733, 32.2903],
  rwanda: [-1.9403, 29.8739],
  zambia: [-13.1339, 27.8493],
  zimbabwe: [-19.0154, 29.1549],
  botswana: [-22.3285, 24.6849],
  namibia: [-22.9576, 18.4904],
  mozambique: [-18.6657, 35.5296],
  morocco: [31.7917, -7.0926],
  algeria: [28.0339, 1.6596],
  ghana: [7.9465, -1.0232],
  ethiopia: [9.145, 40.4897],
  argentina: [-38.4161, -63.6167],
  chile: [-35.6751, -71.543],
  colombia: [4.5709, -74.2973],
  peru: [-9.19, -75.0152],
  uruguay: [-32.5228, -55.7658],
  paraguay: [-23.4425, -58.4438],
  bolivia: [-16.2902, -63.5887],
  venezuela: [6.4238, -66.5897],
  mexico: [23.6345, -102.5528],
  guatemala: [15.7835, -90.2308],
  belize: [17.1899, -88.4976],
  honduras: [15.2, -86.2419],
  "el salvador": [13.7942, -88.8965],
  nicaragua: [12.8654, -85.2072],
  "costa rica": [9.7489, -83.7534],
  panama: [8.538, -80.7821],
  "south korea": [35.9078, 127.7669],
  malaysia: [4.2105, 101.9758],
  thailand: [15.87, 100.9925],
  vietnam: [14.0583, 108.2772],
  indonesia: [-0.7893, 113.9213],
  philippines: [12.8797, 121.774],
  bangladesh: [23.685, 90.3563],
  "sri lanka": [7.8731, 80.7718],
  "united arab emirates": [23.4241, 53.8478],
  "saudi arabia": [23.8859, 45.0792],
  qatar: [25.3548, 51.1839],
  kuwait: [29.3117, 47.4818],
  bahrain: [26.0667, 50.5577],
  oman: [21.4735, 55.9754],
  jordan: [30.5852, 36.2384],
  lebanon: [33.8547, 35.8623],
  israel: [31.0461, 34.8516],
  iran: [32.4279, 53.688],
  iraq: [33.2232, 43.6793],
  france: [46.2276, 2.2137],
  germany: [51.1657, 10.4515],
  netherlands: [52.1326, 5.2913],
  belgium: [50.5039, 4.4699],
  spain: [40.4637, -3.7492],
  italy: [41.8719, 12.5674],
  sweden: [60.1282, 18.6435],
  poland: [51.9194, 19.1451],
  bulgaria: [42.7339, 25.4858],
  romania: [45.9432, 24.9668],
  greece: [39.0742, 21.8243],
  portugal: [39.3999, -8.2245],
  ireland: [53.1424, -7.6921],
  switzerland: [46.8182, 8.2275],
  austria: [47.5162, 14.5501],
  ukraine: [48.3794, 31.1656],
  turkey: [38.9637, 35.2433],
  "united kingdom": [55.3781, -3.436],
  england: [52.3555, -1.1743],
  scotland: [56.4907, -4.2026],
  wales: [52.1307, -3.7837],
  "new zealand": [-41.5, 172.8],
  australia: [-25.2744, 133.7751],
  canada: [56.1304, -106.3468],
  "united states": [39.8283, -98.5795],
  usa: [39.8283, -98.5795],
  brazil: [-14.235, -51.9253],
  india: [20.5937, 78.9629],
  china: [35.8617, 104.1954],
  russia: [61.524, 105.3188],
  japan: [36.2048, 138.2529],
  colombia: [4.5709, -74.2973],
  "south korea": [35.9078, 127.7669],
  singapore: [1.3521, 103.8198],
  "hong kong": [22.3193, 114.1694],
  "south africa": [-30.5595, 22.9375],
  nigeria: [9.082, 8.6753],
  kenya: [-0.0236, 37.9062],
  mexico: [23.6345, -102.5528],
  chile: [-35.6751, -71.543],
  argentina: [-38.4161, -63.6167],
  norway: [60.472, 8.4689],
  sweden: [60.1282, 18.6435],
  denmark: [56.2639, 9.5018],
  finland: [61.9241, 25.7482],
  jamaica: [18.1096, -77.2975],
  bahamas: [25.0343, -77.3963],
  barbados: [13.1939, -59.5432],
  "dominican republic": [18.7357, -70.1627],
  haiti: [18.9712, -72.2852],
  cuba: [21.5218, -77.7812],
  "trinidad and tobago": [10.6918, -61.2225],
  philippines: [12.8797, 121.774],
  malaysia: [4.2105, 101.9758],
  thailand: [15.87, 100.9925],
  vietnam: [14.0583, 108.2772],
  indonesia: [-0.7893, 113.9213],
  taiwan: [23.6978, 120.9605],
};

const CITY_CENTROIDS = {
  harrisburg: [40.2732, -76.8867],
  philadelphia: [39.9526, -75.1652],
  pittsburgh: [40.4406, -79.9959],
  allentown: [40.6023, -75.4714],
  scranton: [41.4089, -75.6624],
  erie: [42.1292, -80.0851],
  york: [39.9626, -76.7277],
  lancaster: [40.0379, -76.3055],
  richmond: [37.5407, -77.436],
  norfolk: [36.8508, -76.2859],
  alexandria: [38.8048, -77.0469],
  arlington: [38.8816, -77.091],
  baltimore: [39.2904, -76.6122],
  washington: [38.9072, -77.0369],
  "washington dc": [38.9072, -77.0369],
  "new york city": [40.7128, -74.006],
  "los angeles": [34.0522, -118.2437],
  chicago: [41.8781, -87.6298],
  miami: [25.7617, -80.1918],
  houston: [29.7604, -95.3698],
  dallas: [32.7767, -96.797],
  auckland: [-36.8485, 174.7633],
  wellington: [-41.2865, 174.7762],
  christchurch: [-43.5321, 172.6362],
  hamilton: [-37.787, 175.2793],
  tauranga: [-37.6878, 176.1651],
  dunedin: [-45.8788, 170.5028],
  queenstown: [-45.0312, 168.6626],
  whangarei: [-35.7251, 174.3237],
  taupo: [-38.6869, 176.0702],
  "raumati beach": [-40.9398, 174.9768],
  lyon: [45.764, 4.8357],
  paris: [48.8566, 2.3522],
  london: [51.5072, -0.1276],
  amsterdam: [52.3676, 4.9041],
  brussels: [50.8503, 4.3517],
  sofia: [42.6977, 23.3219],
  warsaw: [52.2297, 21.0122],
  stockholm: [59.3293, 18.0686],
  berlin: [52.52, 13.405],
  madrid: [40.4168, -3.7038],
  rome: [41.9028, 12.4964],
  vienna: [48.2082, 16.3738],
  dublin: [53.3498, -6.2603],
  sydney: [-33.8688, 151.2093],
  melbourne: [-37.8136, 144.9631],
  tokyo: [35.6762, 139.6503],
  osaka: [34.6937, 135.5023],
  bogota: [4.711, -74.0721],
  medellin: [6.2442, -75.5812],
  cali: [3.4516, -76.532],
  "the hague": [52.0705, 4.3007],
  rotterdam: [51.9225, 4.4792],
  sacramento: [38.5816, -121.4944],
  "san francisco": [37.7749, -122.4194],
  "san diego": [32.7157, -117.1611],
  "san jose": [37.3382, -121.8863],
};

const ISO2_COUNTRY_HINTS = {
  ZA: "south africa",
  EG: "egypt",
  NG: "nigeria",
  KE: "kenya",
  TZ: "tanzania",
  MG: "madagascar",
  UG: "uganda",
  RW: "rwanda",
  ZM: "zambia",
  ZW: "zimbabwe",
  BW: "botswana",
  NA: "namibia",
  MZ: "mozambique",
  MA: "morocco",
  DZ: "algeria",
  GH: "ghana",
  ET: "ethiopia",
  BR: "brazil",
  AR: "argentina",
  CL: "chile",
  CO: "colombia",
  PE: "peru",
  UY: "uruguay",
  PY: "paraguay",
  BO: "bolivia",
  VE: "venezuela",
  MX: "mexico",
  GT: "guatemala",
  BZ: "belize",
  HN: "honduras",
  SV: "el salvador",
  NI: "nicaragua",
  CR: "costa rica",
  PA: "panama",
  JM: "jamaica",
  TT: "trinidad and tobago",
  BS: "bahamas",
  BB: "barbados",
  DO: "dominican republic",
  HT: "haiti",
  CU: "cuba",
  JP: "japan",
  IN: "india",
  SG: "singapore",
  KR: "south korea",
  MY: "malaysia",
  TH: "thailand",
  VN: "vietnam",
  ID: "indonesia",
  PH: "philippines",
  BD: "bangladesh",
  LK: "sri lanka",
  AE: "united arab emirates",
  SA: "saudi arabia",
  QA: "qatar",
  KW: "kuwait",
  BH: "bahrain",
  OM: "oman",
  JO: "jordan",
  LB: "lebanon",
  IL: "israel",
  TR: "turkey",
  IR: "iran",
  IQ: "iraq",
  FR: "france",
  DE: "germany",
  NL: "netherlands",
  ES: "spain",
  IT: "italy",
  GB: "united kingdom",
  US: "united states",
  CA: "canada",
  AU: "australia",
  NZ: "new zealand",
};

// ─── AGENCY FEEDS ───────────────────────────────────────────────
// Organized by: CISA | FBI | INTERPOL | EUROPOL | NCSC | POLICE (region) | PUBLIC SAFETY
// Only confirmed-working feeds are included.

const sources = [
  // ── CISA (US / North America) ─────────────────────────────────
  {
    type: "kev-json",
    source: {
      source_id: "cisa-kev",
      authority_name: "CISA",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "cert",
      base_url: "https://www.cisa.gov",
    },
    feed_url: "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json",
    category: "cyber_advisory",
    region_tag: "US",
    lat: 38.88,
    lng: -77.02,
    reporting: {
      label: "Report to CISA",
      url: "https://www.cisa.gov/report",
      notes: "Use 911 for emergencies.",
    },
  },

  // ── FBI (US / North America) ──────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "fbi",
      authority_name: "FBI",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.fbi.gov",
    },
    feed_url: "https://www.fbi.gov/feeds/fbi-top-stories/rss.xml",
    category: "public_appeal",
    region_tag: "US",
    lat: 38.9,
    lng: -77.0,
    reporting: {
      label: "Report to FBI",
      url: "https://tips.fbi.gov/",
      phone: "1-800-CALL-FBI (1-800-225-5324)",
      notes: "Use 911 for emergencies.",
    },
  },
  {
    type: "rss",
    source: {
      source_id: "fbi-wanted",
      authority_name: "FBI Wanted",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.fbi.gov",
    },
    feed_url: "https://www.fbi.gov/feeds/all-wanted/rss.xml",
    category: "wanted_suspect",
    region_tag: "US",
    lat: 38.9,
    lng: -77.0,
    reporting: {
      label: "Submit a Tip to FBI",
      url: "https://tips.fbi.gov/",
      phone: "1-800-CALL-FBI (1-800-225-5324)",
      notes: "Use 911 for emergencies.",
    },
  },

  // ── INTERPOL: Removed — replaced by static hub entry in buildAlerts() ──
  // ── EUROPOL (EU / Europe) ─────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "europol",
      authority_name: "Europol",
      country: "Netherlands",
      country_code: "NL",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.europol.europa.eu",
    },
    feed_url: "https://www.europol.europa.eu/rss.xml",
    category: "public_appeal",
    region_tag: "EU",
    lat: 52.09,
    lng: 4.27,
    reporting: {
      label: "Report to Europol",
      url: "https://www.europol.europa.eu/report-a-crime",
    },
  },

  // ── NCSC UK (UK / Europe) ─────────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "ncsc-uk",
      authority_name: "NCSC UK",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://www.ncsc.gov.uk",
    },
    feed_url: "https://www.ncsc.gov.uk/api/1/services/v1/report-rss-feed.xml",
    category: "cyber_advisory",
    region_tag: "GB",
    lat: 51.5,
    lng: -0.13,
    reporting: {
      label: "Report to NCSC",
      url: "https://www.ncsc.gov.uk/section/about-this-website/report-scam-website",
    },
  },
  {
    type: "rss",
    source: {
      source_id: "ncsc-uk-all",
      authority_name: "NCSC UK",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://www.ncsc.gov.uk",
    },
    feed_url: "https://www.ncsc.gov.uk/api/1/services/v1/all-rss-feed.xml",
    category: "cyber_advisory",
    region_tag: "GB",
    lat: 51.51,
    lng: -0.1,
    reporting: {
      label: "Report to NCSC",
      url: "https://www.ncsc.gov.uk/section/about-this-website/report-scam-website",
    },
  },

  // ── POLICE: New Zealand (Oceania) ─────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "nz-police-news",
      authority_name: "NZ Police",
      country: "New Zealand",
      country_code: "NZ",
      region: "Oceania",
      authority_type: "police",
      base_url: "https://www.police.govt.nz",
    },
    feed_url: "https://www.police.govt.nz/rss/news",
    category: "public_safety",
    region_tag: "NZ",
    lat: -41.29,
    lng: 174.78,
    reporting: {
      label: "Report to NZ Police",
      url: "https://www.police.govt.nz/use-105",
      phone: "111 (Emergency) / 105 (Non-emergency)",
    },
  },
  {
    type: "rss",
    source: {
      source_id: "nz-police-alerts",
      authority_name: "NZ Police",
      country: "New Zealand",
      country_code: "NZ",
      region: "Oceania",
      authority_type: "police",
      base_url: "https://www.police.govt.nz",
    },
    feed_url: "https://www.police.govt.nz/rss/alerts",
    category: "public_appeal",
    region_tag: "NZ",
    lat: -41.29,
    lng: 174.78,
    reporting: {
      label: "Report to NZ Police",
      url: "https://www.police.govt.nz/use-105",
      phone: "111 (Emergency) / 105 (Non-emergency)",
    },
  },

  // ── PUBLIC SAFETY: NCMEC (US / North America) ─────────────────
  {
    type: "rss",
    source: {
      source_id: "ncmec",
      authority_name: "NCMEC",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "public_safety_program",
      base_url: "https://www.missingkids.org",
    },
    feed_url:
      "https://api.missingkids.org/missingkids/servlet/XmlServlet?LanguageCountry=en_US&act=rss&orgPrefix=NCMC",
    category: "missing_person",
    region_tag: "US",
    lat: 39.83,
    lng: -98.58,
    reporting: {
      label: "Report to NCMEC",
      url: "https://report.cybertip.org/",
      phone: "1-800-THE-LOST (1-800-843-5678)",
      notes: "Use 911 for immediate danger.",
    },
  },

  // ── CIS MS-ISAC (US / North America) ────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "cis-msisac",
      authority_name: "CIS MS-ISAC",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "cert",
      base_url: "https://www.cisecurity.org",
    },
    feed_url: "https://www.cisecurity.org/feed/advisories",
    category: "cyber_advisory",
    region_tag: "US",
    lat: 42.65,
    lng: -73.76,
    reporting: {
      label: "Report to MS-ISAC",
      url: "https://www.cisecurity.org/ms-isac/services/soc",
      phone: "1-866-787-4722",
      email: "soc@cisecurity.org",
      notes: "24/7 Security Operations Center for state, local, tribal, and territorial governments.",
    },
  },

  // ── California Attorney General (US / North America) ────────────
  {
    type: "rss",
    source: {
      source_id: "ca-oag",
      authority_name: "California AG",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://oag.ca.gov",
    },
    feed_url: "https://oag.ca.gov/news/feed",
    category: "public_appeal",
    region_tag: "US",
    lat: 38.58,
    lng: -121.49,
    reporting: {
      label: "Report to CA Attorney General",
      url: "https://oag.ca.gov/contact/consumer-complaint-against-business-or-company",
      phone: "1-800-952-5225",
    },
  },

  // ── CERT-FR (France / Europe) ───────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "cert-fr",
      authority_name: "CERT-FR",
      country: "France",
      country_code: "FR",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://www.cert.ssi.gouv.fr",
    },
    feed_url: "https://www.cert.ssi.gouv.fr/feed/",
    category: "cyber_advisory",
    region_tag: "FR",
    lat: 48.86,
    lng: 2.35,
    reporting: {
      label: "Report to CERT-FR",
      url: "https://www.cert.ssi.gouv.fr/contact/",
      email: "cert-fr@ssi.gouv.fr",
    },
  },

  // ── NCSC-NL (Netherlands / Europe) ──────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "ncsc-nl",
      authority_name: "NCSC-NL",
      country: "Netherlands",
      country_code: "NL",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://advisories.ncsc.nl",
    },
    feed_url: "https://advisories.ncsc.nl/rss/advisories",
    category: "cyber_advisory",
    region_tag: "NL",
    lat: 52.07,
    lng: 4.30,
    reporting: {
      label: "Report to NCSC-NL",
      url: "https://www.ncsc.nl/contact/kwetsbaarheid-melden",
      email: "cert@ncsc.nl",
    },
  },

  // ── JPCERT/CC (Japan / Asia) ────────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "jpcert",
      authority_name: "JPCERT/CC",
      country: "Japan",
      country_code: "JP",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.jpcert.or.jp",
    },
    feed_url: "https://www.jpcert.or.jp/english/rss/jpcert-en.rdf",
    category: "cyber_advisory",
    region_tag: "JP",
    lat: 35.68,
    lng: 139.69,
    reporting: {
      label: "Report to JPCERT/CC",
      url: "https://www.jpcert.or.jp/english/ir/form.html",
      email: "info@jpcert.or.jp",
    },
  },

  // ── Colombia National Police (South America) ────────────────────
  {
    type: "rss",
    source: {
      source_id: "policia-colombia",
      authority_name: "Policía Nacional de Colombia",
      country: "Colombia",
      country_code: "CO",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.policia.gov.co",
    },
    feed_url: "https://www.policia.gov.co/rss.xml",
    category: "public_appeal",
    region_tag: "CO",
    lat: 4.71,
    lng: -74.07,
    reporting: {
      label: "Report to Policía Nacional",
      url: "https://www.policia.gov.co/denuncia-virtual",
      phone: "123 (Emergency) / 112 (Línea única)",
    },
  },

  // ── CISA Alerts RSS (US / North America) ─────────────────────────
  // May return 403 locally but works from GitHub Actions
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "cisa-alerts",
      authority_name: "CISA Alerts",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "cert",
      base_url: "https://www.cisa.gov",
    },
    feed_url: "https://www.cisa.gov/cybersecurity-advisories/all.xml",
    category: "cyber_advisory",
    region_tag: "US",
    lat: 38.89,
    lng: -77.03,
    reporting: {
      label: "Report to CISA",
      url: "https://www.cisa.gov/report",
      phone: "1-888-282-0870",
      email: "central@cisa.dhs.gov",
      notes: "Use 911 for emergencies.",
    },
  },

  // ── DHS (US / North America) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "dhs",
      authority_name: "DHS",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "national_security",
      base_url: "https://www.dhs.gov",
    },
    feed_url: "https://www.dhs.gov/news/rss.xml",
    category: "public_safety",
    region_tag: "US",
    lat: 38.886,
    lng: -77.015,
    reporting: {
      label: "Report to DHS",
      url: "https://www.dhs.gov/see-something-say-something/how-to-report-suspicious-activity",
      phone: "1-866-347-2423",
    },
  },

  // ── US Secret Service (US / North America) ──────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "usss",
      authority_name: "US Secret Service",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.secretservice.gov",
    },
    feed_url: "https://www.secretservice.gov/rss.xml",
    category: "public_appeal",
    region_tag: "US",
    lat: 38.899,
    lng: -77.034,
    reporting: {
      label: "Report to Secret Service",
      url: "https://www.secretservice.gov/contact",
      phone: "1-202-406-5708",
    },
  },

  // ── DEA (US / North America) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "dea",
      authority_name: "DEA",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.dea.gov",
    },
    feed_url: "https://www.dea.gov/press-releases/rss.xml",
    category: "public_appeal",
    region_tag: "US",
    lat: 38.871,
    lng: -77.053,
    reporting: {
      label: "Report to DEA",
      url: "https://www.dea.gov/submit-tip",
      phone: "1-877-792-2873",
    },
  },

  // ── ATF (US / North America) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "atf",
      authority_name: "ATF",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.atf.gov",
    },
    feed_url: "https://www.atf.gov/news/rss.xml",
    category: "public_appeal",
    region_tag: "US",
    lat: 38.893,
    lng: -77.025,
    reporting: {
      label: "Report to ATF",
      url: "https://www.atf.gov/contact/atf-tips",
      phone: "1-888-283-8477",
      email: "atftips@atf.gov",
    },
  },

  // ── US Marshals (US / North America) ────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "usms",
      authority_name: "US Marshals",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.usmarshals.gov",
    },
    feed_url: "https://www.usmarshals.gov/news/news-releases.rss",
    category: "wanted_suspect",
    region_tag: "US",
    lat: 38.895,
    lng: -77.021,
    reporting: {
      label: "Report to US Marshals",
      url: "https://www.usmarshals.gov/tips",
      phone: "1-877-926-8332",
    },
  },

  // ── NCA UK (UK / Europe) ────────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "nca-uk",
      authority_name: "NCA UK",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.nationalcrimeagency.gov.uk",
    },
    feed_url: "https://nationalcrimeagency.gov.uk/news?format=feed&type=rss",
    category: "public_appeal",
    region_tag: "GB",
    lat: 51.49,
    lng: -0.11,
    reporting: {
      label: "Report to NCA",
      url: "https://www.nationalcrimeagency.gov.uk/what-we-do/crime-threats/cyber-crime/reporting-cyber-crime",
      phone: "0370 496 7622",
    },
  },

  // ── GMP Manchester UK (UK / Europe) ─────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "gmp-uk",
      authority_name: "Greater Manchester Police",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.gmp.police.uk",
    },
    feed_url: "https://www.gmp.police.uk/news/greater-manchester/rss/",
    category: "public_appeal",
    region_tag: "GB",
    lat: 53.48,
    lng: -2.24,
    reporting: {
      label: "Report to GMP",
      url: "https://www.gmp.police.uk/ro/report/",
      phone: "999 (Emergency) / 101 (Non-emergency)",
    },
  },

  // ── Met Police UK (UK / Europe) ─────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "met-police-uk",
      authority_name: "Met Police UK",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "police",
      base_url: "https://news.met.police.uk",
    },
    feed_url: "https://news.met.police.uk/feeds/rss",
    category: "public_appeal",
    region_tag: "GB",
    lat: 51.51,
    lng: -0.14,
    reporting: {
      label: "Report to Met Police",
      url: "https://www.met.police.uk/ro/report/",
      phone: "999 (Emergency) / 101 (Non-emergency)",
    },
  },

  // ── BSI Germany (Germany / Europe) ──────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "bsi-de",
      authority_name: "BSI Germany",
      country: "Germany",
      country_code: "DE",
      region: "Europe",
      authority_type: "cert",
      base_url: "https://www.bsi.bund.de",
    },
    feed_url: "https://www.bsi.bund.de/SiteGlobals/Functions/RSSFeed/RSSNewsfeed/RSSNewsfeed.xml",
    category: "cyber_advisory",
    region_tag: "DE",
    lat: 50.73,
    lng: 7.10,
    reporting: {
      label: "Report to BSI",
      url: "https://www.bsi.bund.de/EN/Service-Navi/Contact/contact_node.html",
      email: "certbund@bsi.bund.de",
    },
  },

  // ── BKA Germany (Germany / Europe) ──────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "bka-de",
      authority_name: "BKA Germany",
      country: "Germany",
      country_code: "DE",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.bka.de",
    },
    feed_url: "https://www.bka.de/SharedDocs/Kurzmeldungen/DE/Warnhinweise/RSS/BKA_Pressemitteilungen_RSS.xml",
    category: "wanted_suspect",
    region_tag: "DE",
    lat: 50.12,
    lng: 8.68,
    reporting: {
      label: "Report to BKA",
      url: "https://www.bka.de/DE/KontaktAufnehmen/Hinweisportal/hinweisportal_node.html",
      phone: "+49 611 55-0",
    },
  },

  // ── ACSC Australia (Oceania) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "acsc-au",
      authority_name: "ACSC Australia",
      country: "Australia",
      country_code: "AU",
      region: "Oceania",
      authority_type: "cert",
      base_url: "https://www.cyber.gov.au",
    },
    feed_url: "https://www.cyber.gov.au/advisories/feed",
    feed_urls: [
      "https://www.cyber.gov.au/advisories/feed",
      "https://www.cyber.gov.au/about-us/advisories/rss.xml",
      "https://www.cyber.gov.au/alerts/feed",
    ],
    category: "cyber_advisory",
    region_tag: "AU",
    lat: -35.28,
    lng: 149.13,
    reporting: {
      label: "Report to ACSC",
      url: "https://www.cyber.gov.au/report-and-recover/report",
      phone: "1300 292 371",
    },
  },

  // ── AFP Australia (Oceania) ─────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "afp-au",
      authority_name: "AFP Australia",
      country: "Australia",
      country_code: "AU",
      region: "Oceania",
      authority_type: "police",
      base_url: "https://www.afp.gov.au",
    },
    feed_url: "https://www.afp.gov.au/news-centre/media-releases/rss.xml",
    feed_urls: [
      "https://www.afp.gov.au/news-centre/media-releases/rss.xml",
      "https://www.afp.gov.au/news-centre/media-release/rss.xml",
      "https://www.afp.gov.au/news-centre/media-releases/feed",
    ],
    category: "public_appeal",
    region_tag: "AU",
    lat: -35.31,
    lng: 149.14,
    reporting: {
      label: "Report to AFP",
      url: "https://www.afp.gov.au/what-we-do/crime-types/report-crime",
      phone: "131 237",
    },
  },

  // ── Queensland Police Service (Oceania) ────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "qps-au",
      authority_name: "Queensland Police",
      country: "Australia",
      country_code: "AU",
      region: "Oceania",
      authority_type: "police",
      base_url: "https://mypolice.qld.gov.au",
    },
    feed_url: "https://mypolice.qld.gov.au/feed/",
    feed_urls: [
      "https://mypolice.qld.gov.au/feed/",
      "https://mypolice.qld.gov.au/category/alert/feed/",
      "https://mypolice.qld.gov.au/category/my-police-news/feed/",
    ],
    category: "public_appeal",
    region_tag: "AU",
    lat: -27.47,
    lng: 153.03,
    reporting: {
      label: "Report to Queensland Police",
      url: "https://www.police.qld.gov.au/policelink-reporting",
      phone: "000 (Emergency) / 131 444 (Policelink)",
    },
  },

  // ── New South Wales Police (Oceania) ───────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "nsw-police-au",
      authority_name: "NSW Police",
      country: "Australia",
      country_code: "AU",
      region: "Oceania",
      authority_type: "police",
      base_url: "https://www.police.nsw.gov.au",
    },
    feed_url: "https://www.police.nsw.gov.au/news/rss",
    feed_urls: [
      "https://www.police.nsw.gov.au/news/rss",
      "https://www.police.nsw.gov.au/rss/news",
      "https://www.police.nsw.gov.au/news/feed",
    ],
    category: "public_appeal",
    region_tag: "AU",
    lat: -33.87,
    lng: 151.21,
    reporting: {
      label: "Report to NSW Police",
      url: "https://portal.police.nsw.gov.au/s/online-services",
      phone: "000 (Emergency) / 131 444 (Police Assistance Line)",
    },
  },

  // ── Canada Cyber Centre (North America) ─────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "cccs-ca",
      authority_name: "Canada Cyber Centre",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "cert",
      base_url: "https://www.cyber.gc.ca",
    },
    feed_url: "https://www.cyber.gc.ca/en/alerts-advisories/feed",
    category: "cyber_advisory",
    region_tag: "CA",
    lat: 45.42,
    lng: -75.69,
    reporting: {
      label: "Report to Cyber Centre",
      url: "https://www.cyber.gc.ca/en/incident-management",
      email: "contact@cyber.gc.ca",
      phone: "1-833-292-3722",
    },
  },

  // ── RCMP Canada (North America) ─────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "rcmp-ca",
      authority_name: "RCMP Canada",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.rcmp-grc.gc.ca",
    },
    feed_url: "https://www.rcmp-grc.gc.ca/en/news/rss",
    category: "public_appeal",
    region_tag: "CA",
    lat: 45.40,
    lng: -75.70,
    reporting: {
      label: "Report to RCMP",
      url: "https://www.rcmp-grc.gc.ca/en/report-information-online",
      phone: "1-800-771-5401",
    },
  },

  // ── Policia Nacional Spain (Europe) ─────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "cnp-es",
      authority_name: "Policía Nacional Spain",
      country: "Spain",
      country_code: "ES",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.policia.es",
    },
    feed_url: "https://www.policia.es/rss/rss_prensa.xml",
    category: "public_appeal",
    region_tag: "ES",
    lat: 40.42,
    lng: -3.70,
    reporting: {
      label: "Report to Policía Nacional",
      url: "https://www.policia.es/colabora.php",
      phone: "091",
    },
  },

  // ── CERT-In India (Asia) ────────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "cert-in",
      authority_name: "CERT-In",
      country: "India",
      country_code: "IN",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.cert-in.org.in",
    },
    feed_url: "https://www.cert-in.org.in/s2cMainServlet?pageid=RSSFEED",
    category: "cyber_advisory",
    region_tag: "IN",
    lat: 28.61,
    lng: 77.21,
    reporting: {
      label: "Report to CERT-In",
      url: "https://www.cert-in.org.in/",
      email: "incident@cert-in.org.in",
      phone: "+91-11-24368572",
    },
  },

  // ── SingCERT Singapore (Asia) ───────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "singcert",
      authority_name: "SingCERT",
      country: "Singapore",
      country_code: "SG",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.csa.gov.sg",
    },
    feed_url: "https://www.csa.gov.sg/singcert/Alerts/rss",
    feed_urls: [
      "https://www.csa.gov.sg/singcert/Alerts/rss",
      "https://www.csa.gov.sg/alerts-and-advisories/alerts/rss",
      "https://www.csa.gov.sg/alerts-and-advisories/advisories/rss",
    ],
    category: "cyber_advisory",
    region_tag: "SG",
    lat: 1.29,
    lng: 103.85,
    reporting: {
      label: "Report to SingCERT",
      url: "https://www.csa.gov.sg/singcert/reporting",
      email: "singcert@csa.gov.sg",
      phone: "+65 6323 5052",
    },
  },

  // ── Singapore Police Force (Asia) ───────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "spf-sg",
      authority_name: "Singapore Police",
      country: "Singapore",
      country_code: "SG",
      region: "Asia",
      authority_type: "police",
      base_url: "https://www.police.gov.sg",
    },
    feed_url: "https://www.police.gov.sg/media-room/news/feed",
    feed_urls: [
      "https://www.police.gov.sg/media-room/news/feed",
      "https://www.police.gov.sg/rss",
      "https://www.police.gov.sg/media-room/news/rss.xml",
    ],
    category: "public_appeal",
    region_tag: "SG",
    lat: 1.31,
    lng: 103.84,
    reporting: {
      label: "Report to Singapore Police",
      url: "https://eservices.police.gov.sg/content/policehubhome/homepage/police-report.html",
      phone: "999 (Emergency) / 1800-255-0000 (Police Hotline)",
    },
  },

  // ── HKCERT Hong Kong (Asia) ─────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "hkcert",
      authority_name: "HKCERT",
      country: "Hong Kong",
      country_code: "HK",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.hkcert.org",
    },
    feed_url: "https://www.hkcert.org/rss",
    category: "cyber_advisory",
    region_tag: "HK",
    lat: 22.32,
    lng: 114.17,
    reporting: {
      label: "Report to HKCERT",
      url: "https://www.hkcert.org/report-incident",
      email: "hkcert@hkcert.org",
      phone: "+852 8105 6060",
    },
  },

  // ── SAPS South Africa (Africa) ──────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "saps-za",
      authority_name: "SAPS South Africa",
      country: "South Africa",
      country_code: "ZA",
      region: "Africa",
      authority_type: "police",
      base_url: "https://www.saps.gov.za",
    },
    feed_url: "https://www.saps.gov.za/newsroom/rss.php",
    category: "public_appeal",
    region_tag: "ZA",
    lat: -25.75,
    lng: 28.19,
    reporting: {
      label: "Report to SAPS",
      url: "https://www.saps.gov.za/resource_centre/contacts/contacts.php",
      phone: "10111 (Emergency) / 08600 10111 (Crime Stop)",
    },
  },
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "missing-children-za",
      authority_name: "Missing Children South Africa",
      country: "South Africa",
      country_code: "ZA",
      region: "Africa",
      authority_type: "public_safety_program",
      base_url: "https://missingchildren.org.za",
    },
    feed_url: "https://missingchildren.org.za/feed/",
    feed_urls: [
      "https://missingchildren.org.za/feed/",
      "https://missingchildren.org.za/category/missing-children/feed/",
      "https://missingchildren.org.za/category/cases/feed/",
    ],
    category: "missing_person",
    region_tag: "ZA",
    lat: -29.0,
    lng: 24.0,
    reporting: {
      label: "Report to Missing Children SA",
      url: "https://missingchildren.org.za/report/",
      phone: "+27 72 647 7464",
      notes: "Coordinate directly with SAPS in emergency situations.",
    },
  },

  // ── Crimestoppers UK (Europe) ───────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "crimestoppers-uk",
      authority_name: "Crimestoppers UK",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "public_safety_program",
      base_url: "https://www.crimestoppers-uk.org",
    },
    feed_url: "https://www.crimestoppers-uk.org/give-information/latest-news-feeds/rss",
    category: "public_appeal",
    region_tag: "GB",
    lat: 51.52,
    lng: -0.08,
    reporting: {
      label: "Report to Crimestoppers",
      url: "https://crimestoppers-uk.org/give-information",
      phone: "0800 555 111",
      notes: "100% anonymous. You can also report online.",
    },
  },

  // ── Japan NPA (Asia) ────────────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "npa-jp",
      authority_name: "Japan NPA",
      country: "Japan",
      country_code: "JP",
      region: "Asia",
      authority_type: "police",
      base_url: "https://www.npa.go.jp",
    },
    feed_url: "https://www.npa.go.jp/rss/index.xml",
    category: "public_safety",
    region_tag: "JP",
    lat: 35.69,
    lng: 139.75,
    reporting: {
      label: "Report to NPA Japan",
      url: "https://www.npa.go.jp/english/index.html",
      phone: "110 (Emergency)",
    },
  },

  // ── Gendarmerie France (Europe) ─────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "gendarmerie-fr",
      authority_name: "Gendarmerie France",
      country: "France",
      country_code: "FR",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.gendarmerie.interieur.gouv.fr",
    },
    feed_url: "https://www.gendarmerie.interieur.gouv.fr/rss",
    category: "public_appeal",
    region_tag: "FR",
    lat: 48.85,
    lng: 2.30,
    reporting: {
      label: "Report to Gendarmerie",
      url: "https://www.pre-plainte-en-ligne.gouv.fr/",
      phone: "17 (Emergency)",
    },
  },

  // ── Polisen Sweden (Europe) ─────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "polisen-se",
      authority_name: "Polisen Sweden",
      country: "Sweden",
      country_code: "SE",
      region: "Europe",
      authority_type: "police",
      base_url: "https://polisen.se",
    },
    feed_url: "https://polisen.se/aktuellt/rss/hela-landet/",
    category: "public_appeal",
    region_tag: "SE",
    lat: 59.33,
    lng: 18.07,
    reporting: {
      label: "Report to Polisen",
      url: "https://polisen.se/en/victims-of-crime/report-a-crime-online/",
      phone: "112 (Emergency) / 114 14 (Non-emergency)",
    },
  },

  // ── Politiet Norway (Europe) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "politiet-no",
      authority_name: "Politiet Norway",
      country: "Norway",
      country_code: "NO",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.politiet.no",
    },
    feed_url: "https://www.politiet.no/rss/",
    category: "public_appeal",
    region_tag: "NO",
    lat: 59.91,
    lng: 10.75,
    reporting: {
      label: "Report to Politiet",
      url: "https://www.politiet.no/en/services/report-an-offence/",
      phone: "112 (Emergency) / 02800 (Non-emergency)",
    },
  },

  // ── Policia Federal Brazil (South America) ──────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "pf-br",
      authority_name: "Polícia Federal Brazil",
      country: "Brazil",
      country_code: "BR",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.gov.br/pf",
    },
    feed_url: "https://www.gov.br/pf/pt-br/assuntos/noticias/@@rss",
    feed_urls: [
      "https://www.gov.br/pf/pt-br/assuntos/noticias/@@rss",
      "https://www.gov.br/pf/pt-br/rss",
      "https://www.gov.br/pf/pt-br/@@search?sort_on=Date&Subject:list=noticias&b_size=100&format=rss",
    ],
    category: "public_appeal",
    region_tag: "BR",
    lat: -15.79,
    lng: -47.88,
    reporting: {
      label: "Report to Polícia Federal",
      url: "https://www.gov.br/pf/pt-br/canais_atendimento/denuncia",
      phone: "190 (Emergency)",
    },
  },

  // ── Carabineros Chile (South America) ───────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "carabineros-cl",
      authority_name: "Carabineros Chile",
      country: "Chile",
      country_code: "CL",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.carabineros.cl",
    },
    feed_url: "https://www.carabineros.cl/feed/",
    feed_urls: [
      "https://www.carabineros.cl/feed/",
      "https://www.carabineros.cl/rss",
      "https://www.carabineros.cl/index.php/feed/",
    ],
    category: "public_appeal",
    region_tag: "CL",
    lat: -33.45,
    lng: -70.67,
    reporting: {
      label: "Report to Carabineros",
      url: "https://www.carabineros.cl/",
      phone: "133 (Emergency)",
    },
  },

  // ── Policía Nacional del Perú (South America) ───────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "pnp-pe",
      authority_name: "Policía Nacional del Perú",
      country: "Peru",
      country_code: "PE",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.policia.gob.pe",
    },
    feed_url: "https://www.policia.gob.pe/feed/",
    feed_urls: [
      "https://www.policia.gob.pe/feed/",
      "https://www.policia.gob.pe/rss",
      "https://www.gob.pe/institucion/pnp/noticias.rss",
    ],
    category: "public_appeal",
    region_tag: "PE",
    lat: -12.05,
    lng: -77.04,
    reporting: {
      label: "Report to PNP Peru",
      url: "https://www.policia.gob.pe/denuncia/",
      phone: "105 (Emergency)",
    },
  },

  // ── Policía Nacional Ecuador (South America) ────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "policia-ec",
      authority_name: "Policía Nacional Ecuador",
      country: "Ecuador",
      country_code: "EC",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.policia.gob.ec",
    },
    feed_url: "https://www.policia.gob.ec/feed/",
    feed_urls: [
      "https://www.policia.gob.ec/feed/",
      "https://www.policia.gob.ec/rss",
      "https://www.policia.gob.ec/category/noticias/feed/",
    ],
    category: "public_appeal",
    region_tag: "EC",
    lat: -0.18,
    lng: -78.47,
    reporting: {
      label: "Report to Policía Ecuador",
      url: "https://www.policia.gob.ec/servicios/",
      phone: "911 (Emergency) / 1800-DELITO",
    },
  },

  // ── Policía Boliviana (South America) ───────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "policia-bo",
      authority_name: "Policía Boliviana",
      country: "Bolivia",
      country_code: "BO",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.policia.bo",
    },
    feed_url: "https://www.policia.bo/feed/",
    feed_urls: [
      "https://www.policia.bo/feed/",
      "https://www.policia.bo/rss",
      "https://www.policia.bo/category/noticias/feed/",
    ],
    category: "public_appeal",
    region_tag: "BO",
    lat: -16.5,
    lng: -68.15,
    reporting: {
      label: "Report to Policía Boliviana",
      url: "https://www.policia.bo/",
      phone: "110 (Emergency)",
    },
  },

  // ── Policía Nacional Paraguay (South America) ───────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "policia-py",
      authority_name: "Policía Nacional Paraguay",
      country: "Paraguay",
      country_code: "PY",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.policianacional.gov.py",
    },
    feed_url: "https://www.policianacional.gov.py/feed/",
    feed_urls: [
      "https://www.policianacional.gov.py/feed/",
      "https://www.policianacional.gov.py/rss",
      "https://www.policianacional.gov.py/category/noticias/feed/",
    ],
    category: "public_appeal",
    region_tag: "PY",
    lat: -25.29,
    lng: -57.64,
    reporting: {
      label: "Report to Policía Paraguay",
      url: "https://www.policianacional.gov.py/",
      phone: "911 (Emergency)",
    },
  },

  // ── Cibercrimen Chile / PDI (South America) ─────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "pdi-ciberchile",
      authority_name: "PDI Chile Cibercrimen",
      country: "Chile",
      country_code: "CL",
      region: "South America",
      authority_type: "police",
      base_url: "https://www.pdichile.cl",
    },
    feed_url: "https://www.pdichile.cl/feed/",
    feed_urls: [
      "https://www.pdichile.cl/feed/",
      "https://www.pdichile.cl/rss",
      "https://www.pdichile.cl/instituci%C3%B3n/noticias/feed",
    ],
    category: "cyber_advisory",
    region_tag: "CL",
    lat: -33.45,
    lng: -70.66,
    reporting: {
      label: "Report Cybercrime to PDI",
      url: "https://www.pdichile.cl/",
      phone: "134 (PDI Emergency)",
    },
  },

  // ── Fiscalía Argentina (South America) ──────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "fiscales-ar",
      authority_name: "Ministerio Público Fiscal Argentina",
      country: "Argentina",
      country_code: "AR",
      region: "South America",
      authority_type: "regulatory",
      base_url: "https://www.fiscales.gob.ar",
    },
    feed_url: "https://www.fiscales.gob.ar/feed/",
    feed_urls: [
      "https://www.fiscales.gob.ar/feed/",
      "https://www.fiscales.gob.ar/category/noticias/feed/",
      "https://www.fiscales.gob.ar/category/cibercrimen/feed/",
    ],
    category: "public_safety",
    region_tag: "AR",
    lat: -34.61,
    lng: -58.38,
    reporting: {
      label: "Report to Fiscalía Argentina",
      url: "https://www.mpf.gob.ar/",
      phone: "137 (Emergency advisory line)",
    },
  },

  // ── NGO / Nonprofit: Missing Children Chile ─────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "missing-cl-ngo",
      authority_name: "Fundación Extraviados Chile",
      country: "Chile",
      country_code: "CL",
      region: "South America",
      authority_type: "public_safety_program",
      base_url: "https://www.extraviados.cl",
    },
    feed_url: "https://www.extraviados.cl/feed/",
    feed_urls: [
      "https://www.extraviados.cl/feed/",
      "https://www.extraviados.cl/category/casos-vigentes/feed/",
    ],
    category: "missing_person",
    region_tag: "CL",
    lat: -33.43,
    lng: -70.65,
    reporting: {
      label: "Report Missing Person in Chile",
      url: "https://www.extraviados.cl/",
      notes: "Coordinate with local police for urgent leads.",
    },
  },

  // ── FBI Seeking Information (US / North America) ────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "fbi-seeking",
      authority_name: "FBI Seeking Info",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.fbi.gov",
    },
    feed_url: "https://www.fbi.gov/feeds/seeking-information/rss.xml",
    category: "public_appeal",
    region_tag: "US",
    lat: 38.91,
    lng: -77.01,
    reporting: {
      label: "Submit a Tip to FBI",
      url: "https://tips.fbi.gov/",
      phone: "1-800-CALL-FBI (1-800-225-5324)",
      notes: "The FBI is seeking the public's assistance. If you have information, submit a tip.",
    },
  },

  // ── FBI Most Wanted (US / North America) ────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "fbi-mostwanted",
      authority_name: "FBI Most Wanted",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.fbi.gov",
    },
    feed_url: "https://www.fbi.gov/feeds/fbi-most-wanted/rss.xml",
    category: "wanted_suspect",
    region_tag: "US",
    lat: 38.89,
    lng: -77.02,
    reporting: {
      label: "Report Sighting to FBI",
      url: "https://tips.fbi.gov/",
      phone: "1-800-CALL-FBI (1-800-225-5324)",
      notes: "Do NOT attempt to apprehend. Call 911 immediately if in danger.",
    },
  },

  // ── Action Fraud UK (Europe) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "actionfraud-uk",
      authority_name: "Action Fraud UK",
      country: "United Kingdom",
      country_code: "GB",
      region: "Europe",
      authority_type: "police",
      base_url: "https://www.actionfraud.police.uk",
    },
    feed_url: "https://www.actionfraud.police.uk/rss",
    category: "fraud_alert",
    region_tag: "GB",
    lat: 51.50,
    lng: -0.12,
    reporting: {
      label: "Report Fraud to Action Fraud",
      url: "https://www.actionfraud.police.uk/reporting-fraud-and-cyber-crime",
      phone: "0300 123 2040",
    },
  },

  // ── CNA Singapore Crime (Asia) ──────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "cna-sg-crime",
      authority_name: "CNA Singapore Crime",
      country: "Singapore",
      country_code: "SG",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.channelnewsasia.com",
    },
    feed_url: "https://www.channelnewsasia.com/api/v1/rss-outbound-feed?_format=xml&category=6511",
    category: "public_safety",
    region_tag: "SG",
    lat: 1.35,
    lng: 103.82,
    reporting: {
      label: "Report Crime in Singapore",
      url: "https://eservices.police.gov.sg/content/policehubhome/homepage/police-report.html",
      phone: "999 (Emergency) / 1800-255-0000 (Police Hotline)",
    },
  },

  // ── Yonhap News Korea (Asia) ────────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "yonhap-kr",
      authority_name: "Yonhap News Korea",
      country: "South Korea",
      country_code: "KR",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://en.yna.co.kr",
    },
    feed_url: "https://en.yna.co.kr/RSS/news.xml",
    category: "public_safety",
    region_tag: "KR",
    lat: 37.57,
    lng: 126.98,
    reporting: {
      label: "Report Crime in South Korea",
      url: "https://www.police.go.kr/eng/index.do",
      phone: "112 (Emergency)",
    },
  },

  // ── NHK Japan News (Asia) ──────────────────────────────────────
  // In Japanese - auto-translated to English
  {
    type: "rss",
    source: {
      source_id: "nhk-jp",
      authority_name: "NHK Japan",
      country: "Japan",
      country_code: "JP",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www3.nhk.or.jp",
    },
    feed_url: "https://www3.nhk.or.jp/rss/news/cat1.xml",
    category: "public_safety",
    region_tag: "JP",
    lat: 35.67,
    lng: 139.71,
    reporting: {
      label: "Report to Japan Police",
      url: "https://www.npa.go.jp/english/index.html",
      phone: "110 (Emergency)",
    },
  },

  // ── SCMP Hong Kong (Asia) ──────────────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "scmp-hk",
      authority_name: "SCMP Hong Kong",
      country: "Hong Kong",
      country_code: "HK",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.scmp.com",
    },
    feed_url: "https://www.scmp.com/rss/5/feed",
    followRedirects: true,
    category: "public_safety",
    region_tag: "HK",
    lat: 22.28,
    lng: 114.16,
    reporting: {
      label: "Report Crime in Hong Kong",
      url: "https://www.police.gov.hk/ppp_en/contact_us.html",
      phone: "999 (Emergency)",
    },
  },

  // ── Straits Times Singapore (Asia) ──────────────────────────────
  {
    type: "rss",
    source: {
      source_id: "straitstimes-sg",
      authority_name: "Straits Times Singapore",
      country: "Singapore",
      country_code: "SG",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.straitstimes.com",
    },
    feed_url: "https://www.straitstimes.com/news/singapore/rss.xml",
    category: "public_safety",
    region_tag: "SG",
    lat: 1.30,
    lng: 103.84,
    reporting: {
      label: "Report Crime in Singapore",
      url: "https://eservices.police.gov.sg/content/policehubhome/homepage/police-report.html",
      phone: "999 (Emergency)",
    },
  },

  // ── Philippine National Police (Asia) ───────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "pnp-ph",
      authority_name: "PNP Philippines",
      country: "Philippines",
      country_code: "PH",
      region: "Asia",
      authority_type: "police",
      base_url: "https://www.pnp.gov.ph",
    },
    feed_url: "https://www.pnp.gov.ph/rss",
    feed_urls: [
      "https://www.pnp.gov.ph/rss",
      "https://www.pnp.gov.ph/feed/",
      "https://www.pnp.gov.ph/category/press-release/feed/",
    ],
    category: "public_appeal",
    region_tag: "PH",
    lat: 14.60,
    lng: 120.98,
    reporting: {
      label: "Report to PNP",
      url: "https://www.pnp.gov.ph/",
      phone: "117 (Emergency) / 8722-0650",
    },
  },

  // ── Royal Malaysia Police (Asia) ────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "pdrm-my",
      authority_name: "PDRM Malaysia",
      country: "Malaysia",
      country_code: "MY",
      region: "Asia",
      authority_type: "police",
      base_url: "https://www.pdrm.gov.my",
    },
    feed_url: "https://www.pdrm.gov.my/rss",
    feed_urls: [
      "https://www.pdrm.gov.my/rss",
      "https://www.rmp.gov.my/rss",
      "https://www.rmp.gov.my/feed/",
    ],
    category: "public_appeal",
    region_tag: "MY",
    lat: 3.14,
    lng: 101.69,
    reporting: {
      label: "Report to PDRM",
      url: "https://semakonline.rmp.gov.my/",
      phone: "999 (Emergency)",
    },
  },

  // ── Trinidad & Tobago Police (Caribbean) ────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "ttps",
      authority_name: "Trinidad & Tobago Police",
      country: "Trinidad and Tobago",
      country_code: "TT",
      region: "Caribbean",
      authority_type: "police",
      base_url: "https://www.ttps.gov.tt",
    },
    feed_url: "https://www.ttps.gov.tt/rss",
    category: "public_appeal",
    region_tag: "TT",
    lat: 10.65,
    lng: -61.50,
    reporting: {
      label: "Report to TTPS",
      url: "https://www.ttps.gov.tt/",
      phone: "999 (Emergency) / 555 (Crime Stoppers)",
    },
  },

  // ── Jamaica Constabulary Force (Caribbean) ──────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "jcf-jm",
      authority_name: "JCF Jamaica",
      country: "Jamaica",
      country_code: "JM",
      region: "Caribbean",
      authority_type: "police",
      base_url: "https://www.jcf.gov.jm",
    },
    feed_url: "https://www.jcf.gov.jm/rss",
    category: "public_appeal",
    region_tag: "JM",
    lat: 18.00,
    lng: -76.79,
    reporting: {
      label: "Report to JCF",
      url: "https://www.jcf.gov.jm/",
      phone: "119 (Emergency) / 311 (Crime Stop)",
    },
  },

  // ── Mexico FGR / Fiscalía (North America) ──────────────────────
  {
    type: "html-list",
    followRedirects: true,
    source: {
      source_id: "fgr-mx",
      authority_name: "FGR Mexico",
      country: "Mexico",
      country_code: "MX",
      region: "North America",
      authority_type: "police",
      base_url: "https://www.gob.mx/fgr",
    },
    feed_url: "https://www.gob.mx/fgr/archivo/prensa",
    feed_urls: [
      "https://www.gob.mx/fgr/archivo/prensa",
      "https://www.gob.mx/fgr/es/archivo/prensa",
      "https://www.gob.mx/fgr",
    ],
    include_keywords: [
      "desaparec",
      "se busca",
      "ficha",
      "recompensa",
      "secuestro",
      "privación de la libertad",
      "denuncia",
      "información",
      "investigación",
      "captura",
      "homicidio",
      "víctima",
      "feminicidio",
      "trata",
      "delincuencia",
      "cártel",
    ],
    exclude_keywords: ["agenda", "discurso", "evento", "licitación", "transparencia"],
    category: "public_appeal",
    region_tag: "MX",
    lat: 19.43,
    lng: -99.13,
    reporting: {
      label: "Report to FGR Mexico",
      url: "https://www.gob.mx/fgr",
      phone: "800-008-5400",
      notes: "Denuncia anónima / Anonymous tip line.",
    },
  },

  // ── Mexico AMBER Alert (North America) ──────────────────────────
  {
    type: "html-list",
    followRedirects: true,
    source: {
      source_id: "amber-mx",
      authority_name: "AMBER Alert Mexico",
      country: "Mexico",
      country_code: "MX",
      region: "North America",
      authority_type: "public_safety_program",
      base_url: "https://www.gob.mx/amber",
    },
    feed_url: "https://www.gob.mx/amber/archivo/acciones_y_programas",
    feed_urls: [
      "https://www.gob.mx/amber/archivo/acciones_y_programas",
      "https://www.gob.mx/amber/es/archivo/acciones_y_programas",
      "https://www.gob.mx/amber",
    ],
    include_keywords: [
      "alerta amber",
      "desaparec",
      "no localizado",
      "se busca",
      "ficha",
      "menor",
      "niña",
      "niño",
      "adolescente",
      "auxilio",
      "información",
    ],
    exclude_keywords: ["evento", "campaña", "conferencia", "manual", "material"],
    category: "missing_person",
    region_tag: "MX",
    lat: 19.44,
    lng: -99.14,
    reporting: {
      label: "Report Missing Child Mexico",
      url: "https://www.gob.mx/amber",
      phone: "800-008-5400",
      notes: "Alerta AMBER México",
    },
  },

  // ── Canada Missing Children (North America) ────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "missing-ca",
      authority_name: "Canada Missing Children",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "public_safety_program",
      base_url: "https://www.canadasmissing.ca",
    },
    feed_url: "https://www.canadasmissing.ca/rss/index-eng.xml",
    category: "missing_person",
    region_tag: "CA",
    lat: 45.43,
    lng: -75.68,
    reporting: {
      label: "Report Missing Person Canada",
      url: "https://www.canadasmissing.ca/index-eng.htm",
      phone: "1-866-KID-TIPS (1-866-543-8477)",
      notes: "Canadian Centre for Child Protection",
    },
  },

  // ── Korea Police (Asia) ─────────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "knpa-kr",
      authority_name: "Korea National Police",
      country: "South Korea",
      country_code: "KR",
      region: "Asia",
      authority_type: "police",
      base_url: "https://www.police.go.kr",
    },
    feed_url: "https://www.police.go.kr/eng/portal/rss/rss.do",
    category: "public_safety",
    region_tag: "KR",
    lat: 37.58,
    lng: 126.97,
    reporting: {
      label: "Report to Korean Police",
      url: "https://www.police.go.kr/eng/index.do",
      phone: "112 (Emergency)",
    },
  },

  // ── Thai CERT (Asia) ────────────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "thaicert",
      authority_name: "ThaiCERT",
      country: "Thailand",
      country_code: "TH",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.thaicert.or.th",
    },
    feed_url: "https://www.thaicert.or.th/RSS/feed-en.xml",
    feed_urls: [
      "https://www.thaicert.or.th/RSS/feed-en.xml",
      "https://www.thaicert.or.th/feed/",
    ],
    category: "cyber_advisory",
    region_tag: "TH",
    lat: 13.76,
    lng: 100.50,
    reporting: {
      label: "Report to ThaiCERT",
      url: "https://www.thaicert.or.th/",
      email: "op@thaicert.or.th",
    },
  },

  // ── MyCERT Malaysia (Asia) ──────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "mycert-my",
      authority_name: "MyCERT Malaysia",
      country: "Malaysia",
      country_code: "MY",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.mycert.org.my",
    },
    feed_url: "https://www.mycert.org.my/portal/rss",
    feed_urls: [
      "https://www.mycert.org.my/portal/rss",
      "https://www.mycert.org.my/feed",
    ],
    category: "cyber_advisory",
    region_tag: "MY",
    lat: 3.15,
    lng: 101.70,
    reporting: {
      label: "Report to MyCERT",
      url: "https://www.mycert.org.my/portal/report-incident",
      email: "mycert@cybersecurity.my",
    },
  },

  // ── BSSN Indonesia (Asia) ──────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "bssn-id",
      authority_name: "BSSN Indonesia",
      country: "Indonesia",
      country_code: "ID",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://bssn.go.id",
    },
    feed_url: "https://bssn.go.id/feed/",
    feed_urls: [
      "https://bssn.go.id/feed/",
      "https://bssn.go.id/category/peringatan-keamanan/feed/",
    ],
    category: "cyber_advisory",
    region_tag: "ID",
    lat: -6.20,
    lng: 106.82,
    reporting: {
      label: "Report to BSSN",
      url: "https://bssn.go.id/",
      notes: "Use official BSSN contact channels for incident reporting.",
    },
  },

  // ── PRIVATE SECTOR: BleepingComputer (Global) ──────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "bleepingcomputer",
      authority_name: "BleepingComputer",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "private_sector",
      base_url: "https://www.bleepingcomputer.com",
    },
    feed_url: "https://www.bleepingcomputer.com/feed/",
    category: "private_sector",
    region_tag: "US",
    lat: 40.71,
    lng: -74.01,
    reporting: {
      label: "Read Full Report",
      url: "https://www.bleepingcomputer.com",
      notes: "Private-sector cybersecurity news. Report incidents to relevant authorities.",
    },
  },

  // ── PRIVATE SECTOR: Krebs on Security (Global) ────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "krebsonsecurity",
      authority_name: "Krebs on Security",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "private_sector",
      base_url: "https://krebsonsecurity.com",
    },
    feed_url: "https://krebsonsecurity.com/feed/",
    category: "private_sector",
    region_tag: "US",
    lat: 38.90,
    lng: -77.04,
    reporting: {
      label: "Read Full Report",
      url: "https://krebsonsecurity.com",
      notes: "Investigative cybersecurity journalism by Brian Krebs.",
    },
  },

  // ── PRIVATE SECTOR: The Hacker News (Global) ──────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "thehackernews",
      authority_name: "The Hacker News",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "private_sector",
      base_url: "https://thehackernews.com",
    },
    feed_url: "https://feeds.feedburner.com/TheHackersNews",
    category: "private_sector",
    region_tag: "US",
    lat: 37.39,
    lng: -122.08,
    reporting: {
      label: "Read Full Report",
      url: "https://thehackernews.com",
      notes: "Cybersecurity news and analysis.",
    },
  },

  // ── PRIVATE SECTOR: DataBreaches.net (Global) ─────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "databreaches-net",
      authority_name: "DataBreaches.net",
      country: "United States",
      country_code: "US",
      region: "North America",
      authority_type: "private_sector",
      base_url: "https://databreaches.net",
    },
    feed_url: "https://databreaches.net/feed/",
    category: "private_sector",
    region_tag: "US",
    lat: 39.83,
    lng: -98.58,
    reporting: {
      label: "Read Full Report",
      url: "https://databreaches.net",
      notes: "Data breach tracking and reporting.",
    },
  },

  // ═══════════════════════════════════════════════════════════════════
  // EXPANDED COVERAGE — sources that openly ask for public help
  // ═══════════════════════════════════════════════════════════════════

  // ── Canada: Vancouver Police (North America) ──────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "vpd-ca",
      authority_name: "Vancouver Police Department",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "police",
      base_url: "https://vpd.ca",
    },
    feed_url: "https://vpd.ca/feed/",
    category: "public_appeal",
    region_tag: "CA",
    lat: 49.2827,
    lng: -123.1207,
    reporting: {
      label: "Submit a Tip to VPD",
      url: "https://vpd.ca/report-a-crime/",
      phone: "604-717-3321 (Non-Emergency)",
      notes: "911 for emergencies.",
    },
  },

  // ── Canada: Calgary Police Newsroom (North America) ───────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "calgary-police-ca",
      authority_name: "Calgary Police Service",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "police",
      base_url: "https://newsroom.calgary.ca",
    },
    feed_url: "https://newsroom.calgary.ca/feed/",
    category: "public_appeal",
    region_tag: "CA",
    lat: 51.0447,
    lng: -114.0719,
    reporting: {
      label: "Submit a Tip to Calgary Police",
      url: "https://www.calgarypolice.ca/contact-us",
      phone: "403-266-1234 (Non-Emergency)",
      notes: "911 for emergencies.",
    },
  },

  // ── Canada: CCCS Cyber Alerts API (North America) ─────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "cccs-ca-api",
      authority_name: "Canadian Centre for Cyber Security (Alerts)",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "cert",
      base_url: "https://www.cyber.gc.ca",
    },
    feed_url: "https://www.cyber.gc.ca/api/cccs/rss/v1/get?feed=alerts_advisories&lang=en",
    category: "cyber_advisory",
    region_tag: "CA",
    lat: 45.4215,
    lng: -75.6972,
    reporting: {
      label: "Report a Cyber Incident",
      url: "https://www.cyber.gc.ca/en/incident-management",
      phone: "1-833-CYBER-88",
    },
  },

  // ── Canada: CBC News (North America) ──────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "cbc-canada",
      authority_name: "CBC Canada News",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "public_safety_program",
      base_url: "https://www.cbc.ca",
    },
    feed_url: "https://www.cbc.ca/webfeed/rss/rss-canada",
    category: "public_safety",
    region_tag: "CA",
    lat: 43.6532,
    lng: -79.3832,
    reporting: {
      label: "CBC News Tips",
      url: "https://www.cbc.ca/news/tips",
    },
  },

  // ── Canada: Global News (North America) ───────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "globalnews-ca",
      authority_name: "Global News Canada",
      country: "Canada",
      country_code: "CA",
      region: "North America",
      authority_type: "public_safety_program",
      base_url: "https://globalnews.ca",
    },
    feed_url: "https://globalnews.ca/feed/",
    category: "public_safety",
    region_tag: "CA",
    lat: 45.5017,
    lng: -73.5673,
    reporting: {
      label: "Global News Tips",
      url: "https://globalnews.ca/pages/contact-us/",
    },
  },

  // ── Turkey: USOM / TR-CERT Cyber Alerts (Asia) ───────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "usom-tr",
      authority_name: "TR-CERT / USOM",
      country: "Turkey",
      country_code: "TR",
      region: "Asia",
      authority_type: "cert",
      base_url: "https://www.usom.gov.tr",
    },
    feed_url: "https://www.usom.gov.tr/rss/tehdit.rss",
    category: "cyber_advisory",
    region_tag: "TR",
    lat: 39.9334,
    lng: 32.8597,
    reporting: {
      label: "Report Cyber Incident to USOM",
      url: "https://www.usom.gov.tr/bildirim",
    },
  },

  // ── Israel: Times of Israel (Asia) ────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "timesofisrael-il",
      authority_name: "Times of Israel",
      country: "Israel",
      country_code: "IL",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.timesofisrael.com",
    },
    feed_url: "https://www.timesofisrael.com/feed/",
    category: "public_safety",
    region_tag: "IL",
    lat: 31.7683,
    lng: 35.2137,
    reporting: {
      label: "Israel Police Tips",
      url: "https://www.police.gov.il/en",
      phone: "100 (Israel Police)",
    },
  },

  // ── Middle East Eye (Asia) ────────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "middleeasteye",
      authority_name: "Middle East Eye",
      country: "Qatar",
      country_code: "QA",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.middleeasteye.net",
    },
    feed_url: "https://www.middleeasteye.net/rss",
    category: "public_safety",
    region_tag: "ME",
    lat: 25.2854,
    lng: 51.531,
    reporting: {
      label: "Middle East Eye Tips",
      url: "https://www.middleeasteye.net/contact",
    },
  },

  // ── Turkey: Daily Sabah (Asia) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "dailysabah-tr",
      authority_name: "Daily Sabah Turkey",
      country: "Turkey",
      country_code: "TR",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.dailysabah.com",
    },
    feed_url: "https://www.dailysabah.com/rssFeed/turkey",
    category: "public_safety",
    region_tag: "TR",
    lat: 41.0082,
    lng: 28.9784,
    reporting: {
      label: "Daily Sabah Contact",
      url: "https://www.dailysabah.com/contact",
    },
  },

  // ── China: Global Times (Asia) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "globaltimes-cn",
      authority_name: "Global Times China",
      country: "China",
      country_code: "CN",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.globaltimes.cn",
    },
    feed_url: "https://www.globaltimes.cn/rss/outbrain.xml",
    category: "public_safety",
    region_tag: "CN",
    lat: 39.9042,
    lng: 116.4074,
    reporting: {
      label: "Global Times Contact",
      url: "https://www.globaltimes.cn/about-us/contact-us.html",
    },
  },

  // ── India: India Today Crime (Asia) ───────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "indiatoday-crime",
      authority_name: "India Today Crime",
      country: "India",
      country_code: "IN",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.indiatoday.in",
    },
    feed_url: "https://www.indiatoday.in/rss/1786661",
    category: "public_safety",
    region_tag: "IN",
    lat: 28.6139,
    lng: 77.209,
    reporting: {
      label: "India Crime Tips",
      url: "https://cybercrime.gov.in/",
      phone: "112 (India Emergency)",
    },
  },

  // ── India: NDTV India News (Asia) ─────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "ndtv-in",
      authority_name: "NDTV India News",
      country: "India",
      country_code: "IN",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.ndtv.com",
    },
    feed_url: "https://feeds.feedburner.com/ndtvnews-india-news",
    category: "public_safety",
    region_tag: "IN",
    lat: 19.076,
    lng: 72.8777,
    reporting: {
      label: "NDTV News Tips",
      url: "https://www.ndtv.com/page/contact-us",
      phone: "112 (India Emergency)",
    },
  },

  // ── India: Hindustan Times (Asia) ─────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "hindustantimes-in",
      authority_name: "Hindustan Times India",
      country: "India",
      country_code: "IN",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.hindustantimes.com",
    },
    feed_url: "https://www.hindustantimes.com/feeds/rss/india-news/rssfeed.xml",
    category: "public_safety",
    region_tag: "IN",
    lat: 12.9716,
    lng: 77.5946,
    reporting: {
      label: "Hindustan Times Tips",
      url: "https://www.hindustantimes.com/contact-us",
      phone: "112 (India Emergency)",
    },
  },

  // ── Vietnam: VnExpress International (Asia) ───────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "vnexpress-vn",
      authority_name: "VnExpress International",
      country: "Vietnam",
      country_code: "VN",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://e.vnexpress.net",
    },
    feed_url: "https://e.vnexpress.net/rss/news.rss",
    category: "public_safety",
    region_tag: "VN",
    lat: 21.0278,
    lng: 105.8342,
    reporting: {
      label: "Vietnam Police Tips",
      url: "https://congan.com.vn/",
      phone: "113 (Vietnam Police)",
    },
  },

  // ── Laos: Laotian Times (Asia) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "laotiantimes-la",
      authority_name: "Laotian Times",
      country: "Laos",
      country_code: "LA",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://laotiantimes.com",
    },
    feed_url: "https://laotiantimes.com/feed/",
    category: "public_safety",
    region_tag: "LA",
    lat: 17.9757,
    lng: 102.6331,
    reporting: {
      label: "Laotian Times Contact",
      url: "https://laotiantimes.com/contact/",
    },
  },

  // ── Thailand: Bangkok Post (Asia) ─────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "bangkokpost-th",
      authority_name: "Bangkok Post",
      country: "Thailand",
      country_code: "TH",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.bangkokpost.com",
    },
    feed_url: "https://www.bangkokpost.com/rss/data/topstories.xml",
    category: "public_safety",
    region_tag: "TH",
    lat: 13.7563,
    lng: 100.5018,
    reporting: {
      label: "Thailand Police Tips",
      url: "https://www.royalthaipolice.go.th/",
      phone: "191 (Thailand Police)",
    },
  },

  // ── Philippines: Rappler (Asia) ───────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "rappler-ph",
      authority_name: "Rappler Philippines",
      country: "Philippines",
      country_code: "PH",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://www.rappler.com",
    },
    feed_url: "https://www.rappler.com/feed/",
    category: "public_safety",
    region_tag: "PH",
    lat: 14.5995,
    lng: 120.9842,
    reporting: {
      label: "PNP Philippines Tips",
      url: "https://www.pnp.gov.ph/",
      phone: "117 (PH Emergency)",
    },
  },

  // ── Indonesia: Tempo English (Asia) ───────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "tempo-id",
      authority_name: "Tempo Indonesia",
      country: "Indonesia",
      country_code: "ID",
      region: "Asia",
      authority_type: "public_safety_program",
      base_url: "https://en.tempo.co",
    },
    feed_url: "https://rss.tempo.co/en/",
    category: "public_safety",
    region_tag: "ID",
    lat: -6.2088,
    lng: 106.8456,
    reporting: {
      label: "Indonesia Police Tips",
      url: "https://www.polri.go.id/",
      phone: "110 (Indonesia Police)",
    },
  },

  // ── Papua New Guinea: Post-Courier (Oceania) ─────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "postcourier-pg",
      authority_name: "Post-Courier PNG",
      country: "Papua New Guinea",
      country_code: "PG",
      region: "Oceania",
      authority_type: "public_safety_program",
      base_url: "https://www.postcourier.com.pg",
    },
    feed_url: "https://www.postcourier.com.pg/feed/",
    category: "public_safety",
    region_tag: "PG",
    lat: -6.3149,
    lng: 147.1802,
    reporting: {
      label: "PNG Police",
      url: "https://www.rpngc.gov.pg/",
      phone: "000 (PNG Emergency)",
    },
  },

  // ── Fiji: Fiji Times (Oceania) ────────────────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "fijitimes-fj",
      authority_name: "Fiji Times",
      country: "Fiji",
      country_code: "FJ",
      region: "Oceania",
      authority_type: "public_safety_program",
      base_url: "https://www.fijitimes.com",
    },
    feed_url: "https://www.fijitimes.com/feed/",
    category: "public_safety",
    region_tag: "FJ",
    lat: -18.1416,
    lng: 178.4419,
    reporting: {
      label: "Fiji Police",
      url: "https://www.police.gov.fj/",
      phone: "917 (Fiji Police)",
    },
  },

  // ── Pacific Islands: RNZ Pacific (Oceania) ────────────────────────
  {
    type: "rss",
    followRedirects: true,
    source: {
      source_id: "rnz-pacific",
      authority_name: "RNZ Pacific",
      country: "New Zealand",
      country_code: "NZ",
      region: "Oceania",
      authority_type: "public_safety_program",
      base_url: "https://www.rnz.co.nz",
    },
    feed_url: "https://www.rnz.co.nz/rss/pacific.xml",
    category: "public_safety",
    region_tag: "NZ",
    lat: -15.3767,
    lng: 166.9592,
    reporting: {
      label: "RNZ Pacific Contact",
      url: "https://www.rnz.co.nz/about/contact",
    },
  },
];

function decodeXml(value) {
  if (!value) return "";
  return value
    .replace(/<!\[CDATA\[/g, "")
    .replace(/\]\]>/g, "")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .trim();
}

function getTag(block, tag) {
  const regex = new RegExp(`<${tag}[^>]*>([\\s\\S]*?)<\\/${tag}>`, "i");
  const match = block.match(regex);
  return match ? decodeXml(match[1]) : "";
}

function getAtomLink(block) {
  const alternate = block.match(/<link[^>]*rel=["']alternate["'][^>]*>/i);
  const linkTag = alternate?.[0] ?? block.match(/<link[^>]*>/i)?.[0];
  if (!linkTag) return "";
  const hrefMatch = linkTag.match(/href=["']([^"']+)["']/i);
  return hrefMatch ? decodeXml(hrefMatch[1]) : "";
}

function getTagValues(block, tag) {
  const regex = new RegExp(`<${tag}[^>]*>([\\s\\S]*?)<\\/${tag}>`, "gi");
  return [...block.matchAll(regex)]
    .map((match) => decodeXml(match[1]))
    .filter(Boolean);
}

function getAuthor(block) {
  const atomAuthor = block.match(
    /<author[^>]*>[\s\S]*?<name[^>]*>([\s\S]*?)<\/name>[\s\S]*?<\/author>/i
  );
  if (atomAuthor?.[1]) {
    return decodeXml(atomAuthor[1]);
  }
  return getTag(block, "author") || getTag(block, "dc:creator") || getTag(block, "creator");
}

function getSummary(block) {
  return (
    getTag(block, "description") ||
    getTag(block, "summary") ||
    getTag(block, "content") ||
    getTag(block, "content:encoded")
  );
}

function getCategories(block) {
  const rssCategories = getTagValues(block, "category");
  const atomCategories = [...block.matchAll(/<category[^>]*term=["']([^"']+)["'][^>]*\/?>/gi)]
    .map((match) => decodeXml(match[1]))
    .filter(Boolean);
  return [...rssCategories, ...atomCategories];
}

function parseItems(xml) {
  if (xml.includes("<feed")) {
    const entries = [...xml.matchAll(/<entry[\s\S]*?<\/entry>/gi)].map((m) => m[0]);
    return entries.map((entry) => ({
      title: getTag(entry, "title"),
      link: getAtomLink(entry),
      published: getTag(entry, "published") || getTag(entry, "updated"),
      author: getAuthor(entry),
      summary: getSummary(entry),
      tags: getCategories(entry),
    }));
  }

  const items = [...xml.matchAll(/<item[\s\S]*?<\/item>/gi)].map((m) => m[0]);
  return items.map((item) => ({
    title: getTag(item, "title"),
    link: getTag(item, "link") || getTag(item, "guid"),
    published: getTag(item, "pubDate") || getTag(item, "dc:date"),
    author: getAuthor(item),
    summary: getSummary(item),
    tags: getCategories(item),
  }));
}

const NEWS_MEDIA_SOURCE_IDS = new Set([
  "cna-sg-crime",
  "yonhap-kr",
  "nhk-jp",
  "scmp-hk",
  "jamaica-observer",
  "straitstimes-sg",
]);

const NEWS_MEDIA_DOMAINS = [
  "channelnewsasia.com",
  "yna.co.kr",
  "nhk.or.jp",
  "scmp.com",
  "jamaicaobserver.com",
  "straitstimes.com",
];

const TECHNICAL_SIGNAL_PATTERNS = [
  /\bcve-\d{4}-\d{4,7}\b/i,
  /\b(?:ioc|iocs|indicator(?:s)? of compromise)\b/i,
  /\b(?:tactic|technique|ttp|mitre)\b/i,
  /\b(?:hash|sha-?256|sha-?1|md5|yara|sigma)\b/i,
  /\b(?:ip(?:v4|v6)?|domain|url|hostname|command and control|c2)\b/i,
  /\b(?:vulnerability|exploit(?:ation)?|zero-?day|patch|mitigation|workaround)\b/i,
];

const INCIDENT_DISCLOSURE_PATTERNS = [
  /\b(?:breach|data leak|compromis(?:e|ed)|intrusion|unauthori[sz]ed access)\b/i,
  /\b(?:ransomware|malware|botnet|ddos|phishing|credential theft)\b/i,
  /\b(?:attack|attacked|target(?:ed|ing)|incident response|security incident)\b/i,
  /\b(?:arrest(?:ed)?|charged|indicted|wanted|fugitive|missing person|kidnapp(?:ed|ing)|homicide)\b/i,
];

const ACTIONABLE_PATTERNS = [
  /\b(?:report|submit (?:a )?tip|contact|hotline|phone|email)\b/i,
  /\b(?:apply update|upgrade|disable|block|monitor|detect|investigate)\b/i,
  /\b(?:advisory|alert|warning|incident notice|public appeal)\b/i,
];

const NARRATIVE_NEWS_PATTERNS = [
  /\b(?:opinion|editorial|commentary|analysis|explainer|podcast|interview)\b/i,
  /\b(?:what we know|live updates|behind the scenes|feature story)\b/i,
  /\b(?:market reaction|share price|investor)\b/i,
];

const GENERAL_NEWS_PATTERNS = [
  /\b(?:announces?|launche[sd]?|conference|summit|webinar|event|awareness month)\b/i,
  /\b(?:ceremony|speech|statement|newsletter|weekly roundup)\b/i,
  /\b(?:partnership|memorandum|mou|initiative|campaign)\b/i,
];

const SECURITY_CONTEXT_PATTERNS = [
  /\b(?:cyber|cybersecurity|infosec|information security|it security)\b/i,
  /\b(?:security posture|security controls?|threat intelligence)\b/i,
  /\b(?:vulnerability|exploit|patch|advisory|defend|defensive)\b/i,
  /\b(?:soc|siem|incident response|malware analysis)\b/i,
];

const ASSISTANCE_REQUEST_PATTERNS = [
  /\b(?:report(?:\s+a)?(?:\s+crime)?|submit (?:a )?tip|tip[-\s]?off)\b/i,
  /\b(?:contact (?:police|authorities|law enforcement)|hotline|helpline)\b/i,
  /\b(?:if you have information|seeking information|appeal for help)\b/i,
  /\b(?:missing|wanted|fugitive|amber alert)\b/i,
];

const IMPACT_SPECIFICITY_PATTERNS = [
  /\b(?:affected|impact(?:ed)?|disrupt(?:ed|ion)|outage|service interruption)\b/i,
  /\b(?:records|accounts|systems|devices|endpoints|victims|organizations)\b/i,
  /\b(?:on\s+\d{1,2}\s+\w+\s+\d{4}|timeline|between\s+\d{1,2}:\d{2})\b/i,
  /\b\d{2,}\s+(?:records|users|systems|devices|victims|organizations)\b/i,
];

function clamp01(value) {
  const numeric = Number.isFinite(value) ? value : 0.42;
  return Math.max(0, Math.min(1, numeric));
}

function thresholdForAlert(alert, defaultThreshold) {
  const category = String(alert?.category ?? "").toLowerCase();
  if (category === "missing_person") {
    return clamp01(MISSING_PERSON_RELEVANCE_THRESHOLD);
  }
  return defaultThreshold;
}

function extractDomain(urlValue) {
  try {
    return new URL(String(urlValue)).hostname.toLowerCase();
  } catch {
    return "";
  }
}

function isNewsMediaSource(alert) {
  const sourceId = String(alert?.source_id ?? "").toLowerCase();
  if (NEWS_MEDIA_SOURCE_IDS.has(sourceId)) {
    return true;
  }
  const host = extractDomain(alert?.canonical_url);
  return NEWS_MEDIA_DOMAINS.some((domain) => host.includes(domain));
}

function inferPublicationType(alert, metaHints = {}) {
  const authorityType = String(alert?.source?.authority_type ?? "").toLowerCase();
  if (isNewsMediaSource(alert)) return "news_media";
  if (authorityType === "cert") return "cert_advisory";
  if (authorityType === "police") return "law_enforcement";
  if (authorityType === "intelligence" || authorityType === "national_security") {
    return "security_bulletin";
  }
  if (authorityType === "public_safety_program") return "public_safety_bulletin";
  if (metaHints.feedType === "kev-json" || metaHints.feedType === "interpol-red-json") {
    return "structured_incident_feed";
  }
  return "official_update";
}

function hasAnyPattern(text, patterns) {
  return patterns.some((pattern) => pattern.test(text));
}

function scoreIncidentRelevance(alert, context = {}) {
  const title = String(alert?.title ?? "");
  const summary = String(context.summary ?? "");
  const author = String(context.author ?? "");
  const tags = Array.isArray(context.tags) ? context.tags.map((t) => String(t)) : [];
  const text = `${title}\n${summary}\n${author}\n${tags.join(" ")}\n${alert?.canonical_url ?? ""}`.toLowerCase();
  const publicationType = inferPublicationType(alert, context.metaHints ?? {});
  const signals = [];
  let score = 0.5;

  const addSignal = (delta, reason) => {
    score += delta;
    signals.push(`${delta >= 0 ? "+" : ""}${delta.toFixed(2)} ${reason}`);
  };

  if (publicationType === "news_media") {
    addSignal(-0.16, "publication type leans general-news");
  } else if (publicationType === "cert_advisory" || publicationType === "structured_incident_feed") {
    addSignal(0.08, "source metadata is incident-oriented");
  } else if (publicationType === "law_enforcement") {
    addSignal(0.06, "law-enforcement source metadata");
  }

  if (alert.category === "cyber_advisory") addSignal(0.09, "cyber advisory category");
  if (alert.category === "wanted_suspect" || alert.category === "missing_person") {
    addSignal(0.09, "law-enforcement incident category");
  }
  if (
    alert.category === "humanitarian_tasking" ||
    alert.category === "conflict_monitoring" ||
    alert.category === "humanitarian_security"
  ) {
    addSignal(0.08, "humanitarian incident/tasking category");
  }
  if (alert.category === "education_digital_capacity") {
    addSignal(0.07, "education and digital capacity category");
  }
  if (alert.category === "fraud_alert") addSignal(0.07, "fraud incident category");

  const hasTechnical = hasAnyPattern(text, TECHNICAL_SIGNAL_PATTERNS);
  const hasIncidentDisclosure = hasAnyPattern(text, INCIDENT_DISCLOSURE_PATTERNS);
  const hasActionable = hasAnyPattern(text, ACTIONABLE_PATTERNS);
  const hasSpecificImpact = hasAnyPattern(text, IMPACT_SPECIFICITY_PATTERNS);
  const hasNarrative = hasAnyPattern(text, NARRATIVE_NEWS_PATTERNS);
  const hasGeneralNews = hasAnyPattern(text, GENERAL_NEWS_PATTERNS);
  const looksLikeBlog = isBlogAlert(alert);

  if (hasTechnical) addSignal(0.16, "technical indicators or tactics present");
  if (hasIncidentDisclosure) addSignal(0.16, "incident/crime disclosure language");
  if (hasActionable) addSignal(0.1, "contains response/reporting actions");
  if (hasSpecificImpact) addSignal(0.08, "specific impact/timeline/system details");

  if (hasNarrative) addSignal(-0.18, "opinion/commentary phrasing");
  if (hasGeneralNews) addSignal(-0.12, "general institutional/news language");
  if (looksLikeBlog) addSignal(-0.1, "blog-style structure");

  if (!hasTechnical && !hasIncidentDisclosure && (hasNarrative || hasGeneralNews)) {
    addSignal(-0.08, "weak incident evidence relative to narrative cues");
  }

  const freshnessHours = Number(alert?.freshness_hours ?? 0);
  if (freshnessHours > 0 && freshnessHours <= 24 && (hasIncidentDisclosure || hasTechnical)) {
    addSignal(0.04, "fresh post with potential early-warning signal");
  }

  const threshold = clamp01(INCIDENT_RELEVANCE_THRESHOLD);
  const relevance = Number(clamp01(score).toFixed(3));
  const distance = Math.abs(relevance - threshold);
  const confidence =
    distance >= 0.25 ? "high" : distance >= 0.1 ? "medium" : "low";
  const disposition = relevance >= threshold ? "retained" : "filtered_review";

  return {
    relevance_score: relevance,
    threshold,
    confidence,
    disposition,
    publication_type: publicationType,
    weak_signals: signals.slice(0, 12),
    metadata: {
      author: author || undefined,
      tags: tags.slice(0, 8),
    },
  };
}

const BLOG_FILTER_EXEMPT_SOURCES = new Set([
  "bleepingcomputer",
  "krebsonsecurity",
  "thehackernews",
  "databreaches-net",
  // News sources that amplify calls for help
  "cbc-canada",
  "globalnews-ca",
  "timesofisrael-il",
  "middleeasteye",
  "dailysabah-tr",
  "globaltimes-cn",
  "indiatoday-crime",
  "ndtv-in",
  "hindustantimes-in",
  "vnexpress-vn",
  "laotiantimes-la",
  "bangkokpost-th",
  "rappler-ph",
  "tempo-id",
  "postcourier-pg",
  "fijitimes-fj",
  "rnz-pacific",
]);

function isBlogContent(item, sourceId) {
  if (sourceId && BLOG_FILTER_EXEMPT_SOURCES.has(sourceId)) return false;
  const title = String(item?.title ?? "").toLowerCase();
  const link = String(item?.link ?? "").toLowerCase();
  if (/\bblog\b/.test(title)) return true;
  if (/\/blog(s)?(\/|$)/.test(link)) return true;
  if (link.includes("medium.com")) return true;
  if (link.includes("wordpress.com")) return true;
  return false;
}

function isBlogAlert(alert) {
  if (BLOG_FILTER_EXEMPT_SOURCES.has(alert?.source_id)) return false;
  const title = String(alert?.title ?? "").toLowerCase();
  const link = String(alert?.canonical_url ?? "").toLowerCase();
  if (/\bblog\b/.test(title)) return true;
  if (/\/blog(s)?(\/|$)/.test(link)) return true;
  if (link.includes("medium.com")) return true;
  if (link.includes("wordpress.com")) return true;
  return false;
}

function isInformational(title) {
  const t = title.toLowerCase();
  const keywords = [
    "traffic",
    "road",
    "highway",
    "motorway",
    "lane",
    "closure",
    "closed",
    "detour",
    "accident",
    "crash",
    "collision",
    "vehicle",
    "multi-vehicle",
    "rollover",
    "roadworks",
    "road work",
  ];
  return keywords.some((word) => t.includes(word));
}

function isSecurityInformationalNews(alert, context = {}) {
  const title = String(alert?.title ?? "");
  const summary = String(context.summary ?? "");
  const author = String(context.author ?? "");
  const tags = Array.isArray(context.tags) ? context.tags.map((t) => String(t)) : [];
  const text = `${title}\n${summary}\n${author}\n${tags.join(" ")}\n${alert?.canonical_url ?? ""}`.toLowerCase();
  const publicationType = inferPublicationType(alert, context.metaHints ?? {});
  const authorityType = String(alert?.source?.authority_type ?? "").toLowerCase();

  const hasSecurityContext = hasAnyPattern(text, SECURITY_CONTEXT_PATTERNS);
  const hasIncidentOrCrime = hasAnyPattern(text, INCIDENT_DISCLOSURE_PATTERNS);
  const hasHelpRequest = hasAnyPattern(text, ASSISTANCE_REQUEST_PATTERNS);
  const hasGeneralNews = hasAnyPattern(text, GENERAL_NEWS_PATTERNS);
  const hasNarrative = hasAnyPattern(text, NARRATIVE_NEWS_PATTERNS);
  const hasImpactSpecifics = hasAnyPattern(text, IMPACT_SPECIFICITY_PATTERNS);

  const sourceIsSecurityRelevant =
    alert?.category === "cyber_advisory" ||
    alert?.category === "private_sector" ||
    publicationType === "cert_advisory" ||
    authorityType === "cert" ||
    authorityType === "private_sector" ||
    authorityType === "regulatory";

  return (
    sourceIsSecurityRelevant &&
    hasSecurityContext &&
    !hasIncidentOrCrime &&
    !hasHelpRequest &&
    !hasImpactSpecifics &&
    (hasGeneralNews || hasNarrative || publicationType === "news_media")
  );
}

function normalizeInformationalSecurityAlert(alert, context = {}) {
  if (!isSecurityInformationalNews(alert, context)) return alert;
  const baseThreshold = clamp01(INCIDENT_RELEVANCE_THRESHOLD);
  const currentScore = Number(alert?.triage?.relevance_score ?? 0);
  const nextScore = Math.max(currentScore, baseThreshold);
  return {
    ...alert,
    category: "informational",
    severity: "info",
    triage: {
      ...(alert?.triage ?? {}),
      relevance_score: Number(nextScore.toFixed(3)),
      threshold: baseThreshold,
      confidence: "medium",
      disposition: "retained",
      weak_signals: [
        "reclassified as informational security/cybersecurity update",
        ...((alert?.triage?.weak_signals ?? []).slice(0, 10)),
      ],
    },
  };
}

function inferSeverity(title, fallback) {
  const t = title.toLowerCase();
  if (isInformational(t)) return "info";
  // Explicit severity keywords
  if (t.includes("critical") || t.includes("emergency") || t.includes("zero-day") || t.includes("0-day")) return "critical";
  if (t.includes("ransomware") || t.includes("actively exploited") || t.includes("exploitation")) return "critical";
  if (t.includes("breach") || t.includes("data leak") || t.includes("crypto heist") || t.includes("million stolen")) return "critical";
  if (t.includes("hack") || t.includes("compromise") || t.includes("vulnerability")) return "high";
  if (t.includes("high") || t.includes("severe") || t.includes("urgent")) return "high";
  if (t.includes("wanted") || t.includes("fugitive") || t.includes("murder") || t.includes("homicide")) return "critical";
  if (t.includes("missing") || t.includes("amber alert") || t.includes("kidnap")) return "critical";
  if (t.includes("fatal") || t.includes("death") || t.includes("shooting")) return "high";
  if (t.includes("fraud") || t.includes("scam") || t.includes("phishing")) return "high";
  if (t.includes("arrested") || t.includes("charged") || t.includes("sentenced")) return "medium";
  if (t.includes("medium") || t.includes("moderate")) return "medium";
  if (t.includes("low") || t.includes("informational")) return "info";
  return fallback;
}

function defaultSeverity(category) {
  switch (category) {
    case "informational":
      return "info";
    case "cyber_advisory":
      return "high";
    case "wanted_suspect":
      return "critical";
    case "missing_person":
      return "critical";
    case "public_appeal":
      return "high";
    case "humanitarian_tasking":
      return "high";
    case "conflict_monitoring":
      return "medium";
    case "humanitarian_security":
      return "high";
    case "education_digital_capacity":
      return "medium";
    case "public_safety":
      return "medium";
    case "private_sector":
      return "high";
    default:
      return "medium";
  }
}

function parseDate(value) {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function isFresh(date, now) {
  const cutoff = now.getTime() - MAX_AGE_DAYS * 86400000;
  return date.getTime() >= cutoff;
}

function hashId(value) {
  return crypto.createHash("sha1").update(value).digest("hex").slice(0, 12);
}

function hashToUnit(value) {
  const hex = crypto.createHash("sha1").update(value).digest("hex").slice(0, 8);
  return Number.parseInt(hex, 16) / 0xffffffff;
}

function normalizeHeadline(value) {
  return String(value ?? "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
}

function compareAlertPreference(a, b) {
  const scoreA = Number(a?.triage?.relevance_score ?? 0);
  const scoreB = Number(b?.triage?.relevance_score ?? 0);
  if (scoreA !== scoreB) return scoreB - scoreA;
  const seenA = new Date(a?.first_seen ?? 0).getTime();
  const seenB = new Date(b?.first_seen ?? 0).getTime();
  if (seenA !== seenB) return seenB - seenA;
  const urlLenA = String(a?.canonical_url ?? "").length;
  const urlLenB = String(b?.canonical_url ?? "").length;
  return urlLenA - urlLenB;
}

function buildVariantCollapseKey(alert) {
  const titleNorm = normalizeHeadline(alert?.title);
  if (!titleNorm || titleNorm.length < 24) return null;
  const sourceId = String(alert?.source_id ?? "").toLowerCase();
  if (!sourceId) return null;
  try {
    const url = new URL(String(alert?.canonical_url ?? ""));
    const host = url.hostname.toLowerCase().replace(/^www\./, "");
    const path = url.pathname.replace(/\/+$/, "");
    const segments = path.split("/").filter(Boolean);
    const leaf = segments[segments.length - 1] ?? "";
    if (!/-\d+$/.test(leaf)) return null;
    const familyLeaf = leaf.replace(/-\d+$/, "");
    const familyPath = `/${segments.slice(0, -1).concat(familyLeaf).join("/")}`;
    return `${sourceId}|${host}${familyPath}|${titleNorm}`;
  } catch {
    return null;
  }
}

function collapseRecurringHeadlineVariants(alerts) {
  const byVariant = new Map();
  const passthrough = [];
  for (const alert of alerts) {
    const key = buildVariantCollapseKey(alert);
    if (!key) {
      passthrough.push(alert);
      continue;
    }
    const list = byVariant.get(key) ?? [];
    list.push(alert);
    byVariant.set(key, list);
  }

  const kept = [...passthrough];
  const suppressed = [];
  for (const list of byVariant.values()) {
    if (list.length === 1) {
      kept.push(list[0]);
      continue;
    }
    list.sort(compareAlertPreference);
    kept.push(list[0]);
    suppressed.push(...list.slice(1));
  }
  return { kept, suppressed };
}

function summarizeTitleDuplicates(alerts) {
  const counts = new Map();
  for (const alert of alerts) {
    const key = normalizeHeadline(alert?.title);
    if (!key) continue;
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return [...counts.entries()]
    .filter(([, count]) => count > 1)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 25)
    .map(([title, count]) => ({ title, count }));
}

function jitterCoords(lat, lng, seed, minRadiusKm = 22, maxRadiusKm = 77) {
  // Spread alerts around a base point so multiple notices don't collapse into one dot.
  const angle = hashToUnit(`${seed}:a`) * Math.PI * 2;
  const radiusKm = minRadiusKm + hashToUnit(`${seed}:r`) * Math.max(1, maxRadiusKm - minRadiusKm);
  const dLat = (radiusKm / 111.32) * Math.cos(angle);
  const cosLat = Math.max(0.2, Math.cos((lat * Math.PI) / 180));
  const dLng = (radiusKm / (111.32 * cosLat)) * Math.sin(angle);
  const outLat = Math.max(-89.5, Math.min(89.5, lat + dLat));
  let outLng = lng + dLng;
  if (outLng > 180) outLng -= 360;
  if (outLng < -180) outLng += 360;
  return { lat: Number(outLat.toFixed(5)), lng: Number(outLng.toFixed(5)) };
}

function escapeRegex(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function inferUSStateCoords(text) {
  const haystackRaw = ` ${String(text ?? "").toLowerCase()} `;
  const haystack = haystackRaw.replace(/\./g, " ");

  // Match common two-letter forms like (FL), , FL, " - FL "
  for (const [abbr, name] of Object.entries(US_STATE_ABBR_TO_NAME)) {
    const abbrPattern = new RegExp(`(?:^|[^a-z])\\(?${abbr.toLowerCase()}\\)?(?:[^a-z]|$)`, "i");
    if (abbrPattern.test(haystack)) {
      const coords = US_STATE_CENTROIDS[name];
      if (coords) return { lat: coords[0], lng: coords[1] };
    }
  }

  // Match short textual forms like "Fla", "Calif", etc.
  for (const [token, name] of Object.entries(US_STATE_ALT_TOKENS)) {
    const altPattern = new RegExp(`\\b${escapeRegex(token).replace(/\s+/g, "\\s+")}\\b`, "i");
    if (altPattern.test(haystack)) {
      const coords = US_STATE_CENTROIDS[name];
      if (coords) return { lat: coords[0], lng: coords[1] };
    }
  }

  const entries = Object.entries(US_STATE_CENTROIDS).sort((a, b) => b[0].length - a[0].length);
  for (const [name, [lat, lng]] of entries) {
    const pattern = new RegExp(`\\b${escapeRegex(name).replace(/\s+/g, "\\s+")}\\b`, "i");
    if (pattern.test(haystack)) {
      return { lat, lng };
    }
  }
  return null;
}

function inferCityCoords(text) {
  const haystack = ` ${String(text ?? "").toLowerCase().replace(/\./g, " ")} `;
  const entries = Object.entries(CITY_CENTROIDS).sort((a, b) => b[0].length - a[0].length);
  for (const [name, [lat, lng]] of entries) {
    const pattern = new RegExp(`\\b${escapeRegex(name).replace(/\s+/g, "\\s+")}\\b`, "i");
    if (pattern.test(haystack)) return { lat, lng };
  }
  return null;
}

function inferCountryCoords(text) {
  const haystack = ` ${String(text ?? "").toLowerCase()} `;
  const entries = Object.entries(COUNTRY_CENTROIDS).sort((a, b) => b[0].length - a[0].length);
  for (const [name, [lat, lng]] of entries) {
    const pattern = new RegExp(`\\b${escapeRegex(name).replace(/\s+/g, "\\s+")}\\b`, "i");
    if (pattern.test(haystack)) return { lat, lng };
  }
  return null;
}

function inferCountryFromIsoCodes(values) {
  const list = Array.isArray(values) ? values : [values];
  for (const value of list) {
    const code = String(value ?? "").trim().toUpperCase();
    const name = ISO2_COUNTRY_HINTS[code];
    if (!name) continue;
    const coords = COUNTRY_CENTROIDS[name];
    if (coords) return { lat: coords[0], lng: coords[1] };
  }
  return null;
}

function inferCountryHintFromIsoCodes(values) {
  const list = Array.isArray(values) ? values : [values];
  for (const value of list) {
    const code = String(value ?? "").trim().toUpperCase();
    const name = ISO2_COUNTRY_HINTS[code];
    if (!name) continue;
    return { code, name };
  }
  return null;
}

function toDisplayCountryName(countryName) {
  return String(countryName ?? "")
    .split(/\s+/)
    .filter(Boolean)
    .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
    .join(" ");
}

function inferRegionFromCoords(lat, lng) {
  if (lat >= 7 && lat <= 83 && lng >= -168 && lng <= -52) return "North America";
  if (lat >= -56 && lat <= 13 && lng >= -82 && lng <= -35) return "South America";
  if (lat >= 35 && lat <= 72 && lng >= -11 && lng <= 40) return "Europe";
  if (lat >= -35 && lat <= 37 && lng >= -17 && lng <= 51) return "Africa";
  if (lat >= 5 && lat <= 77 && lng >= 40 && lng <= 180) return "Asia";
  if (lat >= -50 && lat <= 10 && lng >= 110 && lng <= 180) return "Oceania";
  if (lat < -60) return "Antarctica";
  return "International";
}

function extractUrlLocationText(urlValue) {
  try {
    const url = new URL(String(urlValue));
    const decodedPath = decodeURIComponent(url.pathname);
    const query = decodeURIComponent(url.search.replace(/^\?/, ""));
    return `${url.hostname} ${decodedPath} ${query}`
      .replace(/[._/+?=&%-]+/g, " ")
      .replace(/\s+/g, " ")
      .trim();
  } catch {
    return String(urlValue ?? "");
  }
}

function resolveCoords(meta, text, seed) {
  const inferredUS =
    meta.source.country_code === "US" ? inferUSStateCoords(text) : null;
  if (inferredUS) {
    return jitterCoords(inferredUS.lat, inferredUS.lng, seed, 10, 35);
  }
  const inferredCity = inferCityCoords(text);
  if (inferredCity) {
    return jitterCoords(inferredCity.lat, inferredCity.lng, seed, 5, 24);
  }
  const inferredCountry = inferCountryCoords(text);
  if (inferredCountry) {
    return jitterCoords(inferredCountry.lat, inferredCountry.lng, seed, 12, 52);
  }
  return jitterCoords(meta.lat, meta.lng, seed);
}

function resolveInterpolNoticeCoords(meta, notice, title, seed) {
  const textHints = [
    title,
    notice?.place_of_birth,
    notice?.issuing_entity,
    notice?.forename,
    notice?.name,
    ...(Array.isArray(notice?.nationalities) ? notice.nationalities : []),
    ...(Array.isArray(notice?.countries_likely_to_be_visited)
      ? notice.countries_likely_to_be_visited
      : []),
  ]
    .filter(Boolean)
    .join(" ");

  const isoCoords =
    inferCountryFromIsoCodes(notice?.countries_likely_to_be_visited) ||
    inferCountryFromIsoCodes(notice?.nationalities);
  if (isoCoords) {
    return jitterCoords(isoCoords.lat, isoCoords.lng, seed, 10, 45);
  }
  return resolveCoords(meta, textHints, seed);
}

function kevItemToAlert(entry, meta) {
  const cve = entry.cveID ?? entry.cveId ?? entry.cve;
  const title = `${cve ?? "CVE"}: ${entry.vulnerabilityName ?? "Known Exploited Vulnerability"}`;
  const nvdLink = cve ? `https://nvd.nist.gov/vuln/detail/${cve}` : meta.source.base_url;
  const now = new Date();
  const publishedAt = parseDate(entry.dateAdded);
  if (!publishedAt || !isFresh(publishedAt, now)) {
    return null;
  }
  const hours = Math.max(1, Math.round((now - publishedAt) / 36e5));
  const kevSeverity = hours <= 72 ? "critical" : hours <= 168 ? "high" : "medium";
  const jitter = resolveCoords(
    meta,
    `${title} ${nvdLink} ${extractUrlLocationText(nvdLink)}`,
    `${meta.source.source_id}:${nvdLink}:${cve ?? ""}`
  );
  const alert = {
    alert_id: `${meta.source.source_id}-${hashId(nvdLink)}`,
    source_id: meta.source.source_id,
    source: meta.source,
    title,
    canonical_url: nvdLink,
    first_seen: publishedAt.toISOString(),
    last_seen: now.toISOString(),
    status: "active",
    category: meta.category,
    severity: kevSeverity,
    region_tag: meta.region_tag,
    lat: jitter.lat,
    lng: jitter.lng,
    freshness_hours: hours,
    reporting: meta.reporting,
  };
  return {
    ...alert,
    triage: scoreIncidentRelevance(alert, {
      summary: `${entry.vulnerabilityName ?? ""} ${entry.shortDescription ?? ""}`.trim(),
      tags: [entry.knownRansomwareCampaign ? "known-ransomware-campaign" : ""].filter(Boolean),
      metaHints: { feedType: meta.type },
    }),
  };
}

// ─── AUTO-TRANSLATION ─────────────────────────────────────────
// Detect non-Latin text and translate to English via free Google Translate API.
const NON_LATIN_RE = /[\u3000-\u9FFF\uAC00-\uD7AF\u0400-\u04FF\u0600-\u06FF\u0E00-\u0E7F\u1100-\u11FF\uA960-\uA97F\uD7B0-\uD7FF]/;

async function translateToEnglish(text) {
  if (!text || !NON_LATIN_RE.test(text)) return text;
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 5000);
    const url = `https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=en&dt=t&q=${encodeURIComponent(text)}`;
    const res = await fetch(url, { signal: controller.signal });
    clearTimeout(timer);
    if (!res.ok) return text;
    const data = await res.json();
    const translated = data?.[0]?.map((s) => s?.[0] ?? "").join("") ?? text;
    return translated || text;
  } catch {
    return text;
  }
}

async function translateBatch(items) {
  const results = [];
  for (const item of items) {
    if (NON_LATIN_RE.test(item.title)) {
      item.title = await translateToEnglish(item.title);
    }
    if (item.summary && NON_LATIN_RE.test(item.summary)) {
      item.summary = await translateToEnglish(item.summary);
    }
    results.push(item);
  }
  return results;
}

function stripHtmlTags(value) {
  return String(value ?? "")
    .replace(/<script[\s\S]*?<\/script>/gi, " ")
    .replace(/<style[\s\S]*?<\/style>/gi, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/&nbsp;/gi, " ")
    .replace(/&amp;/gi, "&")
    .replace(/&quot;/gi, "\"")
    .replace(/&#39;|&apos;/gi, "'")
    .replace(/&lt;/gi, "<")
    .replace(/&gt;/gi, ">")
    .replace(/\s+/g, " ")
    .trim();
}

function parseHtmlAnchors(html, baseUrl) {
  const anchors = [];
  const seen = new Set();
  const anchorRe = /<a\b[^>]*href=(["'])(.*?)\1[^>]*>([\s\S]*?)<\/a>/gi;
  let match;
  while ((match = anchorRe.exec(html)) !== null) {
    const rawHref = String(match[2] ?? "").trim();
    if (!rawHref || rawHref.startsWith("#")) continue;
    const title = stripHtmlTags(match[3] ?? "");
    if (!title || title.length < 8) continue;
    let link;
    try {
      link = new URL(rawHref, baseUrl).toString();
    } catch {
      continue;
    }
    if (seen.has(link)) continue;
    seen.add(link);
    anchors.push({ title, link, summary: "" });
  }
  return anchors;
}

function normalizeExternalSource(entry) {
  if (!entry || typeof entry !== "object") return null;
  const source = entry.source;
  if (!source || typeof source !== "object") return null;
  if (!source.source_id || !source.authority_name || !entry.type || !entry.category) {
    return null;
  }
  const normalized = {
    ...entry,
    source: {
      ...source,
      source_id: String(source.source_id),
      authority_name: String(source.authority_name),
      country: String(source.country ?? "Unknown"),
      country_code: String(source.country_code ?? "XX"),
      region: String(source.region ?? "International"),
      authority_type: String(source.authority_type ?? "public_safety_program"),
      base_url: String(source.base_url ?? entry.feed_url ?? ""),
    },
  };
  return normalized;
}

async function loadExternalSources() {
  if (externalSourcesCache) return externalSourcesCache;
  try {
    const raw = await readFile(SOURCE_REGISTRY_PATH, "utf8");
    const parsed = JSON.parse(raw);
    const list = Array.isArray(parsed) ? parsed : [];
    const normalized = list
      .map(normalizeExternalSource)
      .filter(Boolean);
    externalSourcesCache = normalized;
    return normalized;
  } catch (error) {
    console.warn(`WARN source registry: ${error.message}`);
    externalSourcesCache = [];
    return [];
  }
}

async function getAllSources() {
  const extra = await loadExternalSources();
  if (extra.length === 0) return sources;
  const seen = new Set();
  const merged = [];
  for (const entry of [...sources, ...extra]) {
    const id = String(entry?.source?.source_id ?? "");
    if (!id || seen.has(id)) continue;
    seen.add(id);
    merged.push(entry);
  }
  return merged;
}

async function fetchFeed(url, followRedirects = false) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);
  try {
    const response = await fetch(url, {
      redirect: followRedirects ? "follow" : "manual",
      signal: controller.signal,
      headers: {
        "User-Agent": "Mozilla/5.0 (compatible; euosint-bot/1.0)",
        Accept: "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8",
      },
    });
    if (!response.ok) {
      throw new Error(`feed fetch failed ${response.status} ${url}`);
    }
    return response.text();
  } finally {
    clearTimeout(timer);
  }
}

async function fetchFeedWithFallback(urls, followRedirects = false) {
  const candidates = Array.isArray(urls) ? urls.filter(Boolean) : [urls].filter(Boolean);
  let lastError = null;
  for (const url of candidates) {
    try {
      const xml = await fetchFeed(url, followRedirects);
      return { xml, feedUrl: url };
    } catch (error) {
      lastError = error;
    }
  }
  throw lastError ?? new Error("no feed URLs available");
}

async function fetchRss(meta, now) {
  const limit = Math.max(1, Number(meta?.max_items ?? MAX_PER_SOURCE));
  const { xml } = await fetchFeedWithFallback(
    meta.feed_urls ?? [meta.feed_url],
    meta.followRedirects
  );
  let items = parseItems(xml)
    .filter((item) => item.title && item.link)
    .slice(0, limit);

  // Auto-translate non-English titles
  items = await translateBatch(items);

  return items.map((item) => {
    const publishedAt = parseDate(item.published) ?? now;
    if (!isFresh(publishedAt, now)) {
      return null;
    }
    const hours = Math.max(1, Math.round((now - publishedAt) / 36e5));
    const jitter = resolveCoords(
      meta,
      `${item.title} ${item.link} ${extractUrlLocationText(item.link)}`,
      `${meta.source.source_id}:${item.link}`
    );
    const alert = {
      alert_id: `${meta.source.source_id}-${hashId(item.link)}`,
      source_id: meta.source.source_id,
      source: meta.source,
      title: item.title,
      canonical_url: item.link,
      first_seen: publishedAt.toISOString(),
      last_seen: now.toISOString(),
      status: "active",
      category: meta.category,
      severity: inferSeverity(item.title, defaultSeverity(meta.category)),
      region_tag: meta.region_tag,
      lat: jitter.lat,
      lng: jitter.lng,
      freshness_hours: hours,
      reporting: meta.reporting,
    };
    const scored = {
      ...alert,
      triage: scoreIncidentRelevance(alert, {
        summary: item.summary,
        author: item.author,
        tags: item.tags,
        metaHints: { feedType: meta.type },
      }),
    };
    return normalizeInformationalSecurityAlert(scored, {
      summary: item.summary,
      author: item.author,
      tags: item.tags,
      metaHints: { feedType: meta.type },
    });
  }).filter(Boolean);
}

async function fetchHtmlList(meta, now) {
  const limit = Math.max(1, Number(meta?.max_items ?? MAX_PER_SOURCE));
  const { xml: html, feedUrl } = await fetchFeedWithFallback(
    meta.feed_urls ?? [meta.feed_url],
    meta.followRedirects ?? true
  );
  let items = parseHtmlAnchors(html, feedUrl);
  const includeKeywords = Array.isArray(meta?.include_keywords)
    ? meta.include_keywords.map((value) => String(value).toLowerCase()).filter(Boolean)
    : [];
  const excludeKeywords = Array.isArray(meta?.exclude_keywords)
    ? meta.exclude_keywords.map((value) => String(value).toLowerCase()).filter(Boolean)
    : [];
  if (includeKeywords.length > 0) {
    items = items.filter((item) => {
      const hay = `${item.title} ${item.link}`.toLowerCase();
      return includeKeywords.some((keyword) => hay.includes(keyword));
    });
  }
  if (excludeKeywords.length > 0) {
    items = items.filter((item) => {
      const hay = `${item.title} ${item.link}`.toLowerCase();
      return !excludeKeywords.some((keyword) => hay.includes(keyword));
    });
  }
  items = items.slice(0, limit);

  return items
    .map((item) => {
      const publishedAt = now;
      const hours = Math.max(1, Math.round((now - publishedAt) / 36e5));
      const jitter = resolveCoords(
        meta,
        `${item.title} ${item.link} ${extractUrlLocationText(item.link)}`,
        `${meta.source.source_id}:${item.link}`
      );
      const alert = {
        alert_id: `${meta.source.source_id}-${hashId(item.link)}`,
        source_id: meta.source.source_id,
        source: meta.source,
        title: item.title,
        canonical_url: item.link,
        first_seen: publishedAt.toISOString(),
        last_seen: now.toISOString(),
        status: "active",
        category: meta.category,
        severity: inferSeverity(item.title, defaultSeverity(meta.category)),
        region_tag: meta.region_tag,
        lat: jitter.lat,
        lng: jitter.lng,
        freshness_hours: hours,
        reporting: meta.reporting,
      };
      const scored = {
        ...alert,
        triage: scoreIncidentRelevance(alert, {
          summary: item.summary,
          tags: [],
          metaHints: { feedType: meta.type },
        }),
      };
      return normalizeInformationalSecurityAlert(scored, {
        summary: item.summary,
        tags: [],
        metaHints: { feedType: meta.type },
      });
    })
    .filter(Boolean);
}

async function fetchKev(meta) {
  const limit = Math.max(1, Number(meta?.max_items ?? MAX_PER_SOURCE));
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);
  let response;
  try {
    response = await fetch(meta.feed_url, {
      signal: controller.signal,
      headers: {
        "User-Agent": "euosint-bot/1.0",
        Accept: "application/json",
      },
    });
  } finally {
    clearTimeout(timer);
  }
  if (!response.ok) {
    throw new Error(`kev fetch failed ${response.status} ${meta.feed_url}`);
  }
  const data = await response.json();
  const vulnerabilities = Array.isArray(data?.vulnerabilities) ? data.vulnerabilities : [];
  // Sort by dateAdded descending (newest first) then take top N
  vulnerabilities.sort((a, b) => new Date(b.dateAdded).getTime() - new Date(a.dateAdded).getTime());
  return vulnerabilities
    .slice(0, limit)
    .map((entry) => kevItemToAlert(entry, meta))
    .filter(Boolean);
}

function interpolNoticeMatchesCountryCode(notice, countryCode) {
  const normalizedCode = String(countryCode ?? "").trim().toUpperCase();
  if (!normalizedCode) return false;
  const values = [
    ...(Array.isArray(notice?.nationalities) ? notice.nationalities : []),
    ...(Array.isArray(notice?.countries_likely_to_be_visited)
      ? notice.countries_likely_to_be_visited
      : []),
  ];
  return values.some((value) => String(value ?? "").trim().toUpperCase() === normalizedCode);
}

async function fetchInterpolPages(startUrl, limit, headers) {
  const seenPageUrls = new Set();
  const notices = [];
  let nextPageUrl = startUrl;
  let pageCount = 0;
  const MAX_INTERPOL_PAGES = 200;

  while (
    nextPageUrl &&
    notices.length < limit &&
    pageCount < MAX_INTERPOL_PAGES &&
    !seenPageUrls.has(nextPageUrl)
  ) {
    seenPageUrls.add(nextPageUrl);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 15000);
    let response;
    try {
      response = await fetch(nextPageUrl, {
        signal: controller.signal,
        headers,
      });
    } finally {
      clearTimeout(timer);
    }
    if (!response.ok) {
      throw new Error(`interpol fetch failed ${response.status} ${nextPageUrl}`);
    }
    const data = await response.json();
    const pageNotices = Array.isArray(data?._embedded?.notices) ? data._embedded.notices : [];
    if (pageNotices.length === 0) break;
    notices.push(...pageNotices);
    pageCount += 1;
    const nextHref = String(data?._links?.next?.href ?? "").trim();
    nextPageUrl = nextHref
      ? new URL(nextHref, "https://ws-public.interpol.int").toString()
      : null;
  }

  return notices;
}

async function fetchInterpolNotices(meta, now) {
  const limit = Math.max(1, Number(meta?.max_items ?? MAX_PER_SOURCE));
  const headers = {
    "User-Agent": "euosint-bot/1.0",
    Accept: "application/json",
  };
  const primaryNotices = await fetchInterpolPages(meta.feed_url, limit, headers);
  let notices = primaryNotices;
  let fallbackUsed = false;

  // Some nationality-filtered INTERPOL queries can return empty despite matching notices.
  // Fallback: query the parent feed and client-filter by nationality code.
  const url = new URL(meta.feed_url);
  const nationalityCode = String(url.searchParams.get("nationality") ?? "")
    .trim()
    .toUpperCase();
  if (notices.length === 0 && nationalityCode) {
    url.searchParams.delete("nationality");
    const fallbackPoolLimit = Math.max(limit * 5, 1000);
    const fallbackNotices = await fetchInterpolPages(
      url.toString(),
      fallbackPoolLimit,
      headers
    );
    const filteredFallback = fallbackNotices.filter((notice) =>
      interpolNoticeMatchesCountryCode(notice, nationalityCode)
    );
    if (filteredFallback.length > 0) {
      notices = filteredFallback;
      fallbackUsed = true;
    }
  }

  if (fallbackUsed) {
    console.warn(
      `WARN ${meta.source.authority_name}: primary nationality query returned empty; used client-side filtered fallback`
    );
  }

  const noticeTitlePrefix =
    meta.type === "interpol-yellow-json"
      ? "INTERPOL Yellow Notice"
      : "INTERPOL Red Notice";

  return notices.slice(0, limit).map((notice) => {
    const forename = String(notice.forename ?? "").trim();
    const name = String(notice.name ?? "").trim();
    const label = [forename, name].filter(Boolean).join(" ");
    const rawHref = String(notice?._links?.self?.href ?? "").trim();
    const canonicalUrl = rawHref
      ? new URL(rawHref, "https://ws-public.interpol.int").toString()
      : meta.source.base_url;
    const title = label ? `${noticeTitlePrefix}: ${label}` : noticeTitlePrefix;
    const jitter = resolveInterpolNoticeCoords(
      meta,
      notice,
      `${title} ${extractUrlLocationText(canonicalUrl)}`,
      `${meta.source.source_id}:${canonicalUrl}`
    );
    const countryHint =
      inferCountryHintFromIsoCodes(notice?.countries_likely_to_be_visited) ||
      inferCountryHintFromIsoCodes(notice?.nationalities);
    const derivedRegion = inferRegionFromCoords(jitter.lat, jitter.lng);
    const derivedSource = {
      ...meta.source,
      country: countryHint ? toDisplayCountryName(countryHint.name) : meta.source.country,
      country_code: countryHint?.code ?? meta.source.country_code,
      region: derivedRegion || meta.source.region,
    };
    const alert = {
      alert_id: `${meta.source.source_id}-${hashId(canonicalUrl + title)}`,
      source_id: meta.source.source_id,
      source: derivedSource,
      title,
      canonical_url: canonicalUrl,
      first_seen: now.toISOString(),
      last_seen: now.toISOString(),
      status: "active",
      category: meta.category,
      severity: "critical",
      region_tag: countryHint?.code ?? meta.region_tag,
      lat: jitter.lat,
      lng: jitter.lng,
      freshness_hours: 1,
      reporting: meta.reporting,
    };
    return {
      ...alert,
      triage: scoreIncidentRelevance(alert, {
        summary: `${notice?.issuing_entity ?? ""} ${notice?.place_of_birth ?? ""}`.trim(),
        tags: [
          ...(Array.isArray(notice?.nationalities) ? notice.nationalities : []),
          ...(Array.isArray(notice?.countries_likely_to_be_visited)
            ? notice.countries_likely_to_be_visited
            : []),
        ],
        metaHints: { feedType: meta.type },
      }),
    };
  });
}

function createStaticInterpolEntry(now) {
  return {
    alert_id: "interpol-hub-static",
    source_id: "interpol-hub",
    source: {
      source_id: "interpol-hub",
      authority_name: "INTERPOL Notices Hub",
      country: "France",
      country_code: "FR",
      region: "International",
      authority_type: "police",
      base_url: "https://www.interpol.int",
    },
    title: "INTERPOL Red & Yellow Notices — Browse Wanted & Missing Persons",
    canonical_url: "https://www.interpol.int/How-we-work/Notices/View-Red-Notices",
    first_seen: now.toISOString(),
    last_seen: now.toISOString(),
    status: "active",
    category: "wanted_suspect",
    severity: "critical",
    region_tag: "INT",
    lat: 45.764,
    lng: 4.8357,
    freshness_hours: 1,
    reporting: {
      label: "Browse INTERPOL Notices",
      url: "https://www.interpol.int/How-we-work/Notices/View-Red-Notices",
      notes:
        "Red Notices: wanted persons. Yellow Notices: missing persons. " +
        "Browse directly — https://www.interpol.int/How-we-work/Notices/View-Yellow-Notices",
    },
    triage: { relevance_score: 1, reasoning: "Permanent INTERPOL hub link" },
  };
}

async function buildAlerts() {
  const now = new Date();
  const alerts = [createStaticInterpolEntry(now)];
  const sourceHealth = [];
  const sourceEntries = await getAllSources();

  for (const entry of sourceEntries) {
    const sourceId = entry.source.source_id;
    const startedAt = new Date().toISOString();
    try {
      const batch =
        entry.type === "kev-json"
          ? await fetchKev(entry)
          : entry.type === "interpol-red-json" || entry.type === "interpol-yellow-json"
          ? await fetchInterpolNotices(entry, now)
          : entry.type === "html-list"
          ? await fetchHtmlList(entry, now)
          : await fetchRss(entry, now);
      alerts.push(...batch);
      sourceHealth.push({
        source_id: sourceId,
        authority_name: entry.source.authority_name,
        type: entry.type,
        status: "ok",
        fetched_count: batch.length,
        feed_url: entry.feed_url,
        started_at: startedAt,
        finished_at: new Date().toISOString(),
      });
    } catch (error) {
      console.warn(`WARN ${entry.source.authority_name}: ${error.message}`);
      sourceHealth.push({
        source_id: sourceId,
        authority_name: entry.source.authority_name,
        type: entry.type,
        status: "error",
        fetched_count: 0,
        feed_url: entry.feed_url,
        error: error.message,
        started_at: startedAt,
        finished_at: new Date().toISOString(),
      });
    }
  }

  const dedupedByKey = new Map();
  for (const alert of alerts) {
    const key = `${alert.canonical_url}|${alert.title}`.toLowerCase();
    const current = dedupedByKey.get(key);
    const currentScore = Number(current?.triage?.relevance_score ?? -1);
    const nextScore = Number(alert?.triage?.relevance_score ?? -1);
    if (!current || nextScore > currentScore) {
      dedupedByKey.set(key, alert);
    }
  }
  const deduped = [...dedupedByKey.values()];
  const {
    kept: variantCollapsed,
    suppressed: suppressedVariants,
  } = collapseRecurringHeadlineVariants(deduped);
  const threshold = clamp01(INCIDENT_RELEVANCE_THRESHOLD);
  const active = variantCollapsed.filter(
    (alert) =>
      Number(alert?.triage?.relevance_score ?? 0) >=
      thresholdForAlert(alert, threshold)
  );
  const filtered = variantCollapsed.filter(
    (alert) =>
      Number(alert?.triage?.relevance_score ?? 0) <
      thresholdForAlert(alert, threshold)
  );
  active.sort((a, b) => new Date(b.first_seen).getTime() - new Date(a.first_seen).getTime());
  filtered.sort((a, b) => {
    const scoreDelta =
      Number(b?.triage?.relevance_score ?? 0) - Number(a?.triage?.relevance_score ?? 0);
    if (scoreDelta !== 0) return scoreDelta;
    return new Date(b.first_seen).getTime() - new Date(a.first_seen).getTime();
  });
  const activeBySource = active.reduce((acc, alert) => {
    acc[alert.source_id] = (acc[alert.source_id] ?? 0) + 1;
    return acc;
  }, {});
  const filteredBySource = filtered.reduce((acc, alert) => {
    acc[alert.source_id] = (acc[alert.source_id] ?? 0) + 1;
    return acc;
  }, {});
  sourceHealth.forEach((entry) => {
    entry.active_count = activeBySource[entry.source_id] ?? 0;
    entry.filtered_count = filteredBySource[entry.source_id] ?? 0;
  });
  const duplicateHeadlineSamples = summarizeTitleDuplicates(active);
  const duplicateAudit = {
    suppressed_variant_duplicates: suppressedVariants.length,
    repeated_title_groups_in_active: duplicateHeadlineSamples.length,
    repeated_title_samples: duplicateHeadlineSamples,
  };
  if (suppressedVariants.length > 0) {
    console.log(
      `Suppressed ${suppressedVariants.length} recurring headline variants`
    );
  }
  return { active, filtered, sourceHealth, duplicateAudit };
}

async function readAlertsFile(path) {
  try {
    const raw = await readFile(path, "utf8");
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function reconcileAlerts(activeAlerts, filteredAlerts, previousState, now) {
  const nowIso = now.toISOString();
  const nowMs = now.getTime();
  const retentionCutoff = nowMs - REMOVED_RETENTION_DAYS * 86400000;
  const previousById = new Map(previousState.map((a) => [a.alert_id, a]));
  const presentById = new Map(
    [...activeAlerts, ...filteredAlerts].map((a) => [a.alert_id, a])
  );

  const currentActive = activeAlerts.map((a) => {
    const prev = previousById.get(a.alert_id);
    return {
      ...a,
      status: "active",
      first_seen: prev?.first_seen ?? a.first_seen,
      last_seen: nowIso,
    };
  });

  const currentFiltered = filteredAlerts.map((a) => {
    const prev = previousById.get(a.alert_id);
    return {
      ...a,
      status: "filtered",
      first_seen: prev?.first_seen ?? a.first_seen,
      last_seen: nowIso,
    };
  });

  const removedNew = previousState
    .filter((prev) => prev.status !== "removed" && prev.status !== "filtered")
    .filter((prev) => !presentById.has(prev.alert_id))
    .map((prev) => ({
      ...prev,
      status: "removed",
      last_seen: nowIso,
    }));

  const removedCarry = previousState
    .filter((prev) => prev.status === "removed")
    .filter((prev) => !presentById.has(prev.alert_id))
    .filter((prev) => {
      const t = new Date(prev.last_seen).getTime();
      return Number.isFinite(t) && t >= retentionCutoff;
    });

  const state = [...currentActive, ...currentFiltered, ...removedNew, ...removedCarry].sort(
    (a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()
  );

  return { currentActive, currentFiltered, state };
}

function assertCriticalSourceCoverage(sourceHealth) {
  if (!FAIL_ON_CRITICAL_SOURCE_GAP || CRITICAL_SOURCE_PREFIXES.length === 0) return;
  const missingPrefixes = CRITICAL_SOURCE_PREFIXES.filter((prefix) => {
    const matched = sourceHealth.filter(
      (entry) =>
        entry.source_id === prefix || entry.source_id.startsWith(`${prefix}-`)
    );
    const totalFetched = matched.reduce(
      (sum, entry) => sum + Number(entry.fetched_count ?? 0),
      0
    );
    return totalFetched === 0;
  });
  if (missingPrefixes.length > 0) {
    throw new Error(
      `critical source coverage gap: no records fetched for ${missingPrefixes.join(", ")}`
    );
  }
}

async function writeAlerts(
  activeAlerts,
  filteredAlerts,
  stateAlerts,
  sourceHealth,
  duplicateAudit
) {
  await mkdir(dirname(OUTPUT_PATH), { recursive: true });
  await mkdir(dirname(STATE_OUTPUT_PATH), { recursive: true });
  await mkdir(dirname(FILTERED_OUTPUT_PATH), { recursive: true });
  await mkdir(dirname(SOURCE_HEALTH_OUTPUT_PATH), { recursive: true });
  await writeFile(OUTPUT_PATH, JSON.stringify(activeAlerts, null, 2) + "\n", "utf8");
  await writeFile(
    FILTERED_OUTPUT_PATH,
    JSON.stringify(filteredAlerts, null, 2) + "\n",
    "utf8"
  );
  await writeFile(STATE_OUTPUT_PATH, JSON.stringify(stateAlerts, null, 2) + "\n", "utf8");
  const healthDoc = {
    generated_at: new Date().toISOString(),
    critical_source_prefixes: CRITICAL_SOURCE_PREFIXES,
    fail_on_critical_source_gap: FAIL_ON_CRITICAL_SOURCE_GAP,
    total_sources: sourceHealth.length,
    sources_ok: sourceHealth.filter((entry) => entry.status === "ok").length,
    sources_error: sourceHealth.filter((entry) => entry.status === "error").length,
    duplicate_audit: duplicateAudit,
    sources: sourceHealth,
  };
  await writeFile(
    SOURCE_HEALTH_OUTPUT_PATH,
    JSON.stringify(healthDoc, null, 2) + "\n",
    "utf8"
  );
  const removedCount = stateAlerts.filter((a) => a.status === "removed").length;
  const filteredCount = filteredAlerts.length;
  console.log(
    `Wrote ${activeAlerts.length} active alerts -> ${OUTPUT_PATH} (${filteredCount} filtered in ${FILTERED_OUTPUT_PATH}, ${removedCount} removed tracked in ${STATE_OUTPUT_PATH}, source health in ${SOURCE_HEALTH_OUTPUT_PATH})`
  );
}

async function main() {
  const { active, filtered, sourceHealth, duplicateAudit } = await buildAlerts();
  assertCriticalSourceCoverage(sourceHealth);
  const previous =
    (await readAlertsFile(STATE_OUTPUT_PATH)).length > 0
      ? await readAlertsFile(STATE_OUTPUT_PATH)
      : await readAlertsFile(OUTPUT_PATH);
  const { currentActive, currentFiltered, state } = reconcileAlerts(
    active,
    filtered,
    previous,
    new Date()
  );
  await writeAlerts(
    currentActive,
    currentFiltered,
    state,
    sourceHealth,
    duplicateAudit
  );

  if (WATCH) {
    console.log(`Watching feeds every ${Math.round(INTERVAL_MS / 1000)}s...`);
    setInterval(async () => {
      try {
        const {
          active: nextActive,
          filtered: nextFiltered,
          sourceHealth: nextSourceHealth,
          duplicateAudit: nextDuplicateAudit,
        } = await buildAlerts();
        assertCriticalSourceCoverage(nextSourceHealth);
        const prevState =
          (await readAlertsFile(STATE_OUTPUT_PATH)).length > 0
            ? await readAlertsFile(STATE_OUTPUT_PATH)
            : await readAlertsFile(OUTPUT_PATH);
        const {
          currentActive: activeNow,
          currentFiltered: filteredNow,
          state: stateNow,
        } = reconcileAlerts(
          nextActive,
          nextFiltered,
          prevState,
          new Date()
        );
        await writeAlerts(
          activeNow,
          filteredNow,
          stateNow,
          nextSourceHealth,
          nextDuplicateAudit
        );
      } catch (error) {
        console.warn(`WARN refresh: ${error.message}`);
      }
    }, INTERVAL_MS);
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
