# Decisions of the owner

## Decided

| Topic | Decision |
|---|---|
| Phase order | The client comes early: phase 2 is a playable slice (fight, pick up, drop, monsters); the later phases run in parallel with the frontend |
| Parallel work | The owner decides by opening separate sessions and branches; the plan only states dependencies |
| Performance | 2,000 players at once on one machine (one machine is one world), p99 of a tick under 20 ms at the network layer and under 45 ms per layer; these are minimums |
| Entities | An interface: `ID`, `KindID`, `Position`, `Radius`, `BoundingBox`; collision by radius; `CharacterEntity` and `ItemEntity` (with expiry) first |
| Data in the simulation | Every piece of data is a value or has a `Clone` (R29) |
| Owner of data | A piece of data belongs to exactly one engine; remove first, then send; save after the tick ends (R28) |
| Replay | No replay of a single engine; checkpoints of the whole state of some engines at a period, offset between engines to avoid CPU peaks |
| `seq` of messages | Left as it is |
| Business examples | Allowed in the code as examples, to expose a design that does not fit early; a base game can change them later |
| Protocol | Fast and light: protobuf with `buf`, binary WebSocket frames |
| Replay log | An append-only file like Kafka's, with checkpoints; format as in [proposed-designs.md](proposed-designs.md); written to a file, pushed to S3 later (low priority, phase 6) |
| Recovery | Option A: every engine loads its own latest checkpoint and replays its own input; reads of another engine's state (`StableRead`) are not recorded by value, so after a failure they run against the recovered states. A verifying replay from tick 0 still uses the hash |
| Checkpoint period | A config parameter like the hash, but much rarer than the hash |
| R4 | Fixed: business examples are allowed when marked as examples and replaceable without changing the mechanism |
| Production environment | Not now; docker compose only |
| Important data | Postgres; generated queries (sqlc), migrations (goose) |
| Password hashing | argon2id; session token; authenticate once when the WebSocket opens, later messages are not authenticated again |
| Web client | TypeScript, PixiJS and Vite; stable, fast and easy to write |
| Observability | Not needed now; in the plan at low priority (phase 8) |
| Deployment | docker compose |
| Records | No ADRs in project-b; no `depguard` |

## Still open

None.
