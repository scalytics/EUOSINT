export interface ZoneBriefingHotspot {
  label: string;
  lat: number;
  lng: number;
  eventCount: number;
}

export interface ZoneBriefingRecord {
  lensId: string;
  source: string;
  sourceUrl?: string;
  status?: string;
  updatedAt?: string;
  coverageNote?: string;
  countryIds?: string[];
  countryLabels?: string[];
  actors?: string[];
  violenceTypes?: string[];
  hotspots?: ZoneBriefingHotspot[];
}
