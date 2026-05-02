import { useEffect, useMemo, useRef } from "react";
import cytoscape, { type Core, type ElementDefinition } from "cytoscape";
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
  const cyRef = useRef<Core | null>(null);
  const signatureRef = useRef("");
  const onEntityClickRef = useRef<typeof onEntityClick>(onEntityClick);
  const elements = useMemo(() => buildElements(entities, edges, edgeColors, selectedEntityId), [edgeColors, edges, entities, selectedEntityId]);
  const graphSignature = useMemo(() => graphShapeSignature(entities, edges, selectedEntityId), [edges, entities, selectedEntityId]);

  useEffect(() => {
    onEntityClickRef.current = onEntityClick;
  }, [onEntityClick]);

  useEffect(() => {
    if (!container.current || elements.length === 0 || cyRef.current) return undefined;
    const cy = cytoscape({
      container: container.current,
      elements: [],
      layout: { name: "cose", animate: false, fit: true, padding: 44 },
      style: [
        {
          selector: "node",
          style: {
            "background-color": "data(color)",
            "border-color": "data(border)",
            "border-width": 2,
            color: "#e2e8f0",
            "font-size": 9,
            label: "data(label)",
            "min-zoomed-font-size": 7,
            "text-background-color": "#020617",
            "text-background-opacity": 0.96,
            "text-background-padding": "2px",
            "text-border-color": "#0f172a",
            "text-border-opacity": 0.95,
            "text-border-width": 1,
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
            color: "#e2e8f0",
            "font-size": 7,
            "text-rotation": "autorotate",
            "text-background-color": "#020617",
            "text-background-opacity": 0.94,
            "text-background-padding": "1px",
            "text-border-color": "#0f172a",
            "text-border-opacity": 0.95,
            "text-border-width": 0.5,
            "text-margin-y": -8,
            "text-wrap": "wrap",
            "text-max-width": "72px",
            width: 1.5,
          },
        },
      ],
    });
    cyRef.current = cy;
    cy.on("tap", "node", (event) => {
      const id = String(event.target.data("id") || "");
      const ref = splitEntityID(id);
      if (ref) onEntityClickRef.current?.(ref);
    });

    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            cy.resize();
          });
    if (observer && container.current) observer.observe(container.current);

    return () => {
      observer?.disconnect();
      cyRef.current = null;
      signatureRef.current = "";
      cy.destroy();
    };
  }, [elements.length]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy || elements.length === 0) return;

    if (signatureRef.current === graphSignature) {
      return;
    }

    signatureRef.current = graphSignature;
    cy.elements().remove();
    cy.add(elements);
    cy.layout({
      name: "cose",
      animate: false,
      fit: true,
      padding: 44,
      randomize: false,
      nodeRepulsion: 160000,
      idealEdgeLength: 110,
      edgeElasticity: 120,
      nestingFactor: 0.9,
      gravity: 1,
      componentSpacing: 120,
    }).run();
  }, [elements, graphSignature, selectedEntityId]);

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

function graphShapeSignature(entities: Entity[], edges: Edge[], selectedEntityId?: string): string {
  const nodeIDs = entities.map((entity) => entity.id).sort();
  const edgeIDs = edges.map((edge) => `${edge.src_id}:${edge.type}:${edge.dst_id}`).sort();
  return JSON.stringify({ nodeIDs, edgeIDs, selectedEntityId: selectedEntityId || "" });
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
