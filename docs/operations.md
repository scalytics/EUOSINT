<!--
Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
SPDX-License-Identifier: Apache-2.0
-->

# Operations

## Runtime Model

The production stack has two containers:

- `collector`: the Go collector running in watch mode and writing refreshed JSON feeds into a shared Docker volume
- `euosint`: the React bundle served by Caddy, reading the shared JSON volume and serving the UI plus feed files

The web service no longer uses nginx. Caddy serves the SPA, exposes `/alerts.json`, `/alerts-filtered.json`, `/alerts-state.json`, and `/source-health.json`, and can manage TLS automatically when you give it a real domain.

## Local Compose

Copy the example environment file and start the stack:

```bash
cp .env.example .env
docker compose up --build -d
```

If your host only has the legacy plugin installed, use:

```bash
docker-compose up --build -d
```

Default local behavior:

- HTTP on `http://localhost:8080`
- HTTPS listener mapped to `https://localhost:8443` but not used unless `EUOSINT_SITE_ADDRESS` is changed to a hostname that enables TLS
- The collector initializes empty JSON outputs on a fresh shared feed volume, then replaces them with live collector output on the first successful run

## Domain Setup For A VM

1. Provision a VM with Docker Engine and Docker Compose available.
2. Point a DNS `A` record for your chosen hostname to the VM public IPv4 address.
3. Open inbound TCP `80` and `443` on the VM firewall and any cloud security group.
4. Copy the repository to the VM.
5. Create a `.env` file in the repo root:

```dotenv
EUOSINT_SITE_ADDRESS=alerts.example.com
EUOSINT_HTTP_PORT=80
EUOSINT_HTTPS_PORT=443
```

6. Start the stack:

```bash
docker compose up --build -d
```

With a real domain in `EUOSINT_SITE_ADDRESS`, Caddy will request and renew TLS certificates automatically and store them in the `caddy-data` volume.

## VM Service With systemd

Use the checked-in unit at [docs/euosint.service](/Users/alo/Development/scalytics/EUOSINT/docs/euosint.service) so the stack comes back after host reboots:

Install it on the VM:

```bash
sudo cp docs/euosint.service /etc/systemd/system/euosint.service
sudo systemctl daemon-reload
sudo systemctl enable --now euosint.service
```

If the VM only has `docker-compose`, adjust the unit commands accordingly.

## Operational Notes

- The collector writes feed output into the `feed-data` volume shared with the web container.
- The UI footer freshness line is derived from `source-health.json.generated_at` and shows the age of the current collector snapshot. It is normal below 20 minutes, warning from 20 to 60 minutes, and stale above 60 minutes.
- Discovery intake lives in [registry/source_candidates.json](/Users/alo/Development/scalytics/EUOSINT/registry/source_candidates.json).
- Dead sources are written to the terminal DLQ in `source_dead_letter.json` and are not crawled again.
- LLM-assisted source vetting is documented in [docs/source-vetting.md](/Users/alo/Development/scalytics/EUOSINT/docs/source-vetting.md).
- TLS state and certificates persist in the `caddy-data` volume.
- Caddy runtime state persists in the `caddy-config` volume.
- Scheduled refreshes, Docker runtime, and local collection commands all run through the Go collector.
