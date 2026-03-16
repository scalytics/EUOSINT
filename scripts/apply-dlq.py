#!/usr/bin/env python3
"""Apply dead-letter queue rejections to the local JSON registry.

Usage: python3 scripts/apply-dlq.py registry/source_registry.json .tmp/dlq.json
"""
import json
import sys


def main():
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <registry.json> <dlq.json>", file=sys.stderr)
        sys.exit(1)

    registry_path, dlq_path = sys.argv[1], sys.argv[2]

    with open(dlq_path) as f:
        dlq = json.load(f)

    dead_sources = {}
    for src in dlq.get("sources", []):
        sid = src.get("source_id", "")
        if sid:
            dead_sources[sid] = src.get("error", "unknown error")

    if not dead_sources:
        print("DLQ is empty, nothing to apply.")
        return

    with open(registry_path) as f:
        registry = json.load(f)

    changed = 0
    for entry in registry:
        sid = entry.get("source", {}).get("source_id", "")
        if sid in dead_sources and entry.get("promotion_status") != "rejected":
            entry["promotion_status"] = "rejected"
            entry["rejection_reason"] = f"Dead source: {dead_sources[sid]}"
            changed += 1

    if changed == 0:
        print("No new rejections to apply.")
        return

    with open(registry_path, "w") as f:
        json.dump(registry, f, indent=2, ensure_ascii=False)
        f.write("\n")

    print(f"Rejected {changed} dead source(s) in {registry_path}")


if __name__ == "__main__":
    main()
