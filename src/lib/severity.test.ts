import { describe, it, expect } from "vitest";
import { severityLabel, categoryLabels, categoryOrder, freshnessLabel } from "./severity";

describe("severityLabel", () => {
  it("has all five severity levels", () => {
    expect(Object.keys(severityLabel)).toEqual(["critical", "high", "medium", "low", "info"]);
  });

  it("maps info to informational", () => {
    expect(severityLabel.info).toBe("informational");
  });
});

describe("categoryLabels", () => {
  it("covers all categories in categoryOrder", () => {
    for (const cat of categoryOrder) {
      expect(categoryLabels[cat]).toBeDefined();
      expect(typeof categoryLabels[cat]).toBe("string");
    }
  });

  it("maps cyber_advisory correctly", () => {
    expect(categoryLabels.cyber_advisory).toBe("Cyber Advisory");
  });
});

describe("freshnessLabel", () => {
  it("returns 'Just now' for < 1 hour", () => {
    expect(freshnessLabel(0.5)).toBe("Just now");
  });

  it("returns hours for < 24h", () => {
    expect(freshnessLabel(6)).toBe("6h ago");
  });

  it("returns days for < 7d", () => {
    expect(freshnessLabel(72)).toBe("3d ago");
  });

  it("returns weeks for >= 7d", () => {
    expect(freshnessLabel(336)).toBe("2w ago");
  });
});
