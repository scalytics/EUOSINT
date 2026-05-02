import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import App from "@/App";
import type { AgentOpsState } from "@/agentops/types";

const state: AgentOpsState = {
  generated_at: "",
  enabled: true,
  ui_mode: "AGENTOPS",
  profile: "agentops-default",
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
    mirror_failed_count: 0,
    rejected_by_reason: {},
    replay_active: 0,
    replay_last_record_count: 0,
    topic_health: [],
  },
  replay_sessions: [],
  flows: [],
  traces: [],
  tasks: [],
  messages: [],
};

const mockedUseAgentOpsState = vi.fn(() => ({ state, isLoading: false }));

vi.mock("@/hooks/useAgentOpsState", () => ({
  useAgentOpsState: () => mockedUseAgentOpsState(),
}));

vi.mock("@/agentops/AgentOpsApp", () => ({
  AgentOpsApp: ({ mode }: { mode: string }) => <div>AgentOps shell {mode}</div>,
}));

vi.mock("@/osint/OsintApp", () => ({
  default: () => <div>OSINT shell</div>,
}));

beforeEach(() => {
  mockedUseAgentOpsState.mockReset();
  mockedUseAgentOpsState.mockReturnValue({ state, isLoading: false });
});

test("uses the runtime AgentOps shell for Operations and Fusion modes", () => {
  const { rerender } = render(<App />);
  expect(screen.getByText("AgentOps shell AGENTOPS")).toBeTruthy();
  expect(document.title).toBe("Operations Console");

  mockedUseAgentOpsState.mockReturnValue({ state: { ...state, ui_mode: "HYBRID" }, isLoading: false });
  rerender(<App />);

  expect(screen.getByText("AgentOps shell HYBRID")).toBeTruthy();
  expect(document.title).toBe("Fusion Console");
});

test("leaves OSINT on the existing OSINT shell", () => {
  mockedUseAgentOpsState.mockReturnValue({ state: { ...state, ui_mode: "OSINT" }, isLoading: false });

  render(<App />);

  expect(screen.getByText("OSINT shell")).toBeTruthy();
  expect(document.title).toBe("OSINT");
});
