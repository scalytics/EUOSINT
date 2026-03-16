/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useCallback, useEffect, useRef, useState } from "react";
import type { Alert } from "@/types/alert";

const SEARCH_URL = `${import.meta.env.BASE_URL}api/search`;
const DEBOUNCE_MS = 300;

interface SearchResult {
  query: string;
  count: number;
  results: Alert[];
}

export function useSearch() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Alert[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [isApiAvailable, setIsApiAvailable] = useState<boolean | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Probe API availability once on mount.
  useEffect(() => {
    fetch(`${SEARCH_URL.replace("/search", "/health")}`, { cache: "no-store" })
      .then((r) => setIsApiAvailable(r.ok))
      .catch(() => setIsApiAvailable(false));
  }, []);

  const search = useCallback(
    async (q: string) => {
      abortRef.current?.abort();

      const trimmed = q.trim();
      if (!trimmed || !isApiAvailable) {
        setResults([]);
        setIsSearching(false);
        return;
      }

      const controller = new AbortController();
      abortRef.current = controller;
      setIsSearching(true);

      try {
        const url = `${SEARCH_URL}?q=${encodeURIComponent(trimmed)}&limit=200`;
        const response = await fetch(url, {
          signal: controller.signal,
          cache: "no-store",
        });
        if (!response.ok) throw new Error(`${response.status}`);
        const data = (await response.json()) as SearchResult;
        if (!controller.signal.aborted) {
          setResults(data.results ?? []);
        }
      } catch {
        if (!controller.signal.aborted) {
          setResults([]);
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsSearching(false);
        }
      }
    },
    [isApiAvailable],
  );

  // Debounced search trigger.
  useEffect(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    if (!query.trim()) {
      setResults([]);
      setIsSearching(false);
      return;
    }
    timerRef.current = setTimeout(() => search(query), DEBOUNCE_MS);
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [query, search]);

  return { query, setQuery, results, isSearching, isApiAvailable };
}
