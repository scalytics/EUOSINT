import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import App from "@/App";

const mockedUseAgentOpsState = vi.fn();

vi.mock("@/hooks/useAgentOpsState", () => ({
  useAgentOpsState: () => mockedUseAgentOpsState(),
}));

vi.mock("@/osint/OsintApp", () => ({
  default: () => <div>OSINT APP</div>,
}));

vi.mock("@/agentops/AgentOpsApp", () => ({
  AgentOpsApp: ({ mode }: { mode: string }) => <div>AGENTOPS APP {mode}</div>,
}));

beforeEach(() => {
  mockedUseAgentOpsState.mockReset();
});

test("renders the OSINT app when ui_mode is OSINT", () => {
  mockedUseAgentOpsState.mockReturnValue({
    state: { ui_mode: "OSINT" },
    isLoading: false,
  });

  render(<App />);

  expect(screen.getByText("OSINT APP")).toBeTruthy();
  expect(document.title).toBe("OSINT");
});

test("renders the AgentOps app when ui_mode is AGENTOPS", () => {
  mockedUseAgentOpsState.mockReturnValue({
    state: { ui_mode: "AGENTOPS" },
    isLoading: false,
  });

  render(<App />);

  expect(screen.getByText("AGENTOPS APP AGENTOPS")).toBeTruthy();
  expect(document.title).toBe("Agent Flow Desk");
});

test("renders the AgentOps app when ui_mode is HYBRID", () => {
  mockedUseAgentOpsState.mockReturnValue({
    state: { ui_mode: "HYBRID" },
    isLoading: false,
  });

  render(<App />);

  expect(screen.getByText("AGENTOPS APP HYBRID")).toBeTruthy();
  expect(document.title).toBe("Hybrid Flow Desk");
});
