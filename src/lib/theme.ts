/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

/**
 * Runtime reader for CSS custom-property colours defined in index.css @theme.
 * This is the only place JS code should obtain colour hex values — everything
 * else references Tailwind theme classes or CSS variables directly.
 */

import type { Severity } from "@/types/alert";

const SEVERITY_VARS: Record<Severity, string> = {
  critical: "--color-siem-critical",
  high: "--color-siem-high",
  medium: "--color-siem-medium",
  low: "--color-siem-low",
  info: "--color-siem-info",
};

let root: CSSStyleDeclaration | null = null;

function getRoot(): CSSStyleDeclaration {
  if (!root) {
    root = getComputedStyle(document.documentElement);
  }
  return root;
}

/** Read any `--color-*` CSS variable as a trimmed hex string. */
export function cssColor(varName: string): string {
  return getRoot().getPropertyValue(varName).trim();
}

/** Get the hex colour for a severity level, read from the CSS theme. */
export function severityHex(severity: Severity): string {
  return cssColor(SEVERITY_VARS[severity]);
}

/** Get the hex colour for the brand accent, read from the CSS theme. */
export function accentHex(): string {
  return cssColor("--color-siem-accent");
}

/** Get the hex colour for the primary text, read from the CSS theme. */
export function textHex(): string {
  return cssColor("--color-siem-text");
}
