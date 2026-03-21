export interface CurrentConflictRecord {
  conflictId: string;
  countryId?: string;
  title: string;
  year: number;
  startDate?: string;
  intensityLevel: number;
  typeOfConflict?: string;
  gwnoLoc?: string;
  sideA?: string;
  sideB?: string;
  region?: string;
  lensIds: string[];
  overlayCountryCodes?: string[];
  sourceUrl?: string;
}

export interface ConflictCountryFocus {
  code: string;
  label: string;
  lensId?: string;
}
