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
import { currentDemoScenario, type AgentOpsDemoScenario } from "@/agentops/lib/demo";

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

const DRONE_TOPICS = [
  "ops.drones.command.v1",
  "ops.drones.telemetry.v1",
  "ops.drones.detections.v1",
  "ops.drones.ontology.edges.v1",
];

const SCADA_TOPICS = [
  "ot.scada.modbus.readings.v1",
  "ot.scada.alarm.events.v1",
  "ot.scada.change.audit.v1",
  "ot.scada.ontology.edges.v1",
];

function topicsForScenario(scenario = currentDemoScenario()): string[] {
  if (scenario === "drones") return DRONE_TOPICS;
  if (scenario === "scada") return SCADA_TOPICS;
  return [...DRONE_TOPICS, ...SCADA_TOPICS];
}

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

function scenarioMatchesFlow(flow: Flow, scenario: AgentOpsDemoScenario): boolean {
  if (scenario === "all") return true;
  return flow.topics.some((topic) => topic.includes(`.${scenario}.`));
}

function flowSeed(): Flow[] {
  const t = tick() % 12;
  const flows: Flow[] = [
    {
      id: "corr-drone-ew-1",
      topic_count: 4,
      sender_count: 5,
      topics: DRONE_TOPICS,
      senders: ["mission-control", "drone-auv-07", "drone-uav-11", "ew-sentinel", "route-planner"],
      trace_ids: ["trace-drone-swarm-readiness"],
      task_ids: ["task-drone-readiness", "task-ew-route", "task-payload-dropout"],
      first_seen: iso(56),
      last_seen: iso(1),
      latest_status: "triage_required",
      message_count: 37 + t,
      latest_preview: "Drone swarm telemetry joined into mission, EW, and payload ontology",
    },
    {
      id: "corr-scada-purdue-1",
      topic_count: 4,
      sender_count: 6,
      topics: SCADA_TOPICS,
      senders: ["plc-12", "hmi-intake-02", "historian-04", "change-gate", "detector-purdue", "firmware-checker"],
      trace_ids: ["trace-scada-purdue-violation"],
      task_ids: ["task-purdue-isolation", "task-change-review", "task-firmware-exception"],
      first_seen: iso(39),
      last_seen: iso(4),
      latest_status: "blocked",
      message_count: 29 + (t % 4),
      latest_preview: "PLC-12 Purdue violation with RTU firmware drift branch",
    },
  ];
  return flows.filter((flow) => scenarioMatchesFlow(flow, currentDemoScenario()));
}

const PACKS: Pack[] = [
  {
    name: "drones",
    version: "0.1.0",
    description: "Unmanned systems Kafka traffic mapped into mission, platform, sortie, payload, and EW ontology",
    entity_types: ["platform", "payload", "area", "location", "mission", "sortie", "subsystem", "ew_event", "finding"],
    edge_types: ["assigned_to", "participates_in", "located_in", "emits", "observed_at", "affects", "mitigates", "depends_on"],
    detectors: [
      {
        id: "drones.telemetry_gap",
        severity: "high",
        window: "15m",
        query: "active platform with heartbeat loss inside sortie window",
        source: "demo",
      },
      {
        id: "drones.ew_overlap",
        severity: "medium",
        window: "30m",
        query: "drone track intersects EW event area and RF link confidence falls",
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
          { id: "callsign", label: "Callsign" },
          { id: "battery", label: "Battery" },
          { id: "link_quality", label: "Link" },
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
    description: "Critical infrastructure Kafka traffic mapped into plant, process, Purdue zone, device, tag, change, and vulnerability ontology",
    entity_types: ["plant", "process", "zone", "device", "tag", "change", "tradecraft", "vulnerability"],
    edge_types: ["controls", "depends_on", "has_vulnerability", "located_in", "writes_to", "changed_by", "matches_tradecraft"],
    detectors: [
      {
        id: "scada.unapproved_firmware",
        severity: "high",
        window: "1h",
        query: "device firmware mismatch with open change ticket",
        source: "demo",
      },
      {
        id: "scada.purdue_violation",
        severity: "critical",
        window: "5m",
        query: "write request crosses Purdue L3 to L1 without approved jump host",
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
          { id: "purdue_level", label: "Purdue" },
          { id: "vendor", label: "Vendor" },
          { id: "risk", label: "Risk" },
        ],
      },
    ],
  },
];

function packsForScenario(scenario = currentDemoScenario()): Pack[] {
  if (scenario === "all") return PACKS;
  return PACKS.filter((pack) => pack.name === scenario);
}

const ENTITIES: Entity[] = [
  {
    id: "correlation:corr-drone-ew-1",
    type: "correlation",
    canonical_id: "corr-drone-ew-1",
    display_name: "Drone flight EW correlation",
    first_seen: iso(56),
    last_seen: iso(1),
    attrs: { status: "triage_required", priority: "high", scenario: "drone-flight-ontology", operator: "mission-control" },
  },
  {
    id: "correlation:corr-scada-purdue-1",
    type: "correlation",
    canonical_id: "corr-scada-purdue-1",
    display_name: "PLC-12 Purdue violation",
    first_seen: iso(39),
    last_seen: iso(4),
    attrs: { status: "blocked", priority: "critical", scenario: "purdue-zone-enforcement", operator: "change-gate" },
  },
  {
    id: "platform:auv-07",
    type: "platform",
    canonical_id: "auv-07",
    display_name: "AUV-07",
    first_seen: iso(140),
    last_seen: iso(1),
    attrs: { readiness: "degraded", callsign: "TRITON-7", battery: "62%", link_quality: "38%", autonomy_version: "7.4.2" },
  },
  {
    id: "platform:uav-11",
    type: "platform",
    canonical_id: "uav-11",
    display_name: "UAV-11",
    first_seen: iso(138),
    last_seen: iso(1),
    attrs: { readiness: "green", callsign: "KITE-11", battery: "74%", link_quality: "88%", autonomy_version: "7.4.2" },
  },
  {
    id: "platform:uav-19",
    type: "platform",
    canonical_id: "uav-19",
    display_name: "UAV-19",
    first_seen: iso(116),
    last_seen: iso(3),
    attrs: { readiness: "amber", callsign: "KITE-19", battery: "69%", link_quality: "71%", autonomy_version: "7.4.1" },
  },
  {
    id: "mission:harbor-isr",
    type: "mission",
    canonical_id: "harbor-isr",
    display_name: "Harbor ISR Mission",
    first_seen: iso(180),
    last_seen: iso(1),
    attrs: { objective: "perimeter surveillance", commander: "mission-control", status: "hold" },
  },
  {
    id: "sortie:sortie-441",
    type: "sortie",
    canonical_id: "sortie-441",
    display_name: "Sortie 441",
    first_seen: iso(62),
    last_seen: iso(1),
    attrs: { window: "2026-05-01T08:00Z/2026-05-01T12:00Z", status: "blocked", launch_gate: "telemetry" },
  },
  {
    id: "area:mtarfa-ridge",
    type: "area",
    canonical_id: "mtarfa-ridge",
    display_name: "Mtarfa Ridge EW Box",
    first_seen: iso(190),
    last_seen: iso(1),
    attrs: { country_code: "MT", sector: "defense", signal: "L-band interference", confidence: "0.81" },
  },
  {
    id: "location:mtarfa-telemetry",
    type: "location",
    canonical_id: "mtarfa-telemetry",
    display_name: "Telemetry Station M-4",
    first_seen: iso(200),
    last_seen: iso(2),
    attrs: { country_code: "MT", lat: 35.895, lon: 14.405 },
  },
  {
    id: "subsystem:rf-link-auv-07",
    type: "subsystem",
    canonical_id: "rf-link-auv-07",
    display_name: "AUV-07 RF Link",
    first_seen: iso(140),
    last_seen: iso(1),
    attrs: { kind: "rf-link", health: "degraded", packet_loss: "21%" },
  },
  {
    id: "payload:eo-uav-19",
    type: "payload",
    canonical_id: "eo-uav-19",
    display_name: "UAV-19 EO Payload",
    first_seen: iso(116),
    last_seen: iso(3),
    attrs: { kind: "electro-optical", health: "intermittent", dropouts: 4 },
  },
  {
    id: "ew_event:ew-mtarfa-042",
    type: "ew_event",
    canonical_id: "ew-mtarfa-042",
    display_name: "EW Event Mtarfa 042",
    first_seen: iso(24),
    last_seen: iso(1),
    attrs: { band: "L-band", confidence: "0.81", source: "ew-sentinel" },
  },
  {
    id: "finding:ew-interference",
    type: "finding",
    canonical_id: "ew-interference",
    display_name: "EW interference hypothesis",
    first_seen: iso(52),
    last_seen: iso(1),
    attrs: { severity: "high", confidence: "0.78", category: "mission_risk" },
  },
  {
    id: "plant:desal-east",
    type: "plant",
    canonical_id: "desal-east",
    display_name: "East Desalination Plant",
    first_seen: iso(400),
    last_seen: iso(4),
    attrs: { sector: "water", country_code: "MT", criticality: "high" },
  },
  {
    id: "process:intake-pump-train",
    type: "process",
    canonical_id: "intake-pump-train",
    display_name: "Intake Pump Train",
    first_seen: iso(390),
    last_seen: iso(4),
    attrs: { process_area: "intake", state: "running", safety_impact: "high" },
  },
  {
    id: "zone:purdue-l1",
    type: "zone",
    canonical_id: "purdue-l1",
    display_name: "Purdue Level 1",
    first_seen: iso(400),
    last_seen: iso(4),
    attrs: { level: "L1", role: "basic control" },
  },
  {
    id: "zone:purdue-l3",
    type: "zone",
    canonical_id: "purdue-l3",
    display_name: "Purdue Level 3",
    first_seen: iso(400),
    last_seen: iso(4),
    attrs: { level: "L3", role: "operations management" },
  },
  {
    id: "device:plc-12",
    type: "device",
    canonical_id: "plc-12",
    display_name: "PLC-12",
    first_seen: iso(340),
    last_seen: iso(4),
    attrs: { firmware: "3.8.9", purdue_level: "L1", vendor: "Siemens", risk: "critical" },
  },
  {
    id: "device:rtu-12",
    type: "device",
    canonical_id: "rtu-12",
    display_name: "RTU-12",
    first_seen: iso(300),
    last_seen: iso(4),
    attrs: { firmware: "4.2.1", purdue_level: "L2", vendor: "Schneider Electric", risk: "high", cve: "CVE-2026-12345" },
  },
  {
    id: "device:hmi-intake-02",
    type: "device",
    canonical_id: "hmi-intake-02",
    display_name: "HMI Intake 02",
    first_seen: iso(300),
    last_seen: iso(4),
    attrs: { firmware: "2.14.0", purdue_level: "L3", vendor: "Inductive Automation", risk: "medium" },
  },
  {
    id: "tag:FIT-201",
    type: "tag",
    canonical_id: "FIT-201",
    display_name: "FIT-201 Flow Rate",
    first_seen: iso(260),
    last_seen: iso(4),
    attrs: { engineering_unit: "m3/h", current_value: "1420", alarm_state: "high-high" },
  },
  {
    id: "change:chg-7731",
    type: "change",
    canonical_id: "chg-7731",
    display_name: "Change CHG-7731",
    first_seen: iso(40),
    last_seen: iso(4),
    attrs: { approval: "missing", engineer: "contractor-a", work_order: "none" },
  },
  {
    id: "tradecraft:unauthorized-write",
    type: "tradecraft",
    canonical_id: "unauthorized-write",
    display_name: "Unauthorized PLC Write",
    first_seen: iso(39),
    last_seen: iso(4),
    attrs: { framework: "ATT&CK ICS", tactic: "impair process control", severity: "critical" },
  },
  {
    id: "vulnerability:CVE-2026-12345",
    type: "vulnerability",
    canonical_id: "CVE-2026-12345",
    display_name: "CVE-2026-12345",
    first_seen: iso(320),
    last_seen: iso(4),
    attrs: { vendor: "Schneider Electric", product: "RTU firmware 4.2.x", severity: "high" },
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

function demoEdges(): Edge[] {
  return [
    edge("correlation:corr-drone-ew-1", "mission:harbor-isr", "observed_subject", 0.95),
    edge("mission:harbor-isr", "sortie:sortie-441", "has_sortie", 0.92),
    edge("sortie:sortie-441", "platform:auv-07", "assigned_to", 0.96),
    edge("sortie:sortie-441", "platform:uav-11", "assigned_to", 0.88),
    edge("platform:auv-07", "subsystem:rf-link-auv-07", "depends_on", 0.82),
    edge("platform:auv-07", "area:mtarfa-ridge", "observed_at", 0.82),
    edge("area:mtarfa-ridge", "location:mtarfa-telemetry", "covers", 0.71),
    edge("ew_event:ew-mtarfa-042", "area:mtarfa-ridge", "located_in", 0.86),
    edge("ew_event:ew-mtarfa-042", "finding:ew-interference", "produces", 0.79),
    edge("finding:ew-interference", "platform:auv-07", "affects", 0.77),
    edge("correlation:corr-drone-ew-1", "platform:uav-19", "fleet_member", 0.9),
    edge("platform:uav-19", "payload:eo-uav-19", "emits", 0.74),
    edge("platform:uav-19", "sortie:sortie-441", "participates_in", 0.72),
    edge("payload:eo-uav-19", "finding:ew-interference", "affected_by", 0.51),
    edge("correlation:corr-scada-purdue-1", "plant:desal-east", "observed_subject", 0.9),
    edge("plant:desal-east", "process:intake-pump-train", "hosts", 0.9),
    edge("process:intake-pump-train", "device:plc-12", "controls", 0.84),
    edge("device:hmi-intake-02", "device:plc-12", "writes_to", 0.93),
    edge("device:hmi-intake-02", "zone:purdue-l3", "located_in", 0.72),
    edge("device:plc-12", "zone:purdue-l1", "located_in", 0.8),
    edge("device:plc-12", "tag:FIT-201", "writes_to", 0.87),
    edge("change:chg-7731", "device:plc-12", "changed_by", 0.69),
    edge("tradecraft:unauthorized-write", "device:plc-12", "matches_tradecraft", 0.67),
    edge("correlation:corr-scada-purdue-1", "device:rtu-12", "adjacent_asset", 0.91),
    edge("plant:desal-east", "device:rtu-12", "controls", 0.8),
    edge("device:rtu-12", "vulnerability:CVE-2026-12345", "has_vulnerability", 0.69),
  ];
}

function activeDemoEdges(): Edge[] {
  return demoEdges();
}

function messagesFor(flowId: string): Message[] {
  const t = tick() % 50;
  const dynamicOffset = 1200 + t;
  const data: Record<string, Message[]> = {
    "corr-drone-ew-1": [
      {
        id: "msg-drone-command-1",
        topic: "ops.drones.command.v1",
        topic_family: "command",
        partition: 0,
        offset: 1188,
        timestamp: iso(54),
        envelope_type: "mission_command",
        sender_id: "mission-control",
        correlation_id: flowId,
        trace_id: "trace-drone-swarm-readiness",
        task_id: "task-drone-readiness",
        status: "requested",
        preview: "Launch gate requested for three-drone harbor ISR sortie",
        content: content({ mission: "harbor-isr", sortie: "sortie-441", platform_id: "auv-07", platform: "uav-11", area: "mtarfa-ridge", command_key: "sortie-441:launch-gate" }),
      },
      {
        id: "msg-drone-telemetry-1",
        topic: "ops.drones.telemetry.v1",
        topic_family: "telemetry",
        partition: 0,
        offset: 1192,
        timestamp: iso(18),
        envelope_type: "telemetry",
        sender_id: "drone-auv-07",
        correlation_id: flowId,
        trace_id: "trace-drone-swarm-readiness",
        task_id: "task-ew-route",
        status: "triage_required",
        preview: "AUV-07 heartbeat loss and RF packet loss crossed detector threshold",
        content: content({ platform: "auv-07", subsystem: "rf-link-auv-07", location: "mtarfa-telemetry", battery: 62, link_quality: 38, packet_loss_pct: 21 }),
      },
      {
        id: "msg-drone-ew-1",
        topic: "ops.drones.detections.v1",
        topic_family: "detections",
        partition: 1,
        offset: 1197,
        timestamp: iso(6),
        envelope_type: "detector_hit",
        sender_id: "ew-sentinel",
        correlation_id: flowId,
        trace_id: "trace-drone-swarm-readiness",
        task_id: "task-ew-route",
        status: "triage_required",
        preview: "EW event overlaps sortie route and degraded RF link",
        content: content({ ew_event: "ew-mtarfa-042", finding: "ew-interference", area: "mtarfa-ridge", platform: "auv-07", confidence: 0.81 }),
      },
      {
        id: `msg-drone-ontology-${t}`,
        topic: "ops.drones.ontology.edges.v1",
        topic_family: "ontology",
        partition: 1,
        offset: dynamicOffset,
        timestamp: iso(1),
        envelope_type: "ontology_edge_batch",
        sender_id: "route-planner",
        correlation_id: flowId,
        trace_id: "trace-drone-swarm-readiness",
        task_id: "task-ew-route",
        status: "streaming",
        preview: `Kafka edge batch ${t}: platform, sortie, EW event, and mitigation route linked`,
        content: content({ platform: "auv-07", mission: "harbor-isr", sortie: "sortie-441", ew_event: "ew-mtarfa-042", edge_count: 8, detector: "drones.ew_overlap" }),
      },
      {
        id: "msg-drone-payload-1",
        topic: "ops.drones.telemetry.v1",
        topic_family: "telemetry",
        partition: 2,
        offset: 2102,
        timestamp: iso(31),
        envelope_type: "telemetry",
        sender_id: "drone-uav-19",
        correlation_id: flowId,
        trace_id: "trace-drone-swarm-readiness",
        task_id: "task-payload-dropout",
        status: "active",
        preview: "UAV-19 payload health stream reports repeated EO dropouts",
        content: content({ platform: "uav-19", payload: "eo-uav-19", mission: "harbor-isr", sortie: "sortie-441", dropouts: 4 }),
      },
      {
        id: "msg-drone-payload-ontology-1",
        topic: "ops.drones.ontology.edges.v1",
        topic_family: "ontology",
        partition: 2,
        offset: 2110,
        timestamp: iso(3),
        envelope_type: "ontology_edge_batch",
        sender_id: "payload-classifier",
        correlation_id: flowId,
        trace_id: "trace-drone-swarm-readiness",
        task_id: "task-payload-dropout",
        status: "active",
        preview: "Payload dropout linked to the same sortie ontology",
        content: content({ platform: "uav-19", payload: "eo-uav-19", mission: "harbor-isr", finding: "ew-interference", edge_count: 5 }),
      },
    ],
    "corr-scada-purdue-1": [
      {
        id: "msg-scada-modbus-1",
        topic: "ot.scada.modbus.readings.v1",
        topic_family: "telemetry",
        partition: 0,
        offset: 530,
        timestamp: iso(38),
        envelope_type: "modbus_write",
        sender_id: "hmi-intake-02",
        correlation_id: flowId,
        trace_id: "trace-scada-purdue-violation",
        task_id: "task-purdue-isolation",
        status: "blocked",
        preview: "HMI Intake 02 issued write request to PLC-12 flow tag",
        content: content({ plant: "desal-east", process: "intake-pump-train", device: "plc-12", tag: "FIT-201", zone: "purdue-l1", source_zone: "purdue-l3", function_code: 16 }),
      },
      {
        id: "msg-scada-alarm-1",
        topic: "ot.scada.alarm.events.v1",
        topic_family: "alarms",
        partition: 1,
        offset: 544,
        timestamp: iso(8),
        envelope_type: "alarm_event",
        sender_id: "plc-12",
        correlation_id: flowId,
        trace_id: "trace-scada-purdue-violation",
        task_id: "task-purdue-isolation",
        status: "blocked",
        preview: "FIT-201 high-high alarm followed unapproved write path",
        content: content({ plant: "desal-east", process: "intake-pump-train", device: "plc-12", tag: "FIT-201", alarm_state: "high-high" }),
      },
      {
        id: "msg-scada-change-1",
        topic: "ot.scada.change.audit.v1",
        topic_family: "changes",
        partition: 0,
        offset: 558,
        timestamp: iso(5),
        envelope_type: "change_audit",
        sender_id: "change-gate",
        correlation_id: flowId,
        trace_id: "trace-scada-purdue-violation",
        task_id: "task-change-review",
        status: "missing_approval",
        preview: "No approved work order found for HMI-to-PLC write path",
        content: content({ change: "chg-7731", device: "plc-12", tag: "FIT-201", engineer: "contractor-a", approval: "missing" }),
      },
      {
        id: `msg-scada-ontology-${t}`,
        topic: "ot.scada.ontology.edges.v1",
        topic_family: "ontology",
        partition: 2,
        offset: 580 + t,
        timestamp: iso(4),
        envelope_type: "ontology_edge_batch",
        sender_id: "detector-purdue",
        correlation_id: flowId,
        trace_id: "trace-scada-purdue-violation",
        task_id: "task-purdue-isolation",
        status: "blocked",
        preview: `Kafka edge batch ${t}: plant, process, zone, device, tag, and change linked`,
        content: content({ plant: "desal-east", process: "intake-pump-train", zone: "purdue-l1", device: "plc-12", tag: "FIT-201", change: "chg-7731", tradecraft: "unauthorized-write", detector: "scada.purdue_violation" }),
      },
      {
        id: "msg-scada-fw-1",
        topic: "ot.scada.change.audit.v1",
        topic_family: "changes",
        partition: 0,
        offset: 730,
        timestamp: iso(70),
        envelope_type: "asset_inventory",
        sender_id: "asset-inventory",
        correlation_id: flowId,
        trace_id: "trace-scada-purdue-violation",
        task_id: "task-firmware-exception",
        status: "active",
        preview: "RTU-12 firmware drift joined the same plant ontology",
        content: content({ plant: "desal-east", device: "rtu-12", vulnerability: "CVE-2026-12345", vendor: "Schneider Electric", firmware: "4.2.1" }),
      },
      {
        id: "msg-scada-fw-ontology-1",
        topic: "ot.scada.ontology.edges.v1",
        topic_family: "ontology",
        partition: 0,
        offset: 744,
        timestamp: iso(27),
        envelope_type: "ontology_edge_batch",
        sender_id: "firmware-checker",
        correlation_id: flowId,
        trace_id: "trace-scada-purdue-violation",
        task_id: "task-firmware-exception",
        status: "active",
        preview: "RTU-12 vulnerability linked into the same plant graph",
        content: content({ plant: "desal-east", device: "rtu-12", vulnerability: "CVE-2026-12345", edge_count: 4, detector: "scada.unapproved_firmware" }),
      },
    ],
  };
  return data[flowId] ?? [];
}

function tasksFor(flowId: string): Task[] {
  if (flowId === "corr-drone-ew-1") {
    return [
      {
        id: "task-drone-readiness",
        requester_id: "mission-control",
        responder_id: "safety-officer",
        status: "blocked",
        description: "Gate sortie launch readiness against platform telemetry and EW context",
        last_summary: "AUV-07 RF link degraded; route mitigation is required before launch.",
        first_seen: iso(54),
        last_seen: iso(18),
      },
      {
        id: "task-ew-route",
        parent_task_id: "task-drone-readiness",
        delegation_depth: 1,
        requester_id: "safety-officer",
        responder_id: "route-planner",
        status: "triage_required",
        description: "Compute mitigation route around EW event area",
        last_summary: "Ontology edge stream is joining sortie, platform, area, and EW event.",
        first_seen: iso(19),
        last_seen: iso(1),
      },
      {
        id: "task-payload-dropout",
        requester_id: "mission-control",
        responder_id: "payload-classifier",
        status: "active",
        description: "Correlate EO payload dropouts across the same sortie ontology",
        last_summary: "UAV-19 payload branch added to sortie 441.",
        first_seen: iso(31),
        last_seen: iso(3),
      },
    ];
  }
  if (flowId === "corr-scada-purdue-1") {
    return [
      {
        id: "task-purdue-isolation",
        requester_id: "detector-purdue",
        responder_id: "change-gate",
        status: "blocked",
        description: "Isolate HMI-to-PLC write path crossing Purdue L3 to L1",
        last_summary: "Write path blocked pending approved work order.",
        first_seen: iso(38),
        last_seen: iso(4),
      },
      {
        id: "task-change-review",
        parent_task_id: "task-purdue-isolation",
        delegation_depth: 1,
        requester_id: "change-gate",
        responder_id: "asset-inventory",
        status: "missing_approval",
        description: "Find approved change for PLC-12 tag write",
        last_summary: "No work order found for CHG-7731.",
        first_seen: iso(8),
        last_seen: iso(4),
      },
      {
        id: "task-firmware-exception",
        requester_id: "asset-inventory",
        responder_id: "firmware-checker",
        status: "active",
        description: "Check RTU-12 firmware drift as a branch of the same plant incident",
        last_summary: "RTU-12 vulnerability added as adjacent asset evidence.",
        first_seen: iso(70),
        last_seen: iso(27),
      },
    ];
  }
  return [];
}

function tracesFor(flowId: string): Trace[] {
  if (flowId === "corr-drone-ew-1") {
    return [
      {
        id: "trace-drone-swarm-readiness",
        span_count: 9 + (tick() % 4),
        agents: ["mission-control", "drone-auv-07", "ew-sentinel", "route-planner", "drone-uav-19", "payload-classifier"],
        span_types: ["mission_command", "telemetry", "detector_hit", "ontology_edge_batch", "payload_health"],
        latest_title: "Drone sortie ontology with payload branch",
        started_at: iso(54),
        ended_at: iso(1),
        duration_ms: 3_180_000,
      },
    ];
  }
  if (flowId === "corr-scada-purdue-1") {
    return [
      {
        id: "trace-scada-purdue-violation",
        span_count: 10 + (tick() % 3),
        agents: ["hmi-intake-02", "plc-12", "change-gate", "detector-purdue", "asset-inventory", "firmware-checker"],
        span_types: ["modbus_write", "alarm_event", "change_audit", "ontology_edge_batch", "asset_inventory"],
        latest_title: "SCADA plant ontology with RTU drift branch",
        started_at: iso(38),
        ended_at: iso(4),
        duration_ms: 2_040_000,
      },
    ];
  }
  return [];
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
  const allEdges = activeDemoEdges();
  if (key === "correlation:corr-drone-ew-1") {
    const ids = ["correlation:corr-drone-ew-1", "mission:harbor-isr", "sortie:sortie-441", "platform:auv-07", "platform:uav-11", "subsystem:rf-link-auv-07", "area:mtarfa-ridge", "location:mtarfa-telemetry", "ew_event:ew-mtarfa-042", "finding:ew-interference", "platform:uav-19", "payload:eo-uav-19"];
    return {
      entities: ENTITIES.filter((item) => ids.includes(item.id)),
      edges: allEdges.filter((item) => ids.includes(item.src_id) && ids.includes(item.dst_id)),
    };
  }
  if (key === "correlation:corr-scada-purdue-1") {
    const ids = ["correlation:corr-scada-purdue-1", "plant:desal-east", "process:intake-pump-train", "zone:purdue-l1", "zone:purdue-l3", "device:plc-12", "device:hmi-intake-02", "tag:FIT-201", "change:chg-7731", "tradecraft:unauthorized-write", "device:rtu-12", "vulnerability:CVE-2026-12345"];
    return {
      entities: ENTITIES.filter((item) => ids.includes(item.id)),
      edges: allEdges.filter((item) => ids.includes(item.src_id) && ids.includes(item.dst_id)),
    };
  }
  const center = ENTITIES.find((item) => item.id === key || (item.type === type && item.canonical_id === id));
  if (!center) return { entities: [], edges: [] };
  const edges = allEdges.filter((item) => item.src_id === center.id || item.dst_id === center.id);
  const ids = new Set([center.id, ...edges.flatMap((item) => [item.src_id, item.dst_id])]);
  return {
    entities: ENTITIES.filter((item) => ids.has(item.id)),
    edges,
  };
}

function provenanceFor(type: string, id: string): Provenance[] {
  const subject = id.startsWith(`${type}:`) ? id : `${type}:${id}`;
  const pack = ["plant", "process", "zone", "device", "tag", "change", "tradecraft", "vulnerability"].includes(type) ? "scada" : "drones";
  const ingestTopic = pack === "scada" ? "ot.scada.modbus.readings.v1" : "ops.drones.telemetry.v1";
  const ontologyTopic = pack === "scada" ? "ot.scada.ontology.edges.v1" : "ops.drones.ontology.edges.v1";
  return [
    {
      subject_kind: "entity",
      subject_id: subject,
      stage: "ingest",
      policy_ver: "demo-stream-v1",
      inputs: { topic: ingestTopic, offset: 1188 },
      decision: "accepted",
      reasons: ["typed-envelope", "pack-field-match"],
      produced_at: iso(54),
    },
    {
      subject_kind: "entity",
      subject_id: subject,
      stage: "ontology",
      policy_ver: "demo-stream-v1",
      inputs: { topic: ontologyTopic, pack, entity_type: type },
      decision: "linked",
      reasons: ["canonical-id", "correlation-window"],
      produced_at: iso(18),
    },
    {
      subject_kind: "entity",
      subject_id: subject,
      stage: "detector",
      policy_ver: "demo-stream-v1",
      inputs: { detector: pack === "scada" ? "scada.purdue_violation" : "drones.ew_overlap" },
      decision: "scored",
      reasons: pack === "scada" ? ["purdue-zone-crossing", "missing-change"] : ["route-overlap", "telemetry-gap"],
      produced_at: iso(2),
    },
  ];
}

function mapFeatures(params: { types?: string }): FeatureCollection {
  const selected = new Set((params.types || "").split(",").map((item) => item.trim()).filter(Boolean));
  const scenario = currentDemoScenario();
  const features: FeatureCollection["features"] = [
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.405, 35.895] },
      properties: { id: "platform:auv-07", type: "platform", display_name: "AUV-07" },
    },
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.42, 35.902] },
      properties: { id: "platform:uav-11", type: "platform", display_name: "UAV-11" },
    },
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.455, 35.887] },
      properties: { id: "platform:uav-19", type: "platform", display_name: "UAV-19" },
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
      properties: { id: "device:plc-12", type: "device", display_name: "PLC-12" },
    },
    {
      type: "Feature",
      geometry: { type: "Point", coordinates: [14.528, 35.879] },
      properties: { id: "device:rtu-12", type: "device", display_name: "RTU-12" },
    },
  ];
  const scenarioFeatures = features.filter((item) => {
    if (scenario === "all") return true;
    const type = String(item.properties.type);
    return scenario === "drones" ? ["platform", "area", "location"].includes(type) : ["plant", "device"].includes(type);
  });
  return {
    type: "FeatureCollection",
    features: selected.size === 0 ? scenarioFeatures : scenarioFeatures.filter((item) => selected.has(String(item.properties.type))),
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
  const detectorHits = packsForScenario().flatMap((pack) => pack.detectors ?? [])
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
      effective_topics: topicsForScenario(),
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
    return Promise.resolve(list(packsForScenario()));
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
    return Promise.resolve(list(packsForScenario().flatMap((pack) => pack.map_layers ?? [])));
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
            topics: topicsForScenario().slice(0, 3),
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
  const rows: TopicHealth[] = [
    { topic: "ops.drones.command.v1", messages_per_hour: 42 + t, message_density: "high", active_agents: 3, is_stale: false, last_message_at: iso(1) },
    { topic: "ops.drones.telemetry.v1", messages_per_hour: 420 + t * 11, message_density: "high", active_agents: 6, is_stale: false, last_message_at: iso(0) },
    { topic: "ops.drones.ontology.edges.v1", messages_per_hour: 118 + t * 3, message_density: "high", active_agents: 4, is_stale: false, last_message_at: iso(0) },
    { topic: "ot.scada.modbus.readings.v1", messages_per_hour: 820 + t * 9, message_density: "high", active_agents: 5, is_stale: false, last_message_at: iso(1) },
    { topic: "ot.scada.alarm.events.v1", messages_per_hour: 24 + t, message_density: "medium", active_agents: 3, is_stale: false, last_message_at: iso(4) },
    { topic: "ot.scada.ontology.edges.v1", messages_per_hour: 64 + t, message_density: "medium", active_agents: 3, is_stale: false, last_message_at: iso(4) },
  ];
  const allowed = new Set(topicsForScenario());
  return rows.filter((row) => allowed.has(row.topic));
}
