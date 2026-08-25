# Eino v0.9 Official Reference

Read these sources before making a v0.9 architecture claim. The release note
and compatibility note are maintained by CloudWeGo in the Eino repository.

## Primary sources

- Release note (v0.9 agentic-runtime):
  https://github.com/cloudwego/eino/blob/v0.10.0-alpha.22/V0.9_RELEASE_NOTE.md
- Compatibility note (v0.8 to v0.9):
  https://github.com/cloudwego/eino/blob/v0.10.0-alpha.22/V0.9_COMPATIBILITY_NOTE.md
- TurnLoop implementation (v0.9.15):
  https://github.com/cloudwego/eino/blob/v0.9.15/adk/turn_loop.go
- ADK agent interfaces (v0.9.15):
  https://github.com/cloudwego/eino/blob/v0.9.15/adk/interface.go
- CloudWeGo Eino manual:
  https://www.cloudwego.io/docs/eino/

## Changes relevant to this project

- Existing `*schema.Message` paths remain the conservative compatibility path.
  `schema.AgenticMessage` is a content-block protocol and requires an
  `AgenticModel`-compatible adapter; do not migrate just to obtain TurnLoop.
- `adk.TurnLoop` supplies `GenInput`, `GenResume`, `PrepareAgent`,
  `OnAgentEvents`, `Push`, `Run`, `Stop`, and `Wait`. It is a per-session
  runtime for queued turns, preemption, and checkpoint/resume.
- v0.9 adds explicit cancel semantics (`WithCancel`, cancel modes, timeout,
  recursive cancellation) and `CancelError`. Cancellation is not an ordinary
  retryable business error.
- Model retry can use `ShouldRetry(ctx, RetryContext)` to inspect output and
  choose a retry decision. Model failover can select another model and rewrite
  the next attempt input. Bound both by policy and budget.
- Runtime tool state is represented by `ToolInfos` and `DeferredToolInfos`.
  Modify them in the documented middleware state path, not by mutating a
  per-call model wrapper that disappears on the next turn/checkpoint.
- Tool search is discovery only. Server-side policy must determine which tools
  may appear in either list and must be checked again when executing a tool.
- `AgenticToolsNode` and `AgenticModel` are for content-block agent protocols;
  current ReAct `compose.Runnable` code can remain on the default path while
  the dependency is upgraded and compatibility is proven.

## Upgrade checklist

- Check handwritten `ChatModelAgentMiddleware` implementations for the new
  `AfterAgent` method.
- Replace removed standalone summarization helpers with middleware methods.
- Review custom finalizers because v0.9 changes raw-summary finalization order.
- Review any code mutating `ModelContext.Tools`; use runtime state tool lists.
- Regenerate lock data with the chosen v0.9 patch release, then run the full
  test/build and a rollback build using the previous version.
