# Eino Architecture Skill Evaluation

The three scenarios in `eino-architecture-engineer-evals.json` were run once
without the skill and once with the skill plus the official v0.9 references.

## Baseline gaps

- Upgrade advice was generic and did not separate the compatible
  `schema.Message`/`compose.Runnable` path from `AgenticMessage` and `TurnLoop`.
- Multi-turn advice described custom locks and queues but omitted the Eino
  `Push`, `GenInput`, `PrepareAgent`, `OnAgentEvents`, and checkpoint APIs.
- Tool policy advice mentioned dynamic search but omitted the v0.9
  `ToolInfos`/`DeferredToolInfos` state path and the discovery-versus-
  authorization distinction.

## With-skill results

All six assertions passed:

- v0.6 to v0.9 compatibility upgrade was separated from AgenticMessage and
  TurnLoop migration, with official compatibility checks, full test/build, and
  rollback gates.
- A single TurnLoop was assigned to each authenticated
  `(tenant_id, user_id, conversation_id)` session; Push, GenInput,
  PrepareAgent, OnAgentEvents, cancellation, checkpoint/resume, and bounded
  SSE behavior were covered.
- ToolInfos and DeferredToolInfos were assigned through the documented state
  rewrite path. Server-side authorization was required both before discovery
  and at execution, with policy audit and tenant-safe checkpoint state.

The exact v0.9 patch and model-adapter support remain deployment decisions;
the skill correctly keeps those choices explicit rather than assuming that a
dependency bump automatically enables TurnLoop or AgenticMessage.
