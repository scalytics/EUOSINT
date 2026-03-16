# EUOSINT

EUOSINT is the EU-focused hard fork of the OSINT SIEM dashboard. This repository imports the codebase from [novatechflow/osint-siem](https://github.com/novatechflow/osint-siem) and packages it to run locally with Docker while EU-specific adaptations are built out.

## Provenance

- Hard fork source: [novatechflow/osint-siem](https://github.com/novatechflow/osint-siem)
- Original upstream repository: [cyberdude88/osint-siem](https://github.com/cyberdude88/osint-siem)

This project is based on the work of `cyberdude88/osint-siem` and the downstream `novatechflow/osint-siem` fork. EUOSINT keeps that lineage explicit in this repository through the `NOTICE` file and per-file provenance headers where comments are supported.

## Run With Docker

```bash
docker-compose up --build
```

The application will be available at `http://localhost:8080`.

- The release pipeline now builds two images: a web image and a Go collector image.
- The scheduled feed refresh workflow still uses the legacy Node collector until the Go collector reaches parity with the reference implementation.

## Run Locally Without Docker

```bash
volta install node@25.8.1 npm@11.11.0
npm install
npm run fetch:alerts:watch
npm run dev
```

For resilient 24/7 collection with auto-restart on crashes:

```bash
npm run collector:run
```

Tuning examples:

```bash
INTERVAL_MS=120000 MAX_PER_SOURCE=80 npm run collector:run
```

Legacy reference collector commands remain available during parity work:

```bash
npm run fetch:alerts:legacy
npm run fetch:alerts:watch:legacy
npm run collector:run:legacy
```

## Operations

```bash
make check
make ci
make docker-build
```

- `make release-patch`, `make release-minor`, and `make release-major` create and push semver tags that trigger the release workflow.
- `.github/workflows/branch-protection.yml` applies protection to `main` using the `ADMIN_GITHUB_TOKEN` repository secret.
- Docker validation runs through `buildx`, and release images publish to GHCR on semver tags.
- Release images are published as `ghcr.io/<owner>/<repo>-web` and `ghcr.io/<owner>/<repo>-collector`.

## Notes

- Local toolchain is pinned to Node `25.8.1` and npm `11.11.0` via `package.json`, `.nvmrc`, and `.node-version`.
- The Go collector is the target backend, but `scripts/fetch-alerts.mjs` remains the operational parity reference until the Go output is validated against it.
- The imported application still reflects upstream geographic coverage and source selection; EU-specific source tuning is a follow-up change.
- The root `LICENSE` applies to repository-local materials and modifications added here. Upstream repository metadata should be reviewed separately for inherited code provenance.
