import type {
  AgentOpsApiClient,
  Cursor,
  Edge,
  Entity,
  FeatureCollection,
  Flow,
  Health,
  ListResponse,
  MapLayer,
  Message,
  NeighborhoodResponse,
  Pack,
  Profile,
  Provenance,
  ReplayRequest,
  ReplaySession,
  SearchResponse,
  Task,
  TopicHealth,
  Trace,
} from "@/agentops/lib/api-client";

export type AgentOpsDataSource = Pick<
  AgentOpsApiClient,
  | "createReplayRequest"
  | "getEntity"
  | "getEntityNeighborhood"
  | "getFlow"
  | "getHealth"
  | "getOntologyPacks"
  | "listEntityProvenance"
  | "listEntityTimeline"
  | "listFlowMessages"
  | "listFlowTasks"
  | "listFlowTraces"
  | "listFlows"
  | "listMapFeatures"
  | "listMapLayers"
  | "listReplays"
  | "listTopicHealth"
  | "searchEntities"
>;

const TOPICS = [
  "group.drones.requests",
  "group.drones.traces",
  "group.drones.observe.audit",
  "group.scada.requests",
  "group.scada.tasks.status",
  "group.scada.memory.context",
];

function tick(): number {
  return Math.floor(Date.now() / 5000);
}

function iso(minutesAgo: number): string {
  return new Date(Date.now() - minutesAgo * 60_000).toISOString();
}

function list<T>(items: T[], next: Cursor = null): ListResponse<T> {
  return { items, next };
}

function content(data: Record<string, unknown>): string {
  return JSON.stringify(data);
}

function flowSeed(): Flow[] {
  const t = tick() % 12;
  return [
    {
      id: "corr-drone-ew-1",
      topic_count: 3,
      sender_count: 4,
      topics: ["group.drones.requests", "group.drones.traces", "group.drones.observe.audit"],
      senders: ["mission-planner", "ew-sentinel", "route-planner", "safety-officer"],
      trace_ids: ["trace-auv-07-sortie"],
      task_ids: ["task-launch-readiness", "task-ew-mitigation"],
      first_seen: iso(56),
      last_seen: iso(1),
      latest_status: "triage_required",
      message_count: 8 + t,
      latest_preview: "AUV-07 telemetry gap near Mtarfa ridge; EW advisory joined",
    },
    {
      id: "corr-scada-fw-1",
      topic_count: 3,
      sender_count: 3,
      topics: ["group.scada.requests", "group.scada.tasks.status", "group.scada.memory.context"],
      senders: ["plant-operator", "firmware-checker", "change-approver"],
      trace_ids: ["trace-rtu-12-firmware"],
      task_ids: ["task-firmware-exception"],
      first_seen: iso(39),
      last_seen: iso(4),
      latest_status: "active",
      message_count: 6 + (t % 4),
      latest_preview: "RTU-12 firmware exception linked to vendor advisory",
    },
    {
      id: "corr-satcom-cve-1",
      topic_count: 2,
      sender_count: 2,
      topics: ["group.drones.requests", "group.drones.memory.context"],
      senders: ["link-auditor", "mission-planner"],
      trace_ids: ["trace-link-cve"],
      task_ids: ["task-satcom-cve"],
      first_seen: iso(91),
      last_seen: iso(27),
      latest_status: "completed",
      message_count: 5,
      latest_preview: "Satcom edge appliance review closed with compensating controls",
    },
  ];
}

const PACKS: Pack[] = [
  {
    name: "drones",
    version: "0.1.0",
    description: "Mock unmanned systems ontology stream",
    entity_types: ["platform", "payload", "area", "location", "mission", "finding"],
    edge_types: ["observed_at", "assigned_to", "reported_by", "affects", "mitigates"],
    detectors: [
      {
        id: "drones.telemetry_gap",
        severity: "high",
        window: "15m",
        query: "platform with missing heartbeat and active sortie",
        source: "demo",
      },
      {
        id: "drones.ew_overlap",
        severity: "medium",
        window: "30m",
        query: "platform location intersects EW advisory area",
        source: "demo",
      },
    ],
    map_layers: [
      { id: "platform", name: "Platforms", kind: "point", entity_types: ["platform"], source: "demo" },
      { id: "area", name: "Areas", kind: "polygon", entity_types: ["area"], source: "demo" },
      { id: "location", name: "Locations", kind: "point", entity_types: ["location"], source: "demo" },
    ],
    views: [
      {
        id: "platform-summary",
        entity_type: "platform",
        title: "Platform",
        source: "demo",
        fields: [
          { id: "readiness", label: "Readiness" },
          { id: "mission", label: "Mission" },
          { id: "battery", label: "Battery" },
          { id: "country_code", label: "Country" },
        ],
      },
      {
        id: "correlation-summary",
        entity_type: "correlation",
        title: "Correlation",
        source: "demo",
        fields: [
          { id: "status", label: "Status" },
          { id: "priority", label: "Priority" },
          { id: "scenario", label: "Scenario" },
          { id: "operator", label: "Operator" },
        ],
      },
    ],
  },
  {
    name: "scada",
    version: "0.1.0",
    description: "Mock critical infrastructure ontology stream",
    entity_types: ["plant", "device", "zone", "asset", "vendor", "vulnerability"],
    edge_types: ["controls", "depends_on", "has_vulnerability", "located_in", "owned_by"],
    detectors: [
      {
        id: "scada.unapproved_firmware",
        severity: "high",
        window: "1h",
        query: "device firmware mismatch with open change ticket",
        source: "demo",
      },
    ],
    map_layers: [
      { id: "plant", name: "Plants", kind: "point", entity_types: ["plant"], source: "demo" },
      { id: "device", name: "Devices", kind: "point", entity_types: ["device"], source: "demo" },
    ],
    views: [
      {
        id: "device-summary",
        entity_type: "device",
        title: "Device",
        source: "demo",
        fields: [
          { id: "firmware", label: "Firmware" },
          { id: "zone", label: "Zone" },
          { id: "vendor", label: "Vendor" },
          { id: "risk", label: "Risk" },
        ],
      },
    ],
  },
];

const ENTITIES: Entity[] = [
  {
    id: "correlation:corr-drone-ew-1",
    type: "correlation",
    canonical_id: "corr-drone-ew-1",
    display_name: "AUV-07 EW telemetry gap",
    first_seen: iso(56),
    last_seen: iso(1),
    attrs: { status: "triage_required", priority: "high", scenario: "sortie-readiness", operator: "mission-planner" },
  },
  {
    id: "correlation:corr-scada-fw-1",
    type: "correlation",
    canonical_id: "corr-scada-fw-1",
    display_name: "RTU-12 firmware exception",
    first_seen: iso(39),
    last_seen: iso(4),
    attrs: { status: "active", priority: "medium", scenario: "plant-change-control", operator: "plant-operator" },
  },
  {
    id: "correlation:corr-satcom-cve-1",
    type: "correlation",
    canonical_id: "corr-satcom-cve-1",
    display_name: "Satcom edge appliance review",
    first_seen: iso(91),
    last_seen: iso(27),
    attrs: { status: "completed", priority: "low", scenario: "edge-appliance-review", operator: "link-auditor" },
  },
  {
    id: "platform:auv-07",
    type: "platform",
    canonical_id: "auv-07",
    display_name: "AUV-07",
    first_seen: iso(140),
    last_seen: iso(1),
    attrs: { readiness: "degraded", mission: "harbor ISR", battery: "62%", country_code: "DE", vendor: "F5", product: "BIG-IP", cve: "CVE-2026-12345" },
  },
  {
    id: "area:mtarfa-ridge",
    type: "area",
    canonical_id: "mtarfa-ridge",
    display_name: "Mtarfa Ridge EW Box",
    first_seen: iso(190),
    last_seen: iso(1),
    attrs: { country_code: "DE", sector: "critical infrastructure", category: "cyber_advisory" },
  },
  {
    id: "location:mtarfa-telemetry",
    type: "location",
    canonical_id: "mtarfa-telemetry",
    display_name: "Telemetry Station M-4",
    first_seen: iso(200),
    last_seen: iso(2),
    attrs: { country_code: "DE", lat: 35.895, lon: 14.405 },
  },
  {
    id: "finding:ew-interference",
    type: "finding",
    canonical_id: "ew-interference",
    display_name: "EW interference hypothesis",
    first_seen: iso(52),
    last_seen: iso(1),
    attrs: { severity: "high", confidence: "0.78", category: "cyber_advisory" },
  },
  {
    id: "plant:desal-east",
    type: "plant",
    canonical_id: "desal-east",
    display_name: "East Desalination Plant",
    first_seen: iso(400),
    last_seen: iso(4),
    attrs: { sector: "water", country_code: "DE" },
  },
  {
    id: "device:rtu-12",
    type: "device",
    canonical_id: "rtu-12",
    display_name: "RTU-12",
    first_seen: iso(300),
    last_seen: iso(4),
    attrs: { firmware: "4.2.1", zone: "intake", vendor: "F5", risk: "high", cve: "CVE-2026-12345" },
  },
  {
    id: "vulnerability:CVE-2026-12345",
    type: "vulnerability",
    canonical_id: "CVE-2026-12345",
    display_name: "CVE-2026-12345",
    first_seen: iso(320),
    last_seen: iso(4),
    attrs: { vendor: "F5", product: "BIG-IP", severity: "critical" },
  },
];

function edge(src: string, dst: string, type: string, weight = 0.8): Edge {
  return {
    src_id: src,
    dst_id: dst,
    type,
    valid_from: iso(60),
    weight,
    evidence_msg: "demo-stream",
  };
}

function messagesFor(flowId: string): Message[] {
  const t = tick() % 50;
  const dynamicOffset = 1200 + t;
  const data: Record<string, Message[]> = {
    "corr-drone-ew-1": [
      {
        id: "msg-drone-req-1",
        topic: "group.drones.requests",
        topic_family: "requests",
        partition: 0,
        offset: 1188,
        timestamp: iso(54),
        envelope_type: "request",
        sender_id: "mission-planner",
        correlation_id: flowId,
        trace_id: "trace-auv-07-sortie",
        task_id: "task-launch-readiness",
        status: "requested",
        preview: "Launch readiness check for AUV-07",
        content: content({ platform: "auv-07", area: "mtarfa-ridge", location: "mtarfa-telemetry", category: "cyber_advisory", country_code: "DE", vendor: "F5", product: "BIG-IP", cve: "CVE-2026-12345" }),
      },
      {
        id: "msg-drone-gap-1",
        topic: "group.drones.observe.audit",
        topic_family: "audit",
        partition: 0,
        offset: 1192,
        timestamp: iso(18),
        envelope_type: "observation",
        sender_id: "ew-sentinel",
        correlation_id: flowId,
        trace_id: "trace-auv-07-sortie",
        task_id: "task-ew-mitigation",
        status: "triage_required",
        preview: "Telemetry gap overlaps EW advisory area",
        content: content({ platform: "auv-07", finding: "ew-interference", area: "mtarfa-ridge", sector: "critical infrastructure", category: "cyber_advisory", country_code: "DE" }),
      },
      {
        id: `msg-drone-stream-${t}`,
        topic: "group.drones.traces",
        topic_family: "traces",
        partition: 1,
        offset: dynamicOffset,
        timestamp: iso(1),
        envelope_type: "trace",
        sender_id: "route-planner",
        correlation_id: flowId,
        trace_id: "trace-auv-07-sortie",
        task_id: "task-ew-mitigation",
        status: "streaming",
        preview: `Mock stream heartbeat ${t}: mitigation route recomputed`,
        content: content({ platform: "auv-07", mission: "harbor-isr", area: "mtarfa-ridge", product: "BIG-IP", vendor: "F5" }),
      },
    ],
    "corr-scada-fw-1": [
      {
        id: "msg-scada-req-1",
        topic: "group.scada.requests",
        topic_family: "requests",
        partition: 0,
        offset: 530,
        timestamp: iso(38),
        envelope_type: "request",
        sender_id: "plant-operator",
        correlation_id: flowId,
        trace_id: "trace-rtu-12-firmware",
        task_id: "task-firmware-exception",
        status: "requested",
        preview: "Review RTU-12 firmware exception",
        content: content({ plant: "desal-east", device: "rtu-12", vendor: "F5", product: "BIG-IP", cve: "CVE-2026-12345", category: "cyber_advisory", country_code: "DE" }),
      },
      {
        id: "msg-scada-status-1",
        topic: "group.scada.tasks.status",
        topic_family: "tasks",
        partition: 0,
        offset: 544,
        timestamp: iso(4),
        envelope_type: "task_status",
        sender_id: "firmware-checker",
        correlation_id: flowId,
        trace_id: "trace-rtu-12-firmware",
        task_id: "task-firmware-exception",
        status: "active",
        preview: "Firmware mismatch confirmed; awaiting change approval",
        content: content({ plant: "desal-east", device: "rtu-12", vulnerability: "CVE-2026-12345", sector: "water" }),
      },
    ],
    "corr-satcom-cve-1": [
      {
        id: "msg-satcom-1",
        topic: "group.drones.memory.context",
        topic_family: "memory",
        partition: 0,
        offset: 300,
        timestamp: iso(90),
        envelope_type: "context",
        sender_id: "link-auditor",
        correlation_id: flowId,
        trace_id: "trace-link-cve",
        task_id: "task-satcom-cve",
        status: "completed",
        preview: "Edge appliance compensating controls recorded",
        content: content({ platform: "auv-07", vendor: "F5", product: "BIG-IP", cve: "CVE-2026-12345", category: "cyber_advisory" }),
      },
    ],
  };
  return data[flowId] ?? [];
}

function tasksFor(flowId: string): Task[] {
  if (flowId === "corr-drone-ew-1") {
    return [
      {
        id: "task-launch-readiness",
        requester_id: "mission-planner",
        responder_id: "safety-officer",
        status: "blocked",
        description: "Gate AUV-07 launch readiness against telemetry confidence",
        last_summary: "Battery acceptable; telemetry confidence degraded by EW overlap.",
        first_seen: iso(54),
        last_seen: iso(18),
      },
      {
        id: "task-ew-mitigation",
        parent_task_id: "task-launch-readiness",
        delegation_depth: 1,
        requester_id: "safety-officer",
        responder_id: "route-planner",
        status: "triage_required",
        description: "Compute mitigation route around EW advisory",
        last_summary: "Route recompute stream is live.",
        first_seen: iso(19),
        last_seen: iso(1),
      },
    ];
  }
  if (flowId === "corr-scada-fw-1") {
    return [
      {
        id: "task-firmware-exception",
        requester_id: "plant-operator",
        responder_id: "firmware-checker",
        status: "active",
        description: "Check firmware exception for RTU-12",
        last_summary: "Exception requires change approval.",
        first_seen: iso(38),
        last_seen: iso(4),
      },
    ];
  }
  return [
    {
      id: "task-satcom-cve",
      requester_id: "link-auditor",
      responder_id: "mission-planner",
      status: "completed",
      description: "Review satcom edge appliance advisory",
      last_summary: "Controls accepted.",
      first_seen: iso(90),
      last_seen: iso(27),
    },
  ];
}

function tracesFor(flowId: string): Trace[] {
  if (flowId === "corr-drone-ew-1") {
    return [
      {
        id: "trace-auv-07-sortie",
        span_count: 7 + (tick() % 4),
        agents: ["mission-planner", "ew-sentinel", "route-planner"],
        span_types: ["request", "observation", "route_update"],
        latest_title: "AUV-07 launch readiness",
        started_at: iso(54),
        ended_at: iso(1),
        duration_ms: 3_180_000,
      },
    ];
  }
  if (flowId === "corr-scada-fw-1") {
    return [
      {
        id: "trace-rtu-12-firmware",
        span_count: 5,
        agents: ["plant-operator", "firmware-checker"],
        span_types: ["request", "approval_gate"],
        latest_title: "RTU-12 firmware exception",
        started_at: iso(38),
        ended_at: iso(4),
        duration_ms: 2_040_000,
      },
    ];
  }
  return [
    {
      id: "trace-link-cve",
      span_count: 4,
      agents: ["link-auditor", "mission-planner"],
      span_types: ["context", "control"],
      latest_title: "Satcom advisory review",
      started_at: iso(90),
      ended_at: iso(27),
      duration_ms: 1_260_000,
    },
  ];
}

function profileFor(type: string, id: string): Profile {
  const full = id.startsWith(`${type}:`) ? id : `${type}:${id}`;
  const entity = ENTITIES.find((item) => item.id === full || (item.type === type && item.canonical_id === id)) ?? {
    id: full,
    type,
    canonical_id: id,
    display_name: full,
    first_seen: iso(60),
    last_seen: iso(1),
    attrs: {},
  };
  const edges = neighborhoodFor(type, id).edges;
  const edgeCounts = edges.reduce<Record<string, number>>((acc, item) => {
    acc[item.type] = (acc[item.type] ?? 0) + 1;
    return acc;
  }, {});
  return {
    entity,
    first_seen: entity.first_seen,
    last_seen: entity.last_seen,
    edge_counts: edgeCounts,
    top_neighbors: edges
      .filter((item) => item.src_id === entity.id || item.dst_id === entity.id)
      .slice(0, 4)
      .map((item) => ({
        entity_id: item.src_id === entity.id ? item.dst_id : item.src_id,
        entity_type: (item.src_id === entity.id ? item.dst_id : item.src_id).split(":")[0] || "entity",
        weight: item.weight,
      })),
  };
}

function neighborhoodFor(type: string, id: string): NeighborhoodResponse {
  const key = id.startsWith(`${type}:`) ? id : `${type}:${id}`;
  if (key === "correlation:corr-drone-ew-1") {
    const ids = ["correlation:corr-drone-ew-1", "platform:auv-07", "area:mtarfa-ridge", "location:mtarfa-telemetry", "finding:ew-interference", "vulnerability:CVE-2026-12345"];
    return {
      entities: ENTITIES.filter((item) => ids.includes(item.id)),
      edges: [
        edge("correlation:corr-drone-ew-1", "platform:auv-07", "observed_subject", 0.96),
        edge("platform:auv-07", "area:mtarfa-ridge", "observed_at", 0.82),
        edge("area:mtarfa-ridge", "location:mtarfa-telemetry", "covers", 0.71),
        edge("finding:ew-interference", "platform:auv-07", "affects", 0.77),
        edge("vulnerability:CVE-2026-12345", "platform:auv-07", "external_context", 0.63),
      ],
    };
  }
  if (key === "correlation:corr-scada-fw-1") {
    const ids = ["correlation:corr-scada-fw-1", "plant:desal-east", "device:rtu-12", "vulnerability:CVE-2026-12345"];
    return {
      entities: ENTITIES.filter((item) => ids.includes(item.id)),
      edges: [
        edge("correlation:corr-scada-fw-1", "device:rtu-12", "observed_subject", 0.91),
        edge("plant:desal-east", "device:rtu-12", "controls", 0.8),
        edge("device:rtu-12", "vulnerability:CVE-2026-12345", "has_vulnerability", 0.69),
      ],
    };
  }
  const center = ENTITIES.find((item) => item.id === key || (item.type === type && item.canonical_id === id));
  if (!center) return { entities: [], edges: [] };
  return {
    entities: [center, ...ENTITIES.filter((item) => item.id !== center.id).slice(0, 4)],
    edges: [
      edge(center.id, "correlation:corr-drone-ew-1", "seen_in", 0.72),
      edge(center.id, "vulnerability:CVE-2026-12345", "context", 0.48),
    ],
  };
}

function provenanceFor(type: string, id: string): Provenance[] {
  const subject = id.startsWith(`${type}:`) ? id : `${type}:${id}`;
  return [
    {
      subject_kind: "entity",
      subject_id: subject,
      stage: "ingest",
      policy_ver: "demo-stream-v1",
      inputs: { topic: "group.drones.requests", offset: 1188 },
      decision: "accepted",
      reasons: ["typed-envelope", "pack-field-match"],
      produced_at: iso(54),
    },
    {
      subject_kind: "entity",
      subject_id: subject,
      stage: "ontology",
      policy_ver: "demo-stream-v1",
      inputs: { pack: "drones", entity_type: type },
      decision: "linked",
      reasons: ["canonical-id", "correlation-window"],
      produced_at: iso(18),
    },
    {
      subject_kind: "entity",
      subject_id: subject,
      stage: "fusion",
      policy_ver: "demo-stream-v1",
      inputs: { alert_id: "advisory-f5-2026-12345" },
      decision: "candidate-context",
      reasons: ["vendor:F5", "cve:CVE-2026-12345"],
      produced_at: iso(2),
    },
  ];
}

function mapFeatures(params: { types?: string }): FeatureCollection {
  const selected = new Set((params.types || "").split(",").map((item) => item.trim()).filter(Boolean));
  const features: FeatureCollection["features"] = [
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.405, 35.895] },
      properties: { id: "platform:auv-07", type: "platform", display_name: "AUV-07" },
    },
    {
      type: "Feature",
      geometry: {
        type: "Polygon",
        coordinates: [[[14.34, 35.84], [14.48, 35.84], [14.48, 35.94], [14.34, 35.94], [14.34, 35.84]]],
      },
      properties: { id: "area:mtarfa-ridge", type: "area", display_name: "Mtarfa Ridge EW Box" },
    },
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.48, 35.91] },
      properties: { id: "location:mtarfa-telemetry", type: "location", display_name: "Telemetry Station M-4" },
    },
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.52, 35.88] },
      properties: { id: "plant:desal-east", type: "plant", display_name: "East Desalination Plant" },
    },
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.525, 35.881] },
      properties: { id: "device:rtu-12", type: "device", display_name: "RTU-12" },
    },
  ];
  return {
    type: "FeatureCollection",
    features: selected.size === 0 ? features : features.filter((item) => selected.has(String(item.properties.type))),
  };
}

function search(q: string): SearchResponse {
  const value = q.trim().toLowerCase();
  if (!value) return list([]);
  const typedIndex = value.indexOf(":");
  const requestedType = typedIndex > 0 ? value.slice(0, typedIndex) : "";
  const entityNeedle = value.replace(/^[^:]+:/, "");
  const flows = flowSeed()
    .filter((flow) => [flow.id, flow.latest_preview, ...flow.topics, ...flow.senders].join(" ").toLowerCase().includes(value.replace(/^status:/, "")))
    .map((flow) => ({
      kind: "flow" as const,
      id: flow.id,
      title: flow.latest_preview,
      latest_status: flow.latest_status,
      message_count: flow.message_count,
      first_seen: flow.first_seen,
      last_seen: flow.last_seen,
      score: 0.88,
    }));
  const entities = ENTITIES.filter((entity) => [entity.id, entity.canonical_id, entity.display_name].join(" ").toLowerCase().includes(entityNeedle))
    .map((entity) => ({
      kind: "entity" as const,
      id: entity.id,
      type: entity.type,
      canonical_id: entity.canonical_id,
      display_name: entity.display_name,
      first_seen: entity.first_seen,
      last_seen: entity.last_seen,
      attrs: entity.attrs,
      score: requestedType && entity.type === requestedType && entity.canonical_id.toLowerCase() === entityNeedle ? 1 : 0.94,
    }))
    .sort((left, right) => right.score - left.score || left.id.localeCompare(right.id));
  const detectorHits = PACKS.flatMap((pack) => pack.detectors ?? [])
    .filter((detector) => [detector.id, detector.query].join(" ").toLowerCase().includes(value))
    .map((detector) => ({
      kind: "detector_hit" as const,
      id: detector.id,
      detector_id: detector.id,
      title: detector.query,
      severity: detector.severity,
      source: detector.source,
      score: 0.74,
    }));
  return list([...entities, ...flows, ...detectorHits].slice(0, 10));
}

function applyFlowFilter(flows: Flow[], params: { topic?: string; sender?: string; status?: string; q?: string; limit?: number }): Flow[] {
  const q = params.q?.trim().toLowerCase();
  let out = flows;
  if (params.topic) out = out.filter((flow) => flow.topics.some((topic) => topic.includes(params.topic || "")));
  if (params.sender) out = out.filter((flow) => flow.senders.some((sender) => sender.includes(params.sender || "")));
  if (params.status) out = out.filter((flow) => (flow.latest_status || "").toLowerCase().includes((params.status || "").toLowerCase()));
  if (q) out = out.filter((flow) => [flow.id, flow.latest_preview, ...flow.topics, ...flow.senders].join(" ").toLowerCase().includes(q));
  return out.slice(0, params.limit ?? 50);
}

export const mockAgentOpsApiClient: AgentOpsDataSource = {
  createReplayRequest(topics: string[] = []): Promise<ReplayRequest> {
    return Promise.resolve({
      id: `demo-replay-${tick()}`,
      status: "accepted",
      requested_at: iso(0),
      topics,
    });
  },
  getEntity(type: string, id: string): Promise<Profile> {
    return Promise.resolve(profileFor(type, id));
  },
  getEntityNeighborhood(type: string, id: string): Promise<NeighborhoodResponse> {
    return Promise.resolve(neighborhoodFor(type, id));
  },
  getFlow(id: string): Promise<Flow> {
    return Promise.resolve(flowSeed().find((flow) => flow.id === id) ?? flowSeed()[0]);
  },
  getHealth(): Promise<Health> {
    const t = tick() % 120;
    return Promise.resolve({
      connected: true,
      effective_topics: TOPICS,
      group_id: "kafsiem-ontology-demo",
      accepted_count: 620 + t * 3,
      rejected_count: 2,
      mirrored_count: 2,
      mirror_failed_count: 0,
      rejected_by_reason: { schema_mismatch: 1, missing_correlation: 1 },
      last_poll_at: iso(0),
      replay_status: "idle",
      replay_active: 0,
      replay_last_record_count: 84,
      topic_health: topicHealth(),
    });
  },
  getOntologyPacks(): Promise<ListResponse<Pack>> {
    return Promise.resolve(list(PACKS));
  },
  listEntityProvenance(type: string, id: string): Promise<ListResponse<Provenance>> {
    return Promise.resolve(list(provenanceFor(type, id)));
  },
  listEntityTimeline(type: string, id: string): Promise<ListResponse<Message>> {
    const canonical = id.startsWith(`${type}:`) ? id.slice(type.length + 1) : id;
    const items = flowSeed().flatMap((flow) => messagesFor(flow.id)).filter((message) => (message.content || "").includes(canonical) || message.correlation_id === canonical);
    return Promise.resolve(list(items));
  },
  listFlowMessages(id: string): Promise<ListResponse<Message>> {
    return Promise.resolve(list(messagesFor(id)));
  },
  listFlowTasks(id: string): Promise<ListResponse<Task>> {
    return Promise.resolve(list(tasksFor(id)));
  },
  listFlowTraces(id: string): Promise<ListResponse<Trace>> {
    return Promise.resolve(list(tracesFor(id)));
  },
  listFlows(params = {}): Promise<ListResponse<Flow>> {
    return Promise.resolve(list(applyFlowFilter(flowSeed(), params)));
  },
  listMapFeatures(params: { bbox: string; types?: string; window?: string }): Promise<FeatureCollection> {
    return Promise.resolve(mapFeatures(params));
  },
  listMapLayers(): Promise<ListResponse<MapLayer>> {
    return Promise.resolve(list(PACKS.flatMap((pack) => pack.map_layers ?? [])));
  },
  listReplays(limit = 20): Promise<ListResponse<ReplaySession>> {
    return Promise.resolve(
      list(
        [
          {
            id: "demo-replay-baseline",
            group_id: "kafsiem-ontology-demo-replay",
            status: "finished",
            started_at: iso(120),
            finished_at: iso(116),
            message_count: 84,
            topics: TOPICS.slice(0, 3),
          },
        ].slice(0, limit),
      ),
    );
  },
  listTopicHealth(): Promise<ListResponse<TopicHealth>> {
    return Promise.resolve(list(topicHealth()));
  },
  searchEntities(q: string): Promise<SearchResponse> {
    return Promise.resolve(search(q));
  },
};

function topicHealth(): TopicHealth[] {
  const t = tick() % 10;
  return [
    { topic: "group.drones.requests", messages_per_hour: 42 + t, message_density: "high", active_agents: 4, is_stale: false, last_message_at: iso(1) },
    { topic: "group.drones.traces", messages_per_hour: 118 + t * 3, message_density: "high", active_agents: 5, is_stale: false, last_message_at: iso(0) },
    { topic: "group.scada.tasks.status", messages_per_hour: 24 + t, message_density: "medium", active_agents: 3, is_stale: false, last_message_at: iso(4) },
    { topic: "group.scada.memory.context", messages_per_hour: 5, message_density: "low", active_agents: 2, is_stale: t > 6, last_message_at: iso(31) },
  ];
}
