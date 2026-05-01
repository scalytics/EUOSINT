import { useEffect, useMemo, useRef } from "react";
import cytoscape, { type ElementDefinition } from "cytoscape";
import { EmptyState, Tag } from "@/agentops/components/Chrome";
import { colorForEdge } from "@/agentops/lib/graph";
import { splitEntityID, type EntityRef } from "@/agentops/lib/entities";
import type { Edge, Entity } from "@/agentops/types";

interface Props {
  entities: Entity[];
  edges: Edge[];
  edgeColors: Record<string, string>;
  selectedEntityId?: string;
  onEntityClick?: (entity: EntityRef) => void;
}

export function GraphCanvas({ entities, edges, edgeColors, selectedEntityId, onEntityClick }: Props) {
  const container = useRef<HTMLDivElement | null>(null);
  const elements = useMemo(() => buildElements(entities, edges, edgeColors, selectedEntityId), [edgeColors, edges, entities, selectedEntityId]);

  useEffect(() => {
    if (!container.current || elements.length === 0) return undefined;
    const cy = cytoscape({
      container: container.current,
      elements,
      layout: { name: "cose", animate: false, fit: true, padding: 44 },
      style: [
        {
          selector: "node",
          style: {
            "background-color": "data(color)",
            "border-color": "data(border)",
            "border-width": 2,
            color: "#e2e8f0",
            "font-size": 10,
            label: "data(label)",
            "min-zoomed-font-size": 7,
            "text-background-color": "#020617",
            "text-background-opacity": 0.8,
            "text-background-padding": "3px",
            "text-valign": "bottom",
            "text-wrap": "wrap",
            width: "data(size)",
            height: "data(size)",
          },
        },
        {
          selector: "edge",
          style: {
            "curve-style": "bezier",
            "line-color": "data(color)",
            "target-arrow-color": "data(color)",
            "target-arrow-shape": "triangle",
            label: "data(label)",
            color: "#94a3b8",
            "font-size": 8,
            "text-rotation": "autorotate",
            width: 1.5,
          },
        },
      ],
    });
    cy.on("tap", "node", (event) => {
      const id = String(event.target.data("id") || "");
      const ref = splitEntityID(id);
      if (ref) onEntityClick?.(ref);
    });

    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            cy.resize();
            cy.fit(undefined, 44);
          });
    if (observer && container.current) observer.observe(container.current);

    const pulse = window.setInterval(() => {
      const now = Date.now() / 700;
      cy.edges().forEach((edge, index) => {
        const phase = (Math.sin(now + index) + 1) / 2;
        edge.style({
          opacity: 0.58 + phase * 0.34,
          width: 1.2 + phase * 1.2,
        });
      });
    }, 700);

    return () => {
      observer?.disconnect();
      window.clearInterval(pulse);
      cy.destroy();
    };
  }, [elements, onEntityClick]);

  if (elements.length === 0) {
    return <EmptyState text="No neighborhood graph returned for this subject." />;
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div ref={container} className="min-h-0 flex-1 overflow-hidden border border-siem-border bg-siem-bg/45" />
      <div className="flex flex-wrap gap-2 text-[11px] text-siem-muted">
        <Tag>{entities.length} entities</Tag>
        <Tag>{edges.length} edges</Tag>
      </div>
    </div>
  );
}

function buildElements(entities: Entity[], edges: Edge[], edgeColors: Record<string, string>, selectedEntityId?: string): ElementDefinition[] {
  const nodes = new Map<string, ElementDefinition>();
  for (const entity of entities) {
    nodes.set(entity.id, {
      data: {
        id: entity.id,
        label: entity.display_name || entity.canonical_id || entity.id,
        color: selectedEntityId === entity.id ? "#38bdf8" : "#0f766e",
        border: selectedEntityId === entity.id ? "#bae6fd" : "#14b8a6",
        size: selectedEntityId === entity.id ? 42 : 34,
      },
    });
  }
  for (const edge of edges) {
    if (!nodes.has(edge.src_id)) nodes.set(edge.src_id, fallbackNode(edge.src_id, selectedEntityId));
    if (!nodes.has(edge.dst_id)) nodes.set(edge.dst_id, fallbackNode(edge.dst_id, selectedEntityId));
  }
  const edgeElements = edges.map((edge, index) => ({
    data: {
      id: `${edge.src_id}-${edge.type}-${edge.dst_id}-${index}`,
      source: edge.src_id,
      target: edge.dst_id,
      label: edge.type,
      color: colorForEdge(edge, edgeColors),
    },
  }));
  return [...nodes.values(), ...edgeElements];
}

function fallbackNode(id: string, selectedEntityId?: string): ElementDefinition {
  return {
    data: {
      id,
      label: id,
      color: selectedEntityId === id ? "#38bdf8" : "#334155",
      border: selectedEntityId === id ? "#bae6fd" : "#64748b",
      size: selectedEntityId === id ? 42 : 30,
    },
  };
}
