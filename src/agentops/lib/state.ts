import type { AgentOpsHealth, AgentOpsMode, AgentOpsState } from "@/agentops/types";

const MODE_KEY = "kafsiem.ui_mode";
const PROFILE_KEY = "kafsiem.profile";

export function profileForMode(mode: AgentOpsMode): string {
  switch (mode) {
    case "AGENTOPS":
      return "agentops-default";
    case "HYBRID":
      return "hybrid-ops";
    case "OSINT":
    default:
      return "osint-default";
  }
}

export function displayModeName(mode: AgentOpsMode): "OSINT" | "Operations" | "Fusion" {
  switch (mode) {
    case "AGENTOPS":
      return "Operations";
    case "HYBRID":
      return "Fusion";
    case "OSINT":
    default:
      return "OSINT";
  }
}

export function normalizeMode(raw: string | undefined): AgentOpsMode {
  switch ((raw || "").toUpperCase()) {
    case "AGENTOPS":
      return "AGENTOPS";
    case "HYBRID":
      return "HYBRID";
    case "OSINT":
    default:
      return "OSINT";
  }
}

export function normalizeProfile(raw: string | undefined, mode: AgentOpsMode): string {
  const value = (raw || "").trim().toLowerCase();
  if (!value) return profileForMode(mode);
  switch (value) {
    case "agentops-default":
      return "agentops-default";
    case "hybrid-ops":
      return "hybrid-ops";
    case "osint-default":
      return "osint-default";
    default:
      return profileForMode(mode);
  }
}

export function normalizeHealth(health: Partial<AgentOpsHealth> | undefined): AgentOpsHealth {
  return {
    connected: false,
    effective_topics: [],
    group_id: "",
    accepted_count: 0,
    rejected_count: 0,
    mirrored_count: 0,
    mirror_failed_count: 0,
    rejected_by_reason: {},
    replay_active: 0,
    replay_last_record_count: 0,
    topic_health: [],
    ...health,
  };
}

export function normalizeState(data: Partial<AgentOpsState>): AgentOpsState {
  const mode = normalizeMode(data.ui_mode);
  const base: AgentOpsState = {
    generated_at: "",
    enabled: false,
    ui_mode: mode,
    profile: normalizeProfile(data.profile, mode),
    group_name: "",
    topics: [],
    flow_count: 0,
    trace_count: 0,
    task_count: 0,
    message_count: 0,
    health: normalizeHealth(data.health),
    replay_sessions: data.replay_sessions ?? [],
    flows: data.flows ?? [],
    traces: data.traces ?? [],
    tasks: data.tasks ?? [],
    messages: data.messages ?? [],
  };
  return {
    ...base,
    ...data,
    ui_mode: mode,
    profile: normalizeProfile(data.profile, mode),
    health: normalizeHealth(data.health),
  };
}

export function loadPersistedShell(): Pick<AgentOpsState, "ui_mode" | "profile"> {
  const storage = safeStorage();
  if (!storage) {
    return { ui_mode: "OSINT", profile: "osint-default" };
  }
  const mode = normalizeMode(storage.getItem(MODE_KEY) ?? undefined);
  return { ui_mode: mode, profile: normalizeProfile(storage.getItem(PROFILE_KEY) ?? undefined, mode) };
}

export function persistShell(mode: AgentOpsMode, profile: string) {
  const storage = safeStorage();
  if (!storage) return;
  storage.setItem(MODE_KEY, mode);
  storage.setItem(PROFILE_KEY, profile);
}

function safeStorage(): Storage | null {
  if (typeof window === "undefined" || !("localStorage" in window)) {
    return null;
  }
  const storage = window.localStorage;
  if (!storage || typeof storage.getItem !== "function" || typeof storage.setItem !== "function") {
    return null;
  }
  return storage;
}
