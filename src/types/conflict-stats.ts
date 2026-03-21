export interface ConflictCountryStat {
  gwno: string;
  iso2?: string;
  label?: string;
  fatalitiesTotal: number;
  fatalitiesLatest: number;
  latestYear: number;
}

export interface ConflictStatRecord {
  conflictId: string;
  title: string;
  year: number;
  startDate?: string;
  intensityLevel: number;
  typeOfConflict?: string;
  region?: string;
  sideA?: string;
  sideB?: string;
  lensIds: string[];
  overlayCountryCodes: string[];
  sourceUrl?: string;
  fatalitiesTotal: number;
  fatalitiesLatestYear: number;
  fatalitiesLatestYearYear: number;
  countries: ConflictCountryStat[];
}
