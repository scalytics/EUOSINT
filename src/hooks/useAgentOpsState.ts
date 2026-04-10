import { useEffect, useState } from "react";
import { appURL } from "@/lib/app-url";
import type { AgentOpsState } from "@/types/agentops";

const AGENTOPS_STATE_URL = appURL("agentops-state.json");
const POLL_MS = 15000;

const FALLBACK_STATE: AgentOpsState = {
  generated_at: "",
  enabled: false,
  ui_mode: "OSINT",
  profile: "osint-default",
  group_name: "",
  topics: [],
  flow_count: 0,
  trace_count: 0,
  task_count: 0,
  message_count: 0,
  health: {
    connected: false,
    effective_topics: [],
    group_id: "",
    accepted_count: 0,
    rejected_count: 0,
    mirrored_count: 0,
    rejected_by_reason: {},
    topic_health: [],
  },
  replay_sessions: [],
  flows: [],
  traces: [],
  tasks: [],
  messages: [],
};

export function useAgentOpsState() {
  const [state, setState] = useState<AgentOpsState>(FALLBACK_STATE);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const response = await fetch(`${AGENTOPS_STATE_URL}?t=${Date.now()}`, { cache: "no-store" });
        if (!response.ok) {
          throw new Error(`agentops state fetch failed: ${response.status}`);
        }
        const data = (await response.json()) as AgentOpsState;
        if (!cancelled) {
          setState({ ...FALLBACK_STATE, ...data });
          setIsLoading(false);
        }
      } catch {
        if (!cancelled) {
          setState(FALLBACK_STATE);
          setIsLoading(false);
        }
      }
    }

    load();
    const id = window.setInterval(load, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  return { state, isLoading };
}
