import type { EntityRef } from "@/agentops/lib/entities";

export function readEntityRoute(): EntityRef | null {
  if (typeof window === "undefined") return null;
  const params = new URLSearchParams(window.location.search);
  if (params.get("view") !== "entity") return null;
  const type = (params.get("type") || "").trim();
  const id = (params.get("id") || "").trim();
  return type && id ? { type, id, label: `${type}:${id}` } : null;
}
