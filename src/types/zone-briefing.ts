export interface ZoneBriefingHotspot {
  label: string;
  lat: number;
  lng: number;
  eventCount: number;
}

export interface ZoneBriefingMetrics {
  events7d: number;
  events30d: number;
  fatalitiesBest7d: number;
  fatalitiesBest30d: number;
  civilianDeaths30d: number;
  trend7d: string;
  trend30d: string;
}

export interface ZoneBriefingSummary {
  headline?: string;
  bullets?: string[];
  watchItems?: string[];
}

export interface ZoneBriefingEvent {
  title: string;
  published?: string;
  country?: string;
  countryCode?: string;
  fatalities?: number;
  civilianDeaths?: number;
  lat?: number;
  lng?: number;
  link?: string;
}

export interface ZoneBriefingRecord {
  lensId: string;
  source: string;
  sourceUrl?: string;
  title?: string;
  status?: string;
  updatedAt?: string;
  coverageNote?: string;
  countryIds?: string[];
  countryLabels?: string[];
  actors?: string[];
  violenceTypes?: string[];
  hotspots?: ZoneBriefingHotspot[];
  metrics?: ZoneBriefingMetrics;
  summary?: ZoneBriefingSummary;
  recentEvents?: ZoneBriefingEvent[];
}
