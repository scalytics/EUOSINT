import { appURL } from "@/lib/app-url";

type DemoMode = "agentops" | null;

function currentDemoMode(): DemoMode {
  if (typeof window === "undefined") return null;
  const value = new URLSearchParams(window.location.search).get("demo")?.trim().toLowerCase();
  return value === "agentops" ? "agentops" : null;
}

export function isAgentOpsDemo(): boolean {
  return currentDemoMode() === "agentops";
}

export function alertsURL(): string {
  return isAgentOpsDemo() ? appURL("demo/alerts.json") : appURL("alerts.json");
}

export function agentOpsGroupsURL(): string {
  return isAgentOpsDemo() ? appURL("demo/agentops-groups.json") : "/api/agentops/groups";
}
