import { describe, it, expect } from "vitest";
import { latLngToRegion, alertMatchesRegionFilter, MIDDLE_EAST_CODES } from "./regions";
import type { Alert } from "@/types/alert";

describe("latLngToRegion", () => {
  it("maps Berlin to Europe", () => {
    expect(latLngToRegion(52.52, 13.405)).toBe("Europe");
  });

  it("maps Paris to Europe", () => {
    expect(latLngToRegion(48.86, 2.35)).toBe("Europe");
  });

  it("maps Washington DC to North America", () => {
    expect(latLngToRegion(38.9, -77.04)).toBe("North America");
  });

  it("maps Lagos to Africa", () => {
    expect(latLngToRegion(6.45, 3.4)).toBe("Africa");
  });

  it("maps Riyadh to Middle East", () => {
    expect(latLngToRegion(24.7, 46.7)).toBe("Middle East");
  });

  it("maps Tehran to Middle East", () => {
    expect(latLngToRegion(35.7, 51.4)).toBe("Middle East");
  });

  it("maps Tokyo to Asia-Pacific", () => {
    expect(latLngToRegion(35.68, 139.69)).toBe("Asia-Pacific");
  });

  it("maps Sydney to Asia-Pacific", () => {
    expect(latLngToRegion(-33.87, 151.21)).toBe("Asia-Pacific");
  });

  it("maps Bogota to South America", () => {
    expect(latLngToRegion(4.71, -74.07)).toBe("South America");
  });

  it("maps Kingston Jamaica to Caribbean", () => {
    expect(latLngToRegion(18.0, -76.8)).toBe("Caribbean");
  });

  it("returns null for Antarctica", () => {
    expect(latLngToRegion(-80, 0)).toBeNull();
  });
});

describe("MIDDLE_EAST_CODES", () => {
  it("includes core Middle East countries", () => {
    for (const code of ["SA", "IR", "IQ", "IL", "AE", "SY", "YE", "JO"]) {
      expect(MIDDLE_EAST_CODES.has(code)).toBe(true);
    }
  });

  it("excludes non-ME countries", () => {
    for (const code of ["US", "DE", "CN", "AU", "BR"]) {
      expect(MIDDLE_EAST_CODES.has(code)).toBe(false);
    }
  });
});

const defaultSource: Alert["source"] = {
  source_id: "src",
  authority_name: "Test",
  country: "Test",
  country_code: "",
  region: "",
  authority_type: "police",
  base_url: "",
};

function makeAlert(overrides: Record<string, unknown> = {}): Alert {
  const { source: srcOverrides, ...rest } = overrides;
  return {
    alert_id: "test-1",
    source_id: "src",
    status: "active",
    title: "Test alert",
    canonical_url: "https://example.test",
    category: "informational",
    severity: "medium",
    first_seen: new Date().toISOString(),
    last_seen: new Date().toISOString(),
    region_tag: "",
    freshness_hours: 0,
    lat: 0,
    lng: 0,
    source: { ...defaultSource, ...(srcOverrides as Partial<Alert["source"]>) },
    ...rest,
  } as Alert;
}

describe("alertMatchesRegionFilter", () => {
  it("returns true for 'all' filter", () => {
    const alert = makeAlert({});
    expect(alertMatchesRegionFilter(alert, "all")).toBe(true);
  });

  it("matches country: prefix filter", () => {
    const alert = makeAlert({ source: { country_code: "DE" } });
    expect(alertMatchesRegionFilter(alert, "country:DE")).toBe(true);
    expect(alertMatchesRegionFilter(alert, "country:FR")).toBe(false);
  });

  it("matches Middle East by country code", () => {
    const alert = makeAlert({ source: { country_code: "SA" } });
    expect(alertMatchesRegionFilter(alert, "Middle East")).toBe(true);
  });

  it("matches Middle East by source region", () => {
    const alert = makeAlert({ source: { region: "Middle East" } });
    expect(alertMatchesRegionFilter(alert, "Middle East")).toBe(true);
  });

  it("matches Asia-Pacific for Oceania source", () => {
    const alert = makeAlert({ source: { region: "Oceania" } });
    expect(alertMatchesRegionFilter(alert, "Asia-Pacific")).toBe(true);
  });

  it("matches Asia-Pacific for Asia source", () => {
    const alert = makeAlert({ source: { region: "Asia" } });
    expect(alertMatchesRegionFilter(alert, "Asia-Pacific")).toBe(true);
  });

  it("matches Caribbean by lat/lng bounds", () => {
    const alert = makeAlert({ lat: 18, lng: -76 });
    expect(alertMatchesRegionFilter(alert, "Caribbean")).toBe(true);
  });

  it("matches simple region by source.region", () => {
    const alert = makeAlert({ source: { region: "Europe" } });
    expect(alertMatchesRegionFilter(alert, "Europe")).toBe(true);
    expect(alertMatchesRegionFilter(alert, "Africa")).toBe(false);
  });
});
