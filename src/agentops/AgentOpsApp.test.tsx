import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { AgentOpsApp } from "@/agentops/AgentOpsApp";

vi.mock("@/agentops/pages/AgentOpsRuntimeDesk", () => ({
  AgentOpsRuntimeDesk: ({ mode }: { mode: string }) => <div>Runtime desk {mode}</div>,
}));

vi.mock("@/agentops/pages/EntityProfilePage", () => ({
  EntityProfilePage: ({ mode }: { mode: string }) => <div>Entity page {mode}</div>,
}));

beforeEach(() => {
  window.history.replaceState({}, "", "/");
});

test("routes Operations and Fusion to the runtime desk", () => {
  const { rerender } = render(<AgentOpsApp mode="AGENTOPS" />);
  expect(screen.getByText("Runtime desk AGENTOPS")).toBeTruthy();

  rerender(<AgentOpsApp mode="HYBRID" />);
  expect(screen.getByText("Runtime desk HYBRID")).toBeTruthy();
});

test("routes entity URLs to the entity profile surface", () => {
  window.history.replaceState({}, "", "/?view=entity&type=platform&id=auv-07");

  render(<AgentOpsApp mode="HYBRID" />);

  expect(screen.getByText("Entity page HYBRID")).toBeTruthy();
});
