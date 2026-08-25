---
name: eino-architecture-engineer
description: Use when designing, upgrading, reviewing, or debugging a Go service built with CloudWeGo Eino, especially when the task mentions Eino v0.9, agentic-runtime, TurnLoop, AgenticMessage, ChatModelAgent, tool search, cancellation, checkpoint/resume, model retry/failover, multi-user conversations, or high-concurrency isolation.
---

# Eino Architecture Engineer

Use Eino's official runtime primitives for orchestration and lifecycle. Keep
identity, authorization, conversation ownership, and persistence in the
application boundary. A reusable Eino object is not a reusable user session.

## First: establish the version path

1. Read `go.mod`, `go.sum`, and all Eino imports. Record the exact version and
   whether the code uses `compose.Runnable`, `flow/agent/react`, or `adk`.
2. Read [references/eino-v09-official.md](references/eino-v09-official.md) and
   verify the current official source before relying on an API.
3. Separate these decisions:
   - **Compatibility upgrade:** keep `*schema.Message` and existing
     `compose.Runnable` behavior while moving to Eino v0.9.
   - **Agentic protocol migration:** use `schema.AgenticMessage` only when the
     selected model adapter supports `AgenticModel` and content blocks.
   - **Long-lived turns:** use `adk.TurnLoop` only for a session runtime that
     needs queued input, preemption, or checkpoint/resume.
4. Do not claim that a dependency bump adopts TurnLoop. Prove each selected
   path with compile, unit, integration, and rollback checks.

## Architecture boundaries

Use these layers for a multi-user service:

| Layer | Owns | Must not own |
| --- | --- | --- |
| Process Runtime | compiled graph, model clients, retrievers, tool factories, health and shutdown | tenant ID, user ID, conversation history, request writer |
| Conversation runtime | one `SessionKey{tenant,user,conversation}`, turn queue, checkpoint ID | other sessions or global mutable prompt state |
| Request context | auth principal, request ID, cancellation, trace data | data copied from user text that overrides auth |
| Durable store | versioned messages, idempotency, checkpoints, audit records | in-memory-only cross-instance ordering |
| Tool policy | server-side allow/deny and tenant credentials | model-supplied DSN, raw user authorization |

For Eino v0.9, prefer one `adk.TurnLoop` per long-lived conversation. Use
`GenInput` to select/merge queued items, `PrepareAgent` to build the turn's
agent view, and `OnAgentEvents` as the single event consumer. Call `Push` from
the HTTP adapter, `Run` once, and `Wait` during controlled shutdown. Never put
one TurnLoop in a global Runtime for all users.

## TurnLoop and cancellation pattern

Use the framework's controls instead of a hand-written per-session loop:

```go
loop := adk.NewTurnLoop(adk.TurnLoopConfig[ChatInput, *schema.Message]{
    GenInput: func(ctx context.Context, l *adk.TurnLoop[ChatInput, *schema.Message], items []ChatInput) (*adk.GenInputResult[ChatInput, *schema.Message], error) {
        consumed, remaining := chooseTurn(items) // enforce one session key
        return &adk.GenInputResult[ChatInput, *schema.Message]{
            RunCtx: ctx, Consumed: consumed, Remaining: remaining,
            Input: &adk.TypedAgentInput[*schema.Message]{Messages: toMessages(consumed), EnableStreaming: true},
        }, nil
    },
    PrepareAgent: func(ctx context.Context, _ *adk.TurnLoop[ChatInput, *schema.Message], _ []ChatInput) (adk.Agent, error) {
        return runtimeAgent, nil // request-free shared dependencies
    },
    OnAgentEvents: writeOneSSEStream,
})
loop.Run(serviceContext)
accepted, _ := loop.Push(input)
```

The actual adapter must also:

- derive `RunCtx` from the authenticated request context;
- propagate client disconnect to Eino cancellation (`WithCancel`, cancel mode,
  and timeout as appropriate), not merely stop writing bytes;
- distinguish `CancelError`, business interrupt, and ordinary errors;
- commit the complete user/assistant result before emitting terminal `done`;
- persist `CheckpointID`, interrupted items, unhandled items, and an idempotency
  key in the durable store before allowing resume;
- bound event buffering and have exactly one SSE writer.

## Tool governance in v0.9

Authorization happens before model exposure. A tool search result is discovery,
not permission.

1. Resolve a server-side `ToolRegistry` from principal, tenant policy, intent,
   and named data sources.
2. Put immediately allowed tools in `ChatModelAgentState.ToolInfos` from
   `BeforeModelRewriteState`.
3. Put only policy-approved discoverable tools in `DeferredToolInfos`.
4. Configure native, schema-based, or Eino custom tool search only after the
   allowlist is applied. Re-check authorization at tool execution time.
5. Pass request context and per-tool deadlines. Never accept a model-supplied
   DSN, unrestricted SQL, or terminal process exit.
6. Record tool authorization decisions and execution outcomes without putting
   tenant/user IDs into high-cardinality metric labels.

## Reliability and graph construction

- Treat every `AddLambdaNode`, `AddRetrieverNode`, `AddEdge`, and `Compile`
  error as startup-fatal; include the node or edge name in the error.
- Use v0.9 `ShouldRetry` when retry depends on output or business acceptance;
  do not retry cancellation, stream cancellation, authorization failures, or
  non-idempotent writes.
- Use model failover for provider outages, with explicit timeout, token budget,
  cost metadata, and tenant policy. Do not hide failover behind an infinite
  retry loop.
- Keep shared agents request-free. Test concurrent Invoke/Stream calls with
  independent contexts and inputs; run `go test -race` in CI.
- Own every client created at startup. Expose health checks and `Close`/drain
  behavior; do not recreate Milvus, embedding, MCP, or model clients inside a
  tool invocation.

## Required verification

Before declaring an Eino change complete, show:

1. `go test ./...` and `go build ./...` on the selected version.
2. A compatibility test for unchanged `schema.Message` paths.
3. Tenant and conversation isolation tests with identical IDs across tenants.
4. Same-session ordering and different-session parallelism tests.
5. Client-disconnect cancellation and bounded-SSE tests.
6. Tool allow/deny, retry/failover, graph-construction, checkpoint/resume, and
   idempotency tests for any enabled feature.
7. A rollback command or previous lockfile that restores the prior Eino version.

Report which Eino primitives were adopted, which remain intentionally unused,
and which upstream version was actually verified.
