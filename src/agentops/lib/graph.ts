import type { Edge, Pack } from "@/agentops/types";

const palette = ["#38bdf8", "#22c55e", "#f59e0b", "#f97316", "#a78bfa", "#f43f5e", "#14b8a6", "#eab308"];

export function edgeColorMap(packs: Pack[]): Record<string, string> {
  const out: Record<string, string> = {};
  let index = 0;
  const assign = (type: string) => {
    if (!out[type]) {
      out[type] = palette[index % palette.length];
      index += 1;
    }
  };
  for (const type of ["sent", "responded", "spans", "mentions", "member_of", "delegated_to", "observed_at", "in_area"]) {
    assign(type);
  }
  for (const pack of packs) {
    for (const type of pack.edge_types ?? []) assign(type);
  }
  return out;
}

export function colorForEdge(edge: Edge, colors: Record<string, string>): string {
  return colors[edge.type] || "#94a3b8";
}
