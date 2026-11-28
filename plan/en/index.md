# Plan for project-b

The plan covers everything an online game needs: the simulation engine, space, characters and combat, effects, storage, network, accounts, client, tools and content.

## Principles

- Every idea is designed for project-b itself, and the owner approves a design before any code is written.
- The owner decides every state method, action and mechanism (R5). The plan only lists proposals, marked `pending approval`.
- The plan is divided into phases and tasks, and every phase has a measurable acceptance. A later phase builds on an earlier one.
- The rules in `plan/rule/` (R0 to R16) apply to every phase.

## Files

| File | Content |
|---|---|
| [proposed-designs.md](proposed-designs.md) | The proposed designs; what `observe` and `effects` are |
| [phases.md](phases.md) | Phases 0 to 10: tasks and acceptance |
| [decisions.md](decisions.md) | The owner's decisions: decided and open |
| [protocol.md](protocol.md) | The protocol between the client and the server: layers, messages, choices |
| [roadmap.md](roadmap.md) | What is to be done soonest, in order |

## Status of an item

- `done`: built and tested.
- `agreed`: in the plan, not yet designed in detail.
- `pending approval`: proposed, not yet approved.
