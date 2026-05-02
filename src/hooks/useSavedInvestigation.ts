import { useEffect, useMemo, useState } from "react";
import type { EntityRef } from "@/agentops/lib/entities";
import { entityKey } from "@/agentops/lib/entities";

export interface SavedInvestigation {
  pinnedEntities: string[];
  notes: string;
  openedAt: string;
}

function keyFor(id: string): string {
  return `kafsiem.investigation.${id}`;
}

function emptyInvestigation(): SavedInvestigation {
  return {
    pinnedEntities: [],
    notes: "",
    openedAt: new Date().toISOString(),
  };
}

function safeStorage(): Storage | null {
  if (typeof window === "undefined" || !("localStorage" in window)) return null;
  const storage = window.localStorage;
  if (!storage || typeof storage.getItem !== "function" || typeof storage.setItem !== "function") return null;
  return storage;
}

function loadInvestigation(id: string): SavedInvestigation {
  const storage = safeStorage();
  if (!storage) return emptyInvestigation();
  const raw = storage.getItem(keyFor(id));
  if (!raw) return emptyInvestigation();
  try {
    const parsed = JSON.parse(raw) as Partial<SavedInvestigation>;
    return {
      pinnedEntities: Array.isArray(parsed.pinnedEntities) ? parsed.pinnedEntities.filter((item): item is string => typeof item === "string") : [],
      notes: typeof parsed.notes === "string" ? parsed.notes : "",
      openedAt: typeof parsed.openedAt === "string" && parsed.openedAt ? parsed.openedAt : new Date().toISOString(),
    };
  } catch {
    return emptyInvestigation();
  }
}

export function useSavedInvestigation(id: string | null) {
  const storageKey = id || "scratch";
  const [investigation, setInvestigation] = useState<SavedInvestigation>(() => loadInvestigation(storageKey));

  useEffect(() => {
    setInvestigation(loadInvestigation(storageKey));
  }, [storageKey]);

  useEffect(() => {
    const storage = safeStorage();
    if (!storage) return;
    storage.setItem(keyFor(storageKey), JSON.stringify(investigation));
  }, [investigation, storageKey]);

  const pinnedSet = useMemo(() => new Set(investigation.pinnedEntities), [investigation.pinnedEntities]);

  function setNotes(notes: string) {
    setInvestigation((current) => ({ ...current, notes }));
  }

  function togglePinned(ref: EntityRef) {
    const key = entityKey(ref);
    setInvestigation((current) => {
      const pinned = new Set(current.pinnedEntities);
      if (pinned.has(key)) {
        pinned.delete(key);
      } else {
        pinned.add(key);
      }
      return { ...current, pinnedEntities: [...pinned].sort() };
    });
  }

  function isPinned(ref: EntityRef): boolean {
    return pinnedSet.has(entityKey(ref));
  }

  return { investigation, setNotes, togglePinned, isPinned };
}
