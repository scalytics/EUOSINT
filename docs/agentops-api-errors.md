# AgentOps API Errors

The kafSIEM analyst API returns errors as `application/problem+json` following
RFC 9457 Problem Details.

Problem `type` values are relative documentation links in this form:

```text
/docs/agentops-api-errors.md#<slug>
```

Clients should branch on the slug in `type`, not on the human-readable `title`.
The `instance` field is the request path that produced the error.

## Registry

| Slug | Status | Meaning | Operator Action |
| --- | --- | --- | --- |
| `bad-request` | `400` | Request parameters were missing or malformed. | Fix the request parameters and retry. |
| `bad-cursor` | `400` | The supplied pagination cursor could not be decoded. | Restart pagination without the stale cursor. |
| `bad-bbox` | `400` | The bounding box did not follow `minLon,minLat,maxLon,maxLat`. | Send a valid four-value bounding box. |
| `db-unavailable` | `503` | The API could not reach the SQLite backing store. | Check `/data/agentops.db`, mounts, and file permissions. |
| `not-ready` | `503` | The API has not observed a usable recent health snapshot. | Wait for the collector to write health, or check collector logs. |
| `stale-health` | `503` | The latest collector health snapshot is older than the readiness threshold. | Check whether the collector is stalled or disconnected from Kafka. |
| `unknown-entity-type` | `404` | The requested entity type is not part of the active core or pack ontology. | Check `/api/v1/ontology/types` and active packs. |
| `entity-not-found` | `404` | The requested entity id was not found for the requested type. | Verify the canonical id and time window. |
| `legacy-proxy-disabled` | `501` | A legacy `/api/*` route needed the collector proxy, but no proxy URL was configured. | Configure `KAFSIEM_COLLECTOR_BASE_URL` for the API process. |
| `bad-legacy-proxy` | `500` | The configured legacy collector proxy URL could not be parsed. | Fix `KAFSIEM_COLLECTOR_BASE_URL`. |
| `legacy-proxy-failed` | `502` | The API failed while forwarding a legacy route to the collector. | Check collector availability and network routing. |
| `not-found` | `404` | The requested API resource was not found. | Check the route and `/api/v1` prefix. |
| `internal-error` | `500` | Unexpected server-side failure. | Capture API logs and request context before retrying. |

## Example

```json
{
  "type": "/docs/agentops-api-errors.md#unknown-entity-type",
  "title": "Not Found",
  "status": 404,
  "detail": "entity type is not enabled",
  "instance": "/api/v1/entities/platform/auv-7"
}
```
