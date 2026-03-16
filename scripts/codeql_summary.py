#!/usr/bin/env python3
"""
EUOSINT
Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
See NOTICE for provenance and LICENSE for repository-local terms.
"""

import json
import pathlib
import sys


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: codeql_summary.py <sarif-path>")
        return 1

    path = pathlib.Path(sys.argv[1])
    if not path.exists():
        print(f"Missing {path}. Run `make code-ql` first.")
        return 1

    data = json.loads(path.read_text())
    results = []
    for run in data.get("runs", []):
      results.extend(run.get("results", []))

    print(f"CodeQL findings: {len(results)}")
    for result in results[:20]:
        level = result.get("level", "warning")
        rule = result.get("ruleId", "unknown-rule")
        message = (result.get("message") or {}).get("text", "").replace("\n", " ").strip()
        loc = (result.get("locations") or [{}])[0]
        phys = loc.get("physicalLocation") or {}
        art = phys.get("artifactLocation") or {}
        region = phys.get("region") or {}
        print(
            f"- [{level}] {rule} :: {art.get('uri', 'unknown')}:{region.get('startLine', 0)}"
            f" :: {message[:160]}"
        )

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
