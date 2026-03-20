export interface ZoneBriefingHotspot {
  label: string;
  lat: number;
  lng: number;
  eventCount: number;
}

export interface ZoneBriefingConflict {
  conflictId: string;
  name: string;
  type: string;
  intensity: number;
}

export interface ZoneBriefingACLED {
  events7d: number;
  fatalities7d: number;
  topEvent?: string;
  asOf?: string;
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
  conflictIntensity?: string;
  conflictType?: string;
  activeConflicts?: ZoneBriefingConflict[];
  acledRecency?: ZoneBriefingACLED;
}
