<!--
Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
SPDX-License-Identifier: Apache-2.0
-->

# ACLED Conflict Data Integration

EUOSINT integrates with the [Armed Conflict Location & Event Data](https://acleddata.com/) (ACLED) project to provide structured, geo-located conflict event data covering battles, explosions, violence against civilians, protests, riots, and strategic military developments worldwide.

## Setup

1. Register at **https://acleddata.com/register/**
2. **Use an institutional email** (`@yourorg.com`) — gmail and other public email addresses only grant "Open myACLED" (web-based data export), which does **not** include API access
3. Institutional accounts should have API access by default. If not, contact `access@acleddata.com` to request the API tier
4. Set credentials in `.env`:

```dotenv
ACLED_USERNAME=you@yourorg.com
ACLED_PASSWORD=your-password
```

5. The collector will automatically start pulling ACLED events on the next run.

If credentials are not set, the ACLED source is silently skipped — all other sources continue normally.

### Verifying API Access

You can test whether your account has API access:

```bash
# Get an OAuth token
TOKEN=$(curl -s -X POST "https://acleddata.com/oauth/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "username=YOU@YOURORG.COM" \
  -d "password=YOUR_PASSWORD" \
  -d "grant_type=password" \
  -d "client_id=acled" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

# Test the API — should return JSON data, not "Access denied"
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://acleddata.com/api/acled/read?_format=json&limit=2"
```

If you get `{"message":"Access denied"}`, your account does not have API access yet.

## Authentication

ACLED uses OAuth2 password grant:

1. Collector POSTs to `https://acleddata.com/oauth/token` with `grant_type=password`, `client_id=acled`, and your credentials
2. Receives a Bearer access token (valid 24 hours)
3. Uses the token in `Authorization: Bearer <token>` header for API requests

A new token is obtained at the start of each collection run. No token caching or refresh logic is needed since collection runs are ~15 minutes apart and tokens last 24 hours.

## Data Flow

```
ACLED API (/api/acled/read)
  → parse.ParseACLED()         — JSON → ACLEDItem (with lat/lng, event type, fatalities)
  → normalize.ACLEDAlert()     — category/severity mapping, ISO3→ISO2 country, geo override
  → standard pipeline          — FTS indexing, trend detection, country digest, globe view
```

## Event Type Mapping

| ACLED Event Type             | EUOSINT Category       | Default Severity |
|------------------------------|------------------------|------------------|
| Battles                      | conflict_monitoring    | high             |
| Explosions/Remote violence   | conflict_monitoring    | high             |
| Violence against civilians   | conflict_monitoring    | high             |
| Protests                     | public_safety          | medium           |
| Riots                        | public_safety          | medium           |
| Strategic developments       | intelligence_report    | medium           |

Severity is upgraded to **critical** when fatalities >= 10, and to **high** when any fatalities are reported.

## Query Parameters

The collector fetches a rolling 7-day window, ordered by date descending. The registry entry limits output to 100 events per run (`max_items: 100`). Pagination uses ACLED's `page` and `limit` parameters with 500 events per page.

## Geo-Precision

Unlike RSS-based sources that are pinned to country capitals, ACLED events use precise coordinates from the dataset (down to village/neighbourhood level). This gives accurate placement on the globe view.

Each event's source metadata (country, country code, region) is set per-event from ACLED data rather than using the registry's "International" default. This means ACLED events appear correctly when filtering by country or region.
