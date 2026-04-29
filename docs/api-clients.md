# API Clients

The kafSIEM analyst API is served by `cmd/kafsiem-api` and versioned under
`/api/v1`. The generated OpenAPI contract is [api/openapi.yaml](../api/openapi.yaml).

Regenerate API artifacts after changing the source contract:

```bash
go generate ./api/...
```

Generated outputs:

- `api/openapi.yaml`
- `src/agentops/lib/api-client/types.ts`
- `src/agentops/lib/api-client/client.ts`
- `src/agentops/lib/api-client/index.ts`

## TypeScript

The repository-local TypeScript client is generated for the web app:

```ts
import { AgentOpsApiClient } from "@/agentops/lib/api-client";

const api = new AgentOpsApiClient("/api/v1");
const profile = await api.getEntity("platform", "auv-7");
const neighborhood = await api.getEntityNeighborhood("platform", "auv-7", {
  depth: 2,
});
```

Errors are thrown as `APIError` with the RFC 9457 problem payload attached:

```ts
import { APIError } from "@/agentops/lib/api-client";

try {
  await api.getEntity("platform", "missing");
} catch (error) {
  if (error instanceof APIError) {
    console.error(error.problem.type, error.problem.status);
  }
}
```

## Go

The Go API service in this repository is implemented in `internal/kafsiemapi`.
Third-party Go clients should generate client types from `api/openapi.yaml`.
For example, with `oapi-codegen`:

```bash
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
oapi-codegen -generate types,client -package kafsiemclient api/openapi.yaml > kafsiemclient.gen.go
```

Example client usage:

```go
client, err := kafsiemclient.NewClientWithResponses("http://localhost:8080/api/v1")
if err != nil {
	return err
}

resp, err := client.GetEntityWithResponse(ctx, "platform", "auv-7")
if err != nil {
	return err
}
if resp.JSON200 == nil {
	return fmt.Errorf("kafSIEM returned status %d", resp.StatusCode())
}
```

## Base URLs

- Docker web/Caddy path: `http://localhost:8080/api/v1`
- Direct API service path inside compose: `http://kafsiem-api:8081/api/v1`
- Local API process path: `http://127.0.0.1:8081/api/v1`

Problem details are documented in [docs/agentops-api-errors.md](agentops-api-errors.md).
