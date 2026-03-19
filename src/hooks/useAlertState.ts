/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useState } from "react";
import type { Alert } from "@/types/alert";

const ALERT_STATE_URL = `${import.meta.env.BASE_URL}alerts-state.json`;
const POLL_MS = 45000;

function normalizeAlerts(data: unknown): Alert[] | null {
  if (!Array.isArray(data)) {
    return null;
  }

  return data.filter((item) => item && typeof item === "object") as Alert[];
}

export function useAlertState() {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    let inFlight = false;

    async function load() {
      if (inFlight) return;
      inFlight = true;
      try {
        const response = await fetch(`${ALERT_STATE_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (!response.ok) {
          throw new Error(`alerts-state fetch failed: ${response.status}`);
        }
        const data = (await response.json()) as unknown;
        const normalized = normalizeAlerts(data);
        if (!cancelled && normalized) {
          setAlerts(normalized);
          setIsLoading(false);
        }
      } catch {
        if (!cancelled) {
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

  return { alerts, isLoading };
}
