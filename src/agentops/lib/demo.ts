import { appURL } from "@/lib/app-url";
import type { AgentOpsMode } from "@/agentops/types";

export type AgentOpsDemoMode = "ontology" | "fusion";

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

export function alertsURL(): string {
  return isAgentOpsDemo() ? appURL("demo/alerts.json") : appURL("alerts.json");
}

export function agentOpsGroupsURL(): string {
  return isAgentOpsDemo() ? appURL("demo/agentops-groups.json") : "/api/agentops/groups";
}
