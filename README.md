# EUOSINT

EUOSINT is the EU-focused hard fork of the OSINT SIEM dashboard. This repository imports the codebase from [novatechflow/osint-siem](https://github.com/novatechflow/osint-siem) and packages it to run locally with Docker while EU-specific adaptations are built out.

## Provenance

- Hard fork source: [novatechflow/osint-siem](https://github.com/novatechflow/osint-siem)
- Original upstream repository: [cyberdude88/osint-siem](https://github.com/cyberdude88/osint-siem)

This project is based on the work of `cyberdude88/osint-siem` and the downstream `novatechflow/osint-siem` fork. EUOSINT keeps that lineage explicit in this repository through the `NOTICE` file and per-file provenance headers where comments are supported.

## Run With Docker

```bash
if command -v docker-compose >/dev/null 2>&1; then
  docker-compose up --build
else
  docker compose up --build
fi
```

The application will be available at `http://localhost:8080`.

You can also use the Make targets for local HTTP development:

```bash
make dev-start
make dev-stop
make dev-restart
make dev-logs
```

## Remote Install (wget bootstrap)

```bash
wget -qO- https://raw.githubusercontent.com/scalytics/EUOSINT/main/deploy/install.sh | bash
```

The installer will:
- verify Docker + Compose availability
- clone or update the repo on the host
- set GHCR runtime images (`ghcr.io/scalytics/euosint-web` + `ghcr.io/scalytics/euosint-collector`)
- prompt for install mode (`preserve` or `fresh` volume reset)
- prompt for domain (`EUOSINT_SITE_ADDRESS`)
- when domain mode is enabled, optionally check `ufw`/`firewalld` and validate local 80/443 availability
- prompt for key runtime flags (browser + LLM vetting settings)
- optionally run `docker compose pull` and start with `--no-build`

- The release pipeline now builds two images: a web image and a Go collector image.
- The scheduled feed refresh workflow now runs the Go collector.
- The web image now uses Caddy instead of nginx, with the collector output mounted into the web container at runtime.
- In Docker dev mode, the collector initializes empty JSON outputs on a fresh volume and then writes live output on the first successful run.

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
- `docker-compose up --build` or `docker compose up --build` now runs the Go collector as a background refresh service and serves the generated JSON through the Caddy web container.
- VM/domain deployment instructions live in [docs/operations.md](/Users/alo/Development/scalytics/EUOSINT/docs/operations.md).

## Notes

- Local toolchain is pinned to Node `25.8.1` and npm `11.11.0` via `package.json`, `.nvmrc`, and `.node-version`.
- The Go collector is now the operational backend for scheduled feed refreshes, Docker runtime, and local commands.
- The imported application still reflects upstream geographic coverage and source selection; EU-specific source tuning is a follow-up change.
- The root `LICENSE` applies to repository-local materials and modifications added here. Upstream repository metadata should be reviewed separately for inherited code provenance.
