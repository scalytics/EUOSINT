import { appURL } from "@/lib/app-url";
import type { AgentOpsMode } from "@/agentops/types";

export type AgentOpsDemoMode = "ontology" | "fusion";
export type AgentOpsDemoScenario = "all" | "drones" | "scada";

export function currentDemoMode(): AgentOpsDemoMode | null {
  if (typeof window === "undefined") return null;
  const value = new URLSearchParams(window.location.search).get("demo")?.trim().toLowerCase();
  switch (value) {
    case "agentops":
    case "ontology":
      return "ontology";
    case "hybrid":
    case "fusion":
      return "fusion";
    default:
      return null;
  }
}

export function isAgentOpsDemo(): boolean {
  return currentDemoMode() !== null;
}

export function demoShellMode(): AgentOpsMode | null {
  const mode = currentDemoMode();
  if (mode === "fusion") return "HYBRID";
  if (mode === "ontology") return "AGENTOPS";
  return null;
}

export function demoLabel(): string {
  const mode = currentDemoMode();
  if (mode === "fusion") return "Fusion demo";
  if (mode === "ontology") return "Ontology demo";
  return "";
}

export function currentDemoScenario(): AgentOpsDemoScenario {
  if (typeof window === "undefined") return "all";
  const params = new URLSearchParams(window.location.search);
  const value = (params.get("scenario") || params.get("pack") || "").trim().toLowerCase();
  return value === "drones" || value === "scada" ? value : "all";
}

export function demoScenarioHref(scenario: AgentOpsDemoScenario): string {
  const params = new URLSearchParams();
  params.set("demo", currentDemoMode() ?? "ontology");
  if (scenario !== "all") params.set("scenario", scenario);
  return `?${params.toString()}`;
}

export function alertsURL(): string {
  return isAgentOpsDemo() ? appURL("demo/alerts.json") : appURL("alerts.json");
}

export function agentOpsGroupsURL(): string {
  return isAgentOpsDemo() ? appURL("demo/agentops-groups.json") : "/api/agentops/groups";
}
