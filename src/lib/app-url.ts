function basePathOnly(rawBase: string): string {
  const fallback = "/";
  try {
    const base = String(rawBase || fallback).trim();
    // Always resolve against current origin and keep only pathname to avoid
    // accidental cross-origin hosts (for example localhost in production builds).
    const parsed = new URL(base, window.location.origin);
    const path = parsed.pathname || fallback;
    return path.endsWith("/") ? path : `${path}/`;
  } catch {
    return fallback;
  }
}

export function appURL(path: string): string {
  const clean = path.replace(/^\/+/, "");
  const base = basePathOnly(import.meta.env.BASE_URL);
  return `${base}${clean}`;
}
