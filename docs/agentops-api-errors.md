# AgentOps API Errors

## `bad-request`

The request parameters were missing or malformed.

## `bad-cursor`

The supplied pagination cursor could not be decoded.

## `bad-bbox`

The supplied bounding box did not follow `minLon,minLat,maxLon,maxLat`.

## `db-unavailable`

The API could not reach the SQLite backing store.

## `not-ready`

The API has not observed a usable recent health snapshot yet.

## `stale-health`

The latest collector health snapshot is older than the readiness threshold.

## `unknown-entity-type`

The requested entity type is not part of the active core or pack ontology.

## `entity-not-found`

The requested entity was not found for the given type and id.

## `not-found`

The requested API resource was not found.

## `read-only-api`

The route is intentionally unavailable because the standalone API process is read-only.

## `internal-error`

An unexpected server-side error occurred while processing the request.
