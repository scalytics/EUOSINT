/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useMemo, useState } from "react";
import type { Alert } from "@/types/alert";
const ALERTS_URL = `${import.meta.env.BASE_URL}alerts.json`;
const POLL_MS = 15000;

function normalizeAlerts(data: unknown): Alert[] | null {
  if (!Array.isArray(data)) {
    return null;
  }

  return data.filter((item) => item && typeof item === "object") as Alert[];
}

function alertsAreEqual(a: Alert[], b: Alert[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i += 1) {
    const left = a[i];
    const right = b[i];
    if (left.alert_id !== right.alert_id) return false;
    if (left.title !== right.title) return false;
    if (left.severity !== right.severity) return false;
    if (left.first_seen !== right.first_seen) return false;
    if (left.last_seen !== right.last_seen) return false;
    if (left.lat !== right.lat || left.lng !== right.lng) return false;
    if (left.source.region !== right.source.region) return false;
    if (left.canonical_url !== right.canonical_url) return false;
    if (left.triage?.relevance_score !== right.triage?.relevance_score) return false;
    if (left.triage?.disposition !== right.triage?.disposition) return false;
  }
  return true;
}

export function useAlerts() {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [isLive, setIsLive] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    let inFlight = false;

    async function load() {
      if (inFlight) return;
      inFlight = true;
      try {
        const response = await fetch(`${ALERTS_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (!response.ok) {
          throw new Error(`alerts fetch failed: ${response.status}`);
        }
        const data = (await response.json()) as unknown;
        const normalized = normalizeAlerts(data);
        if (!cancelled && normalized) {
          setAlerts((prev) => (alertsAreEqual(prev, normalized) ? prev : normalized));
          setIsLive(true);
          setIsLoading(false);
        }
      } catch {
        if (!cancelled) {
          setIsLive(false);
          setIsLoading(false);
        }
      } finally {
        inFlight = false;
      }
    }

    load();
    const interval = setInterval(load, POLL_MS);
    const onFocus = () => load();
    const onVisible = () => {
      if (document.visibilityState === "visible") load();
    };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      cancelled = true;
      clearInterval(interval);
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, []);

  const sourceCount = useMemo(() => new Set(alerts.map((a) => a.source_id)).size, [alerts]);

  return { alerts, isLive, isLoading, sourceCount };
}
