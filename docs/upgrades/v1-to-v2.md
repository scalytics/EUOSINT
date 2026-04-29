# v1 To v2 Upgrade Notes

No v2 upgrade path is defined yet.

The `/api/v1` contract is the current compatibility boundary. Breaking API,
storage, or pack-schema changes should not be introduced under `/api/v1`.

When v2 is planned, this file must document:

- breaking API route or schema changes
- storage migration requirements
- pack-schema changes
- operational rollout and rollback steps
- client regeneration steps
