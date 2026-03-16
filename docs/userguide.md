# EUOSINT User Guide

## Data Sources

### Interpol Notices

EUOSINT pulls the **newest 160 Red Notices** (wanted suspects) and **newest 160 Yellow Notices** (missing persons) from the Interpol public API per collector run. This limit is intentional to avoid data overflow and excessive API load.

- Red Notices: ~6,400 active notices globally
- Yellow Notices: ~4,000 active notices globally

Only the most recent window is fetched each cycle. Notices are pinned on the map to the suspect's nationality country rather than Interpol HQ in Lyon.

### Severity Classification

Alerts are classified into five severity levels based on title keyword analysis:

| Level | Examples |
|-------|---------|
| **Critical** | Zero-day exploits, ransomware, active exploitation, wanted fugitives, missing persons, AMBER alerts |
| **High** | Vulnerabilities, compromises, phishing campaigns, fraud, urgent advisories |
| **Medium** | Arrests, charges, sentences, moderate-severity advisories |
| **Info** | Newsletters, info packets (Infopaket), guidance documents (Handreichung, Leitfaden) |

Keyword matching supports both English and German terms (e.g., "Kritische Schwachstelle" maps to critical/high).

### Map Tiles

The map uses [CARTO](https://carto.com/) dark basemap tiles loaded from their CDN. An active internet connection is required for map rendering. Missing or slow-loading tiles indicate network connectivity issues to `basemaps.cartocdn.com`.

### Collector Cycle

The collector runs on a configurable interval (default: 15 minutes). Each run:

1. Fetches all active sources from the registry
2. Parses and normalizes alerts
3. Deduplicates across sources
4. Reconciles with previous state (tracks new, active, and removed alerts)
5. Outputs JSON snapshots consumed by the frontend

Removed alerts (e.g., a resolved Interpol notice) are retained in state for 14 days before being purged.
