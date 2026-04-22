package specgen

type Output struct {
	Path    string
	Content string
}

func Outputs() []Output {
	return []Output{
		{Path: "api/openapi.yaml", Content: openapiYAML},
		{Path: "src/agentops/lib/api-client/types.ts", Content: typesTS},
		{Path: "src/agentops/lib/api-client/client.ts", Content: clientTS},
		{Path: "src/agentops/lib/api-client/index.ts", Content: indexTS},
		{Path: "src/agentops/types/index.ts", Content: typesIndexTS},
	}
}

const openapiYAML = `openapi: 3.1.0
info:
  title: kafSIEM Analyst API
  version: 0.1.0
servers:
  - url: /api/v1
paths:
  /entities/{type}/{id}:
    get:
      operationId: getEntity
      parameters:
        - in: path
          name: type
          required: true
          schema: { type: string }
        - in: path
          name: id
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Entity profile
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Profile' }
        default:
          $ref: '#/components/responses/Problem'
  /entities/{type}/{id}/neighborhood:
    get:
      operationId: getEntityNeighborhood
      parameters:
        - in: path
          name: type
          required: true
          schema: { type: string }
        - in: path
          name: id
          required: true
          schema: { type: string }
        - in: query
          name: depth
          schema: { type: integer, minimum: 1, maximum: 3, default: 2 }
        - in: query
          name: types
          schema: { type: string }
        - in: query
          name: window
          schema: { type: string }
      responses:
        '200':
          description: Entity neighborhood
          content:
            application/json:
              schema:
                type: object
                required: [entities, edges]
                properties:
                  entities:
                    type: array
                    items: { $ref: '#/components/schemas/Entity' }
                  edges:
                    type: array
                    items: { $ref: '#/components/schemas/Edge' }
        default:
          $ref: '#/components/responses/Problem'
  /entities/{type}/{id}/provenance:
    get:
      operationId: listEntityProvenance
      parameters:
        - in: path
          name: type
          required: true
          schema: { type: string }
        - in: path
          name: id
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Provenance chain
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ListProvenanceResponse'
        default:
          $ref: '#/components/responses/Problem'
  /entities/{type}/{id}/geometry:
    get:
      operationId: getEntityGeometry
      parameters:
        - in: path
          name: type
          required: true
          schema: { type: string }
        - in: path
          name: id
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Geometry
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Geometry' }
        default:
          $ref: '#/components/responses/Problem'
  /entities/{type}/{id}/timeline:
    get:
      operationId: listEntityTimeline
      parameters:
        - in: path
          name: type
          required: true
          schema: { type: string }
        - in: path
          name: id
          required: true
          schema: { type: string }
        - in: query
          name: after
          schema: { type: string }
        - in: query
          name: limit
          schema: { type: integer, default: 50 }
      responses:
        '200':
          description: Timeline
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListMessageResponse' }
        default:
          $ref: '#/components/responses/Problem'
  /graph/path:
    get:
      operationId: getGraphPath
      parameters:
        - in: query
          name: src
          required: true
          schema: { type: string }
        - in: query
          name: dst
          required: true
          schema: { type: string }
        - in: query
          name: max
          schema: { type: integer, minimum: 1, maximum: 3, default: 3 }
      responses:
        '200':
          description: Path result
          content:
            application/json:
              schema:
                type: object
                required: [found, edges]
                properties:
                  found: { type: boolean }
                  edges:
                    type: array
                    items: { $ref: '#/components/schemas/Edge' }
        default:
          $ref: '#/components/responses/Problem'
  /map/features:
    get:
      operationId: listMapFeatures
      parameters:
        - in: query
          name: bbox
          required: true
          schema: { type: string, example: '14.40,35.80,14.60,36.00' }
        - in: query
          name: types
          schema: { type: string }
        - in: query
          name: window
          schema: { type: string }
      responses:
        '200':
          description: GeoJSON feature collection
          content:
            application/json:
              schema: { $ref: '#/components/schemas/FeatureCollection' }
        default:
          $ref: '#/components/responses/Problem'
  /map/layers:
    get:
      operationId: listMapLayers
      responses:
        '200':
          description: Map layers
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListMapLayerResponse' }
  /flows:
    get:
      operationId: listFlows
      parameters:
        - in: query
          name: after
          schema: { type: string }
        - in: query
          name: limit
          schema: { type: integer, default: 50 }
        - in: query
          name: topic
          schema: { type: string }
        - in: query
          name: sender
          schema: { type: string }
        - in: query
          name: status
          schema: { type: string }
        - in: query
          name: q
          schema: { type: string }
      responses:
        '200':
          description: Flows
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListFlowResponse' }
        default:
          $ref: '#/components/responses/Problem'
  /flows/{id}:
    get:
      operationId: getFlow
      parameters:
        - in: path
          name: id
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Flow
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Flow' }
        default:
          $ref: '#/components/responses/Problem'
  /flows/{id}/messages:
    get:
      operationId: listFlowMessages
      parameters:
        - in: path
          name: id
          required: true
          schema: { type: string }
        - in: query
          name: after
          schema: { type: string }
        - in: query
          name: limit
          schema: { type: integer, default: 50 }
      responses:
        '200':
          description: Flow messages
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListMessageResponse' }
        default:
          $ref: '#/components/responses/Problem'
  /flows/{id}/tasks:
    get:
      operationId: listFlowTasks
      parameters:
        - in: path
          name: id
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Flow tasks
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListTaskResponse' }
        default:
          $ref: '#/components/responses/Problem'
  /flows/{id}/traces:
    get:
      operationId: listFlowTraces
      parameters:
        - in: path
          name: id
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Flow traces
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListTraceResponse' }
        default:
          $ref: '#/components/responses/Problem'
  /flows/{id}/timeline:
    get:
      operationId: listFlowTimeline
      parameters:
        - in: path
          name: id
          required: true
          schema: { type: string }
        - in: query
          name: after
          schema: { type: string }
        - in: query
          name: limit
          schema: { type: integer, default: 50 }
      responses:
        '200':
          description: Flow timeline
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListMessageResponse' }
  /topic-health:
    get:
      operationId: listTopicHealth
      responses:
        '200':
          description: Topic health
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListTopicHealthResponse' }
  /health:
    get:
      operationId: getHealth
      responses:
        '200':
          description: Latest health
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Health' }
  /replays:
    get:
      operationId: listReplays
      parameters:
        - in: query
          name: limit
          schema: { type: integer, default: 20 }
      responses:
        '200':
          description: Replay sessions
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListReplayResponse' }
    post:
      operationId: createReplayRequest
      requestBody:
        required: false
        content:
          application/json:
            schema:
              type: object
              properties:
                topics:
                  type: array
                  items: { type: string }
      responses:
        '202':
          description: Replay request accepted
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ReplayRequest' }
        default:
          $ref: '#/components/responses/Problem'
  /search:
    get:
      operationId: searchEntities
      parameters:
        - in: query
          name: q
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                required: [items, next]
                properties:
                  items:
                    type: array
                    items:
                      type: object
                      additionalProperties: true
                  next:
                    type: [string, 'null']
  /ontology/types:
    get:
      operationId: getOntologyTypes
      responses:
        '200':
          description: Active ontology types
          content:
            application/json:
              schema:
                type: object
                required: [entity_types, edge_types]
                properties:
                  entity_types:
                    type: array
                    items: { $ref: '#/components/schemas/TypeSpec' }
                  edge_types:
                    type: array
                    items: { $ref: '#/components/schemas/TypeSpec' }
  /ontology/packs:
    get:
      operationId: getOntologyPacks
      responses:
        '200':
          description: Loaded packs
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ListPackResponse' }
components:
  responses:
    Problem:
      description: RFC 9457 problem details
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
  schemas:
    Problem:
      type: object
      required: [type, title, status]
      properties:
        type: { type: string }
        title: { type: string }
        status: { type: integer }
        detail: { type: string }
        instance: { type: string }
    Entity:
      type: object
      required: [id, type, canonical_id, first_seen, last_seen]
      properties:
        id: { type: string }
        type: { type: string }
        canonical_id: { type: string }
        display_name: { type: string }
        first_seen: { type: string, format: date-time }
        last_seen: { type: string, format: date-time }
        attrs:
          type: object
          additionalProperties: true
    Edge:
      type: object
      required: [src_id, dst_id, type, valid_from, weight]
      properties:
        src_id: { type: string }
        dst_id: { type: string }
        type: { type: string }
        valid_from: { type: string, format: date-time }
        valid_to: { type: string, format: date-time }
        evidence_msg: { type: string }
        weight: { type: number }
        attrs:
          type: object
          additionalProperties: true
    Provenance:
      type: object
      required: [subject_kind, subject_id, stage, produced_at]
      properties:
        subject_kind: { type: string }
        subject_id: { type: string }
        stage: { type: string }
        policy_ver: { type: string }
        inputs:
          type: object
          additionalProperties: true
        decision: { type: string }
        reasons:
          type: array
          items: { type: string }
        produced_at: { type: string, format: date-time }
    Geometry:
      type: object
      required: [entity_id, geometry_type, geojson, srid, min_lat, min_lon, max_lat, max_lon, observed_at]
      properties:
        entity_id: { type: string }
        geometry_type: { type: string }
        geojson:
          description: RFC 7946 geometry
        srid: { type: integer }
        min_lat: { type: number }
        min_lon: { type: number }
        max_lat: { type: number }
        max_lon: { type: number }
        z_min: { type: number }
        z_max: { type: number }
        observed_at: { type: string, format: date-time }
        valid_to: { type: string, format: date-time }
    Neighbor:
      type: object
      required: [entity_id, entity_type, weight]
      properties:
        entity_id: { type: string }
        entity_type: { type: string }
        weight: { type: number }
    Profile:
      type: object
      required: [entity, first_seen, last_seen, edge_counts, top_neighbors]
      properties:
        entity: { $ref: '#/components/schemas/Entity' }
        first_seen: { type: string, format: date-time }
        last_seen: { type: string, format: date-time }
        edge_counts:
          type: object
          additionalProperties: { type: integer }
        top_neighbors:
          type: array
          items: { $ref: '#/components/schemas/Neighbor' }
    Pointer:
      type: object
      required: [bucket, key, size, sha256, path]
      properties:
        bucket: { type: string }
        key: { type: string }
        size: { type: integer }
        sha256: { type: string }
        content_type: { type: string }
        created_at: { type: string, format: date-time }
        proxy_id: { type: string }
        path: { type: string }
    Message:
      type: object
      required: [id, topic, topic_family, partition, offset, timestamp]
      properties:
        id: { type: string }
        topic: { type: string }
        topic_family: { type: string }
        partition: { type: integer }
        offset: { type: integer }
        timestamp: { type: string, format: date-time }
        envelope_type: { type: string }
        sender_id: { type: string }
        correlation_id: { type: string }
        trace_id: { type: string }
        task_id: { type: string }
        parent_task_id: { type: string }
        status: { type: string }
        preview: { type: string }
        content: { type: string }
        lfs: { $ref: '#/components/schemas/Pointer' }
    Flow:
      type: object
      required: [id, topic_count, sender_count, topics, senders, trace_ids, task_ids, first_seen, last_seen, message_count]
      properties:
        id: { type: string }
        topic_count: { type: integer }
        sender_count: { type: integer }
        topics: { type: array, items: { type: string } }
        senders: { type: array, items: { type: string } }
        trace_ids: { type: array, items: { type: string } }
        task_ids: { type: array, items: { type: string } }
        first_seen: { type: string, format: date-time }
        last_seen: { type: string, format: date-time }
        latest_status: { type: string }
        message_count: { type: integer }
        latest_preview: { type: string }
    TopicHealth:
      type: object
      required: [topic, messages_per_hour, message_density, active_agents, is_stale]
      properties:
        topic: { type: string }
        messages_per_hour: { type: number }
        message_density: { type: string }
        active_agents: { type: integer }
        is_stale: { type: boolean }
        last_message_at: { type: string, format: date-time }
    Health:
      type: object
      required: [connected, effective_topics, group_id, accepted_count, rejected_count, mirrored_count, mirror_failed_count, rejected_by_reason, replay_active, replay_last_record_count, topic_health]
      properties:
        connected: { type: boolean }
        effective_topics: { type: array, items: { type: string } }
        group_id: { type: string }
        accepted_count: { type: integer }
        rejected_count: { type: integer }
        mirrored_count: { type: integer }
        mirror_failed_count: { type: integer }
        rejected_by_reason:
          type: object
          additionalProperties: { type: integer }
        last_reject: { type: string }
        last_mirror_error: { type: string }
        last_poll_at: { type: string, format: date-time }
        replay_status: { type: string }
        replay_active: { type: integer }
        replay_last_error: { type: string }
        replay_last_finished_at: { type: string, format: date-time }
        replay_last_record_count: { type: integer }
        topic_health:
          type: array
          items: { $ref: '#/components/schemas/TopicHealth' }
    ReplaySession:
      type: object
      required: [id, group_id, status, started_at, message_count]
      properties:
        id: { type: string }
        group_id: { type: string }
        status: { type: string }
        started_at: { type: string, format: date-time }
        finished_at: { type: string, format: date-time }
        message_count: { type: integer }
        topics: { type: array, items: { type: string } }
        last_error: { type: string }
    ReplayRequest:
      type: object
      required: [id, status, requested_at, topics]
      properties:
        id: { type: string }
        status: { type: string }
        requested_at: { type: string, format: date-time }
        topics: { type: array, items: { type: string } }
    TypeSpec:
      type: object
      required: [name, source]
      properties:
        name: { type: string }
        source: { type: string }
    MapLayer:
      type: object
      required: [id, name, kind, source]
      properties:
        id: { type: string }
        name: { type: string }
        kind: { type: string }
        url: { type: string }
        attribution: { type: string }
        source: { type: string }
    Pack:
      type: object
      required: [name, version]
      properties:
        name: { type: string }
        version: { type: string }
        description: { type: string }
        owner: { type: string }
        entity_types: { type: array, items: { type: string } }
        edge_types: { type: array, items: { type: string } }
        map_layers: { type: array, items: { $ref: '#/components/schemas/MapLayer' } }
    Feature:
      type: object
      required: [type, geometry, properties]
      properties:
        type: { type: string, const: Feature }
        geometry: {}
        properties:
          type: object
          additionalProperties: true
    FeatureCollection:
      type: object
      required: [type, features]
      properties:
        type: { type: string, const: FeatureCollection }
        features:
          type: array
          items: { $ref: '#/components/schemas/Feature' }
    ListFlowResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/Flow' } }
        next: { type: [string, 'null'] }
    ListMessageResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/Message' } }
        next: { type: [string, 'null'] }
    ListTaskResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/Task' } }
        next: { type: [string, 'null'] }
    ListTraceResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/Trace' } }
        next: { type: [string, 'null'] }
    ListReplayResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/ReplaySession' } }
        next: { type: [string, 'null'] }
    ListTopicHealthResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/TopicHealth' } }
        next: { type: [string, 'null'] }
    ListProvenanceResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/Provenance' } }
        next: { type: [string, 'null'] }
    ListMapLayerResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/MapLayer' } }
        next: { type: [string, 'null'] }
    ListPackResponse:
      type: object
      required: [items, next]
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/Pack' } }
        next: { type: [string, 'null'] }
`

const typesTS = `/* Code generated by go generate ./api/...; DO NOT EDIT. */

export type Cursor = string | null;

export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
}

export interface Entity {
  id: string;
  type: string;
  canonical_id: string;
  display_name?: string;
  first_seen: string;
  last_seen: string;
  attrs?: Record<string, unknown>;
}

export interface Edge {
  src_id: string;
  dst_id: string;
  type: string;
  valid_from: string;
  valid_to?: string;
  evidence_msg?: string;
  weight: number;
  attrs?: Record<string, unknown>;
}

export interface Provenance {
  subject_kind: string;
  subject_id: string;
  stage: string;
  policy_ver?: string;
  inputs?: Record<string, unknown>;
  decision?: string;
  reasons?: string[];
  produced_at: string;
}

export interface Geometry {
  entity_id: string;
  geometry_type: string;
  geojson: unknown;
  srid: number;
  min_lat: number;
  min_lon: number;
  max_lat: number;
  max_lon: number;
  z_min?: number;
  z_max?: number;
  observed_at: string;
  valid_to?: string;
}

export interface Neighbor {
  entity_id: string;
  entity_type: string;
  weight: number;
}

export interface Profile {
  entity: Entity;
  first_seen: string;
  last_seen: string;
  edge_counts: Record<string, number>;
  top_neighbors: Neighbor[];
}

export interface Pointer {
  bucket: string;
  key: string;
  size: number;
  sha256: string;
  content_type?: string;
  created_at?: string;
  proxy_id?: string;
  path: string;
}

export interface Message {
  id: string;
  topic: string;
  topic_family: string;
  partition: number;
  offset: number;
  timestamp: string;
  envelope_type?: string;
  sender_id?: string;
  correlation_id?: string;
  trace_id?: string;
  task_id?: string;
  parent_task_id?: string;
  status?: string;
  preview?: string;
  content?: string;
  lfs?: Pointer;
}

export interface Flow {
  id: string;
  topic_count: number;
  sender_count: number;
  topics: string[];
  senders: string[];
  trace_ids: string[];
  task_ids: string[];
  first_seen: string;
  last_seen: string;
  latest_status?: string;
  message_count: number;
  latest_preview?: string;
}

export interface TopicHealth {
  topic: string;
  messages_per_hour: number;
  message_density: string;
  active_agents: number;
  is_stale: boolean;
  last_message_at?: string;
}

export interface Health {
  connected: boolean;
  effective_topics: string[];
  group_id: string;
  accepted_count: number;
  rejected_count: number;
  mirrored_count: number;
  mirror_failed_count: number;
  rejected_by_reason: Record<string, number>;
  last_reject?: string;
  last_mirror_error?: string;
  last_poll_at?: string;
  replay_status?: string;
  replay_active: number;
  replay_last_error?: string;
  replay_last_finished_at?: string;
  replay_last_record_count: number;
  topic_health: TopicHealth[];
}

export interface ReplaySession {
  id: string;
  group_id: string;
  status: string;
  started_at: string;
  finished_at?: string;
  message_count: number;
  topics?: string[];
  last_error?: string;
}

export interface ReplayRequest {
  id: string;
  status: string;
  requested_at: string;
  topics: string[];
}

export interface TypeSpec {
  name: string;
  source: string;
}

export interface MapLayer {
  id: string;
  name: string;
  kind: string;
  url?: string;
  attribution?: string;
  source: string;
}

export interface Pack {
  name: string;
  version: string;
  description?: string;
  owner?: string;
  entity_types?: string[];
  edge_types?: string[];
  map_layers?: MapLayer[];
}

export interface Feature {
  type: "Feature";
  geometry: unknown;
  properties: Record<string, unknown>;
}

export interface FeatureCollection {
  type: "FeatureCollection";
  features: Feature[];
}

export interface ConsumerGroupMember {
  member_id: string;
  client_id: string;
  client_host: string;
  instance_id?: string;
}

export interface ConsumerGroup {
  group_id: string;
  state: string;
  protocol_type: string;
  protocol: string;
  members: ConsumerGroupMember[];
}

export interface OperatorState {
  supported: boolean;
  live_group_id?: string;
  replay_group_ids: string[];
  groups: ConsumerGroup[];
  last_error?: string;
}

export interface Trace {
  id: string;
  span_count: number;
  agents: string[];
  span_types: string[];
  latest_title?: string;
  started_at?: string;
  ended_at?: string;
  duration_ms?: number;
}

export interface Task {
  id: string;
  parent_task_id?: string;
  delegation_depth?: number;
  requester_id?: string;
  responder_id?: string;
  original_requester_id?: string;
  status?: string;
  description?: string;
  last_summary?: string;
  first_seen: string;
  last_seen: string;
}

export interface FusionMatch {
  alert_id: string;
  title: string;
  category: string;
  severity: string;
  source: string;
  canonical_url: string;
  match_reasons: string[];
}

export type AgentOpsMode = "OSINT" | "AGENTOPS" | "HYBRID";

export interface AgentOpsState {
  generated_at: string;
  enabled: boolean;
  ui_mode: AgentOpsMode;
  profile: string;
  group_name: string;
  topics: string[];
  flow_count: number;
  trace_count: number;
  task_count: number;
  message_count: number;
  health: Health;
  replay_sessions: ReplaySession[];
  flows: Flow[];
  traces: Trace[];
  tasks: Task[];
  messages: Message[];
}

export interface ListResponse<T> {
  items: T[];
  next: Cursor;
}

export interface GraphPathResponse {
  found: boolean;
  edges: Edge[];
}

export interface NeighborhoodResponse {
  entities: Entity[];
  edges: Edge[];
}

export interface OntologyTypesResponse {
  entity_types: TypeSpec[];
  edge_types: TypeSpec[];
}

export type SearchResult = Record<string, unknown>;
export type SearchResponse = ListResponse<SearchResult>;

export type AgentOpsPointer = Pointer;
export type AgentOpsMessage = Message;
export type AgentOpsFlow = Flow;
export type AgentOpsTrace = Trace;
export type AgentOpsTask = Task;
export type AgentOpsTopicHealth = TopicHealth;
export type AgentOpsHealth = Health;
export type AgentOpsReplaySession = ReplaySession;
export type AgentOpsConsumerGroupMember = ConsumerGroupMember;
export type AgentOpsConsumerGroup = ConsumerGroup;
export type AgentOpsOperatorState = OperatorState;
export type AgentOpsFusionMatch = FusionMatch;
`

const clientTS = `/* Code generated by go generate ./api/...; DO NOT EDIT. */

import type {
  Cursor,
  FeatureCollection,
  Flow,
  Geometry,
  GraphPathResponse,
  Health,
  ListResponse,
  MapLayer,
  Message,
  NeighborhoodResponse,
  OntologyTypesResponse,
  Pack,
  ProblemDetails,
  Profile,
  Provenance,
  ReplayRequest,
  ReplaySession,
  SearchResponse,
  Task,
  TopicHealth,
  Trace,
} from "./types";

export class APIError extends Error {
  problem: ProblemDetails;

  constructor(problem: ProblemDetails) {
    super(problem.detail || problem.title);
    this.problem = problem;
  }
}

export interface RequestOptions {
  signal?: AbortSignal;
}

function buildQuery(params: Record<string, string | number | boolean | undefined>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    query.set(key, String(value));
  }
  const encoded = query.toString();
  return encoded ? "?" + encoded : "";
}

async function requestJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const response = await fetch(input, init);
  if (!response.ok) {
    const problem = (await response.json()) as ProblemDetails;
    throw new APIError(problem);
  }
  return (await response.json()) as T;
}

export class AgentOpsApiClient {
  private readonly baseUrl: string;

  constructor(baseUrl = "/api/v1") {
    this.baseUrl = baseUrl;
  }

  getEntity(type: string, id: string, options?: RequestOptions): Promise<Profile> {
    return requestJSON<Profile>(this.baseUrl + "/entities/" + encodeURIComponent(type) + "/" + encodeURIComponent(id), { signal: options?.signal });
  }

  getEntityNeighborhood(type: string, id: string, params: { depth?: number; types?: string; window?: string } = {}, options?: RequestOptions): Promise<NeighborhoodResponse> {
    return requestJSON<NeighborhoodResponse>(this.baseUrl + "/entities/" + encodeURIComponent(type) + "/" + encodeURIComponent(id) + "/neighborhood" + buildQuery(params), { signal: options?.signal });
  }

  listEntityProvenance(type: string, id: string, options?: RequestOptions): Promise<ListResponse<Provenance>> {
    return requestJSON<ListResponse<Provenance>>(this.baseUrl + "/entities/" + encodeURIComponent(type) + "/" + encodeURIComponent(id) + "/provenance", { signal: options?.signal });
  }

  getEntityGeometry(type: string, id: string, options?: RequestOptions): Promise<Geometry> {
    return requestJSON<Geometry>(this.baseUrl + "/entities/" + encodeURIComponent(type) + "/" + encodeURIComponent(id) + "/geometry", { signal: options?.signal });
  }

  listEntityTimeline(type: string, id: string, params: { after?: Cursor; limit?: number } = {}, options?: RequestOptions): Promise<ListResponse<Message>> {
    return requestJSON<ListResponse<Message>>(this.baseUrl + "/entities/" + encodeURIComponent(type) + "/" + encodeURIComponent(id) + "/timeline" + buildQuery({ after: params.after ?? undefined, limit: params.limit }), { signal: options?.signal });
  }

  getGraphPath(params: { src: string; dst: string; max?: number }, options?: RequestOptions): Promise<GraphPathResponse> {
    return requestJSON<GraphPathResponse>(this.baseUrl + "/graph/path" + buildQuery(params), { signal: options?.signal });
  }

  listMapFeatures(params: { bbox: string; types?: string; window?: string }, options?: RequestOptions): Promise<FeatureCollection> {
    return requestJSON<FeatureCollection>(this.baseUrl + "/map/features" + buildQuery(params), { signal: options?.signal });
  }

  listMapLayers(options?: RequestOptions): Promise<ListResponse<MapLayer>> {
    return requestJSON<ListResponse<MapLayer>>(this.baseUrl + "/map/layers", { signal: options?.signal });
  }

  listFlows(params: { after?: Cursor; limit?: number; topic?: string; sender?: string; status?: string; q?: string } = {}, options?: RequestOptions): Promise<ListResponse<Flow>> {
    return requestJSON<ListResponse<Flow>>(this.baseUrl + "/flows" + buildQuery({ ...params, after: params.after ?? undefined }), { signal: options?.signal });
  }

  getFlow(id: string, options?: RequestOptions): Promise<Flow> {
    return requestJSON<Flow>(this.baseUrl + "/flows/" + encodeURIComponent(id), { signal: options?.signal });
  }

  listFlowMessages(id: string, params: { after?: Cursor; limit?: number } = {}, options?: RequestOptions): Promise<ListResponse<Message>> {
    return requestJSON<ListResponse<Message>>(this.baseUrl + "/flows/" + encodeURIComponent(id) + "/messages" + buildQuery({ after: params.after ?? undefined, limit: params.limit }), { signal: options?.signal });
  }

  listFlowTasks(id: string, options?: RequestOptions): Promise<ListResponse<Task>> {
    return requestJSON<ListResponse<Task>>(this.baseUrl + "/flows/" + encodeURIComponent(id) + "/tasks", { signal: options?.signal });
  }

  listFlowTraces(id: string, options?: RequestOptions): Promise<ListResponse<Trace>> {
    return requestJSON<ListResponse<Trace>>(this.baseUrl + "/flows/" + encodeURIComponent(id) + "/traces", { signal: options?.signal });
  }

  listFlowTimeline(id: string, params: { after?: Cursor; limit?: number } = {}, options?: RequestOptions): Promise<ListResponse<Message>> {
    return requestJSON<ListResponse<Message>>(this.baseUrl + "/flows/" + encodeURIComponent(id) + "/timeline" + buildQuery({ after: params.after ?? undefined, limit: params.limit }), { signal: options?.signal });
  }

  listTopicHealth(options?: RequestOptions): Promise<ListResponse<TopicHealth>> {
    return requestJSON<ListResponse<TopicHealth>>(this.baseUrl + "/topic-health", { signal: options?.signal });
  }

  getHealth(options?: RequestOptions): Promise<Health> {
    return requestJSON<Health>(this.baseUrl + "/health", { signal: options?.signal });
  }

  listReplays(limit = 20, options?: RequestOptions): Promise<ListResponse<ReplaySession>> {
    return requestJSON<ListResponse<ReplaySession>>(this.baseUrl + "/replays" + buildQuery({ limit }), { signal: options?.signal });
  }

  createReplayRequest(topics: string[] = [], options?: RequestOptions): Promise<ReplayRequest> {
    return requestJSON<ReplayRequest>(this.baseUrl + "/replays", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ topics }),
      signal: options?.signal,
    });
  }

  searchEntities(q: string, options?: RequestOptions): Promise<SearchResponse> {
    return requestJSON<SearchResponse>(this.baseUrl + "/search" + buildQuery({ q }), { signal: options?.signal });
  }

  getOntologyTypes(options?: RequestOptions): Promise<OntologyTypesResponse> {
    return requestJSON<OntologyTypesResponse>(this.baseUrl + "/ontology/types", { signal: options?.signal });
  }

  getOntologyPacks(options?: RequestOptions): Promise<ListResponse<Pack>> {
    return requestJSON<ListResponse<Pack>>(this.baseUrl + "/ontology/packs", { signal: options?.signal });
  }
}
`

const indexTS = `/* Code generated by go generate ./api/...; DO NOT EDIT. */

export * from "./types";
export * from "./client";
`

const typesIndexTS = `/* Code generated by go generate ./api/...; DO NOT EDIT. */

export * from "@/agentops/lib/api-client/types";
`
