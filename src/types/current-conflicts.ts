export interface CurrentConflictRecord {
  conflictId: string;
  title: string;
  year: number;
  startDate?: string;
  intensityLevel: number;
  typeOfConflict?: string;
  gwnoLoc?: string;
  sideA?: string;
  sideB?: string;
  lensIds: string[];
  sourceUrl?: string;
}
