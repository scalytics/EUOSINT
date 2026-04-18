/*
 * kafSIEM
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useEffect, useState } from "react";
import type { SourceHealthDocument } from "@/types/source-health";

const SOURCE_HEALTH_URL = `${import.meta.env.BASE_URL}source-health.json`;
const POLL_MS = 30000;

export function useSourceHealth() {
  const [document, setDocument] = useState<SourceHealthDocument | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const response = await fetch(`${SOURCE_HEALTH_URL}?t=${Date.now()}`, {
          cache: "no-store",
        });
        if (!response.ok) {
          throw new Error(`source health fetch failed: ${response.status}`);
        }
        const data = (await response.json()) as SourceHealthDocument;
        if (!cancelled) {
          setDocument(data);
          setIsLoading(false);
        }
      } catch {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    load();
    const interval = window.setInterval(load, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, []);

  return { sourceHealth: document, isLoading };
}
