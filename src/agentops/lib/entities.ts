import type { AgentOpsFlow, AgentOpsMessage, Pack, SearchResult } from "@/agentops/types";

export interface EntityRef {
  type: string;
  id: string;
  label?: string;
}

export interface CommandFilters {
  text?: string;
  topic?: string;
  status?: string;
  sender?: string;
}

export function splitEntityID(value: string | undefined): EntityRef | null {
  const raw = (value || "").trim();
  const index = raw.indexOf(":");
  if (index <= 0 || index === raw.length - 1) return null;
  return { type: raw.slice(0, index), id: raw.slice(index + 1), label: raw };
}

export function entityKey(ref: EntityRef): string {
  return `${ref.type}:${entityCanonicalID(ref)}`;
}

export function entityCanonicalID(ref: EntityRef): string {
  const prefix = `${ref.type}:`;
  return ref.id.startsWith(prefix) ? ref.id.slice(prefix.length) : ref.id;
}

export function entityLabel(ref: EntityRef): string {
  return ref.label || entityKey(ref);
}

export function entityHref(ref: EntityRef): string {
  const params = new URLSearchParams();
  params.set("view", "entity");
  params.set("type", ref.type);
  params.set("id", entityCanonicalID(ref));
  return `?${params.toString()}`;
}

export function entityFromSearchResult(result: SearchResult): EntityRef | null {
  if (result.kind !== "entity" || !result.type) return null;
  return {
    type: result.type,
    id: result.canonical_id || result.id,
    label: result.display_name || result.id,
  };
}

export function entityRefFromFlow(flow: AgentOpsFlow): EntityRef {
  return { type: "correlation", id: flow.id, label: flow.id };
}

export function parseCommandFilters(query: string): CommandFilters {
  const filters: CommandFilters = {};
  const free: string[] = [];
  for (const token of query.trim().split(/\s+/)) {
    if (!token) continue;
    const [rawKey, ...rest] = token.split(":");
    const value = rest.join(":").trim();
    if (!value) {
      free.push(token);
      continue;
    }
    const key = rawKey.toLowerCase();
    switch (key) {
      case "agent":
        filters.sender = value;
        break;
      case "topic":
        filters.topic = value;
        break;
      case "status":
        filters.status = value;
        break;
      case "window":
        break;
      default:
        free.push(token);
        break;
    }
  }
  if (free.length > 0) filters.text = free.join(" ");
  return filters;
}

export function refsForFlow(flow: AgentOpsFlow): EntityRef[] {
  return uniqueRefs([
    entityRefFromFlow(flow),
    ...flow.senders.map((sender) => ({ type: "agent", id: sender, label: sender })),
    ...flow.trace_ids.map((trace) => ({ type: "trace", id: trace, label: trace })),
    ...flow.task_ids.map((task) => ({ type: "task", id: task, label: task })),
    ...flow.topics.map((topic) => ({ type: "topic", id: topic, label: topic })),
  ]);
}

export function refsForMessage(message: AgentOpsMessage, packs: Pack[]): EntityRef[] {
  const refs: EntityRef[] = [];
  if (message.sender_id) refs.push({ type: "agent", id: message.sender_id, label: message.sender_id });
  if (message.trace_id) refs.push({ type: "trace", id: message.trace_id, label: message.trace_id });
  if (message.task_id) refs.push({ type: "task", id: message.task_id, label: message.task_id });
  if (message.correlation_id) refs.push({ type: "correlation", id: message.correlation_id, label: message.correlation_id });
  refs.push({ type: "topic", id: message.topic, label: message.topic });
  refs.push(...refsFromPackFields(message.content || message.preview || "", packs));
  return uniqueRefs(refs);
}

export function refsFromPackFields(raw: string, packs: Pack[]): EntityRef[] {
  const entityTypes = new Set<string>();
  for (const pack of packs) {
    for (const type of pack.entity_types ?? []) entityTypes.add(type);
  }
  const parsed = parseJSONRecord(raw);
  if (!parsed) return [];
  const refs: EntityRef[] = [];
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value !== "string" && typeof value !== "number") continue;
    const id = String(value).trim();
    if (!id) continue;
    if (entityTypes.has(key)) {
      refs.push({ type: key, id, label: `${key}:${id}` });
      continue;
    }
    if (key.endsWith("_id")) {
      const type = key.slice(0, -3);
      if (entityTypes.has(type) || ["agent", "task", "trace", "topic", "correlation"].includes(type)) {
        refs.push({ type, id, label: `${type}:${id}` });
      }
    }
  }
  return refs;
}

function parseJSONRecord(raw: string): Record<string, unknown> | null {
  const trimmed = raw.trim();
  if (!trimmed.startsWith("{")) return null;
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

function uniqueRefs(refs: EntityRef[]): EntityRef[] {
  const seen = new Set<string>();
  const out: EntityRef[] = [];
  for (const ref of refs) {
    const key = entityKey(ref);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(ref);
  }
  return out;
}
