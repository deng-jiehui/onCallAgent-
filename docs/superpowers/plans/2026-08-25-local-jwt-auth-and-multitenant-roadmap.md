# Local JWT Authentication and Multi-User Roadmap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a replaceable local JWT authentication module now and preserve explicit integration points for the later multi-user conversation, tenancy, concurrency, and runtime phases.

**Architecture:** The first phase adds a small authentication package with local configured users, bcrypt password verification, HS256 JWT issuance/validation, and context-based principals. Authentication is attached to protected `/api` routes while `/api/auth/login` remains public. Later phases replace the local user repository with PostgreSQL/OIDC and add shared conversation storage, Redis coordination, tenant-scoped retrieval, runtime reuse, and bounded concurrency without changing the controller's identity contract.

**Tech Stack:** Go 1.24, GoFrame v2 HTTP routing, `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, existing OpenTelemetry request context.

## Implementation Status

- Phase 1 local JWT authentication: implemented and pushed in commit `db56cf1`.
- Phase 1 project README and safe configuration template: implemented and pushed in commit `9feb186`.
- Phase 2A session identity and in-process coordination: implemented in commit `9a14cec`; the store interface remains the seam for PostgreSQL/Redis.
- Phase 2B shared PostgreSQL/Redis persistence, tenant-filtered retrieval, and cluster coordination: pending.
- Phase 2B tenant-scoped Milvus retrieval and indexing metadata: implemented in the current follow-up; retrieval quality tuning and shared persistence remain pending.
- Phase 3 Agent Runtime, policy routing, and safe tools: pending.

## Global Constraints

- JWT signing uses HS256 only in the local module; the parser must reject other algorithms.
- The JWT secret is read from `SUPERBIZ_JWT_SECRET`; it must not be committed to source or config files.
- Passwords are verified with bcrypt hashes; plaintext passwords are never persisted or logged.
- `tenant_id` and `user_id` come from the authenticated principal, not request-body fields.
- Authentication is a prerequisite for production multi-user isolation; a development-only fallback identity is not allowed in production.
- This phase does not implement PostgreSQL, Redis, conversation persistence, model routing, tenant-filtered Milvus queries, or concurrency limits.
- Every production function added in this phase gets a focused test before implementation code is considered complete.

## File Map

- Create: `internal/auth/types.go` - principal, local user, and configuration types.
- Create: `internal/auth/context.go` - typed context storage and retrieval for principals.
- Create: `internal/auth/service.go` - local user lookup, bcrypt verification, JWT issue and validate operations.
- Create: `internal/auth/middleware.go` - Bearer token extraction and protected-route middleware.
- Create: `internal/auth/service_test.go` - service behavior tests.
- Create: `internal/auth/middleware_test.go` - middleware behavior tests.
- Create: `api/auth/v1/auth.go` - login request/response contracts.
- Create: `api/auth/auth.go` - generated-style auth controller interface.
- Create: `internal/controller/auth/auth.go` - login controller.
- Modify: `main.go` - initialize auth service and bind public/protected route groups.
- Modify: `config.yaml` - non-secret auth defaults and local user metadata only, if needed by the chosen loader.
- Modify: `go.mod`, `go.sum` - direct JWT and bcrypt dependencies.
- Test: `internal/controller/auth/auth_test.go` - login handler success and failure behavior.

## Task 1: Define Authentication Contracts and Red Tests

**Files:**
- Create: `internal/auth/types.go`
- Create: `internal/auth/context.go`
- Create: `internal/auth/service_test.go`
- Create: `internal/auth/middleware_test.go`

**Interfaces:**
- `type Principal struct { UserID, TenantID, Username string; Roles []string }`
- `type LocalUser struct { Username, PasswordHash, UserID, TenantID string; Roles []string }`
- `type Config struct { Issuer, Secret string; AccessTokenTTL time.Duration; Users []LocalUser }`
- `func WithPrincipal(context.Context, Principal) context.Context`
- `func PrincipalFromContext(context.Context) (Principal, bool)`
- `type Service` with `Issue(ctx context.Context, principal Principal) (string, error)`, `Validate(ctx context.Context, token string) (Principal, error)`, and `Login(ctx context.Context, username, password string) (Principal, string, error)`.

- [ ] **Step 1: Write failing service tests.** Cover: valid login returns a token and principal; unknown username is rejected; wrong password is rejected; valid token round-trips principal; expired token is rejected; malformed token is rejected; token signed with another secret is rejected; a token using a different signing algorithm is rejected; issuer mismatch is rejected.
- [ ] **Step 2: Run the focused tests to verify the expected failure.**

Run: `go test ./internal/auth -run 'Test(Service|Principal)' -count=1`

Expected: FAIL because the package and service methods do not exist yet.
- [ ] **Step 3: Add only the type and context contracts.** Do not add JWT implementation before the tests establish the missing behavior.
- [ ] **Step 4: Run the focused tests again.**

Expected: still FAIL on the unimplemented service behavior, confirming the tests exercise the intended contract.

## Task 2: Implement JWT Service and Password Verification

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/auth/service.go`
- Modify: `internal/auth/service_test.go`

**Interfaces:**
- `func NewService(cfg Config) (*Service, error)` validates issuer, secret length, positive TTL, unique usernames, and bcrypt hashes.
- `func (s *Service) Login(ctx context.Context, username, password string) (Principal, string, error)` verifies the configured user and signs a token.
- `func (s *Service) Validate(ctx context.Context, token string) (Principal, error)` validates HS256, issuer, required subject/tenant claims, and time claims.

- [ ] **Step 1: Add direct dependencies.** Use `github.com/golang-jwt/jwt/v5` and `golang.org/x/crypto/bcrypt`; do not hand-roll JWT parsing or password hashing.
- [ ] **Step 2: Implement minimal claims.** Use `jwt.RegisteredClaims` for issuer, subject, issued-at, expiration, and a unique ID. Add `tenant_id`, `username`, and `roles` as private claims.
- [ ] **Step 3: Enforce algorithm and issuer.** The parser must require `jwt.SigningMethodHS256` and compare the configured issuer. Do not accept `alg=none` or a different HMAC/RSA algorithm.
- [ ] **Step 4: Run the focused tests and confirm they pass.**

Run: `go test ./internal/auth -run 'Test(Service|Principal)' -count=1`

Expected: PASS for all service behavior tests.
- [ ] **Step 5: Refactor only after green.** Centralize error values such as `ErrInvalidCredentials`, `ErrInvalidToken`, and `ErrTokenExpired` without changing externally observable behavior.

## Task 3: Implement Bearer Middleware and Login Contracts

**Files:**
- Create: `internal/auth/middleware.go`
- Create: `api/auth/v1/auth.go`
- Create: `api/auth/auth.go`
- Create: `internal/controller/auth/auth.go`
- Create: `internal/auth/middleware_test.go`
- Create: `internal/controller/auth/auth_test.go`

**Interfaces:**
- `func (s *Service) Middleware(r *ghttp.Request)` rejects missing/malformed/invalid Bearer tokens with HTTP 401 and writes a `Principal` into the request context.
- `type LoginReq struct { Username string; Password string }`
- `type LoginRes struct { AccessToken string; TokenType string; ExpiresIn int64; UserID string; TenantID string }`
- `func (c *Controller) Login(ctx context.Context, req *v1.LoginReq) (*v1.LoginRes, error)`.

- [ ] **Step 1: Write failing middleware tests.** Cover missing Authorization, non-Bearer Authorization, invalid token, valid token context injection, and preservation of the original request context values.
- [ ] **Step 2: Write failing controller tests.** Cover valid login, invalid password, and unknown username. Assert that the response does not contain the password or password hash.
- [ ] **Step 3: Run tests to verify they fail for missing middleware/controller behavior.**

Run: `go test ./internal/auth ./internal/controller/auth -count=1`

Expected: FAIL because middleware and controller files are not implemented.
- [ ] **Step 4: Implement Bearer extraction and context injection.** Use a typed context key and the existing `observability.RequestInfo` as a later integration point; do not put raw Authorization values into context or telemetry.
- [ ] **Step 5: Implement the login controller.** Return `token_type=Bearer` and `expires_in` derived from the configured TTL. Map invalid credentials to a stable 401-compatible error.
- [ ] **Step 6: Run focused tests and confirm they pass.**

Run: `go test ./internal/auth ./internal/controller/auth -count=1`

Expected: PASS.

## Task 4: Load Local Configuration Without Committing Secrets

**Files:**
- Modify: `main.go`
- Modify: `config.yaml`
- Create: `internal/auth/config.go`
- Create: `internal/auth/config_test.go`

**Interfaces:**
- `func LoadConfig(ctx context.Context) (Config, error)` reads `SUPERBIZ_JWT_SECRET` and configured local users.
- Local users must provide bcrypt hashes, not plaintext passwords.

- [ ] **Step 1: Write failing loader tests.** Cover missing secret, too-short secret, malformed user configuration, duplicate usernames, and successful loading from an injected environment/config source.
- [ ] **Step 2: Run loader tests to verify failure.**

Run: `go test ./internal/auth -run TestLoadConfig -count=1`

Expected: FAIL because the loader does not exist.
- [ ] **Step 3: Implement configuration loading.** Keep the secret in environment variables. If YAML stores local users, store only username, user ID, tenant ID, roles, and an environment-variable name for the bcrypt hash; never add an example plaintext password.
- [ ] **Step 4: Run focused tests and confirm they pass.**
- [ ] **Step 5: Add startup validation.** `main` must fail fast with a clear error if authentication is enabled but the secret or local user set is invalid.

## Task 5: Bind Public Login and Protected API Routes

**Files:**
- Modify: `main.go`
- Modify: `internal/controller/chat/chat_v1_chat.go`
- Modify: `internal/controller/chat/chat_v1_chat_stream.go`
- Modify: `internal/observability/request.go`
- Modify: `internal/observability/context.go`

**Interfaces:**
- `/api/auth/login` is public.
- `/api/chat`, `/api/chat_stream`, `/api/upload`, and `/api/ai_ops` require a valid JWT.
- Protected handlers obtain identity through `auth.PrincipalFromContext(ctx)` and pass it to the later conversation coordinator.

- [ ] **Step 1: Add route-level integration tests or a handler-level test harness.** Verify login is public, protected routes reject missing tokens, and valid tokens reach the handler with the expected tenant/user identity.
- [ ] **Step 2: Bind separate GoFrame groups.** Apply CORS and response handling to both groups, apply auth middleware only to protected routes, and avoid protecting `/api/auth/login` with itself.
- [ ] **Step 3: Merge the principal into observability request info.** Preserve `request_id` and `conversation_id`; add tenant/user values only as trace/context data, never as high-cardinality metrics labels.
- [ ] **Step 4: Stop trusting `req.Id` as the user identity.** Keep it temporarily as the client conversation ID until the conversation-store phase, but derive user and tenant exclusively from the principal.
- [ ] **Step 5: Run integration tests.**

Run: `go test ./internal/auth ./internal/controller/auth ./internal/controller/chat ./internal/observability -count=1`

Expected: PASS, with existing chat behavior unchanged for authenticated requests.

## Task 6: Verification and Handoff

**Files:**
- Modify: `docs/superpowers/specs/2026-08-20-concurrency-and-context-isolation-design.md` only if implementation decisions materially differ.
- Create: `docs/superpowers/plans/2026-08-25-local-jwt-auth-and-multitenant-roadmap.md` (this plan).

- [ ] **Step 1: Run formatting.** `gofmt -w internal/auth api/auth internal/controller/auth`
- [ ] **Step 2: Run focused authentication tests.** `go test ./internal/auth ./internal/controller/auth -count=1`
- [ ] **Step 3: Run the full test suite.** `go test ./...`
- [ ] **Step 4: Run a build.** `go build ./...`
- [ ] **Step 5: Check for accidental secrets.** Search changed files for `api_key`, `secret_key`, plaintext passwords, and JWT secret literals.
- [ ] **Step 6: Report exact test/build results and clearly list any environment-dependent checks not run.**

## Later Phases (Kept Explicitly in Scope)

These phases are not silently dropped when JWT is implemented:

### Phase 2: Shared Conversation and Tenant Isolation

- [x] Add `SessionKey{TenantID, UserID, ConversationID}` and bind it to the authenticated JWT principal.
- [x] Add an in-process `ConversationStore` and same-session coordinator as a testable adapter.
- [x] Add PostgreSQL `conversations` and `conversation_messages` tables with `(tenant_id, user_id, conversation_id)` ownership. Migration: `deploy/postgres/conversation.sql`.
- [x] Implement `ConversationStore` with transactional versioned append; append the user/assistant pair atomically. Adapter: `conversation.NewSQLStore`; startup selects memory or PostgreSQL from `conversation.backend`.
- [x] Add Redis hot-cache reads with PostgreSQL as the source of truth; invalidate cache only after a successful database commit. Redis failures fall back to PostgreSQL and do not hide a committed write.
- [x] Add optional `idempotency_key` fields to chat requests and reject duplicate or conflicting turn commits in memory and PostgreSQL stores. External-store wiring remains deployment-configured.
- [x] Add tenant filter enforcement to Milvus retrieval and tenant metadata during knowledge indexing.
- [ ] Add tests proving identical document IDs, conversation IDs, and user IDs across tenants never cross boundaries.

### Phase 2B: Retrieval Quality and Tenant Safety

- [x] Change Retriever options to read `tenant_id` from typed request context and require a tenant filter for production collections.
- [x] Choose the current isolation strategy as a mandatory Milvus JSON scalar filter; reject queries that omit the tenant scope. Partition-based isolation remains an optional deployment optimization.
- [ ] Raise the default retrieval `topK` from `1` to a configured value such as `3` or `5`, and add a configurable distance threshold.
- [ ] Preserve retrieval rank and score in metadata and add tests for empty/low-quality recall.
- [ ] Add an intent-aware retrieval decision so greetings, time queries, and pure follow-ups can skip Milvus when no knowledge lookup is needed.
- [ ] Add evaluation cases for cross-tenant retrieval, no-recall behavior, and multi-document answers.

### Phase 3: Runtime Reuse and Tool Policy

- [x] Add a startup-owned `Runtime` that initializes the compiled Agent Graph and its current model, retriever, and tool dependencies once.
- [x] Inject `Runtime` into controllers and remove `BuildChatAgent` from the request path.
- [x] Upgrade the core Eino dependency to `v0.9.15` while retaining the existing `schema.Message`/`compose.Runnable` path; full tests and build pass.
- [x] Add a tested `compose.Runnable` to Eino `adk.Agent` protocol adapter without changing the HTTP controller path.
- [x] Add a tested conversation-scoped `TurnLoopSession` wrapper with Eino `Push`, `Stop`, and same/different-session concurrency guarantees.
- [x] Add a bounded, tenant-aware `TurnLoopRegistry` that lazily reuses one session loop per `SessionKey`, evicts idle sessions without active turns, and stops all loops during shutdown.
- [x] Migrate long-lived conversations to one Eino `adk.TurnLoop` per conversation for queued turns, input merging, cancellation, and checkpoint/resume instead of implementing another turn loop. The current adapter covers per-conversation queuing and shutdown; durable checkpoint/resume remains a later phase.
- [x] Add the repository skill `.agents/skills/eino-architecture-engineer` with official v0.9 compatibility, TurnLoop, tool policy, cancellation, and verification guidance.
- [ ] Add startup health checks and graceful shutdown for Milvus, MCP, and model clients.
- [ ] Check SDK concurrency safety with parallel integration tests; keep request-specific state in local variables/context only.
- [ ] Add a `ToolRegistry` that resolves tools from `Principal`, tenant policy, and request intent; never expose every tool to every user.
- [x] Replace `mysql_crud` model-supplied DSN with a server-side configured data source. Tenant-scoped credential mapping remains a deployment policy step.
- [x] Make database access read-only by default, validate SQL operations, remove terminal confirmation, replace `log.Fatal` with returned errors, and apply query timeouts.
- [ ] Make MCP clients use request context, support cancellation, and apply per-tool timeouts; initialization now uses the build context with a bounded handshake and closes failed clients. Runtime-owned client Close and per-tool policy remain pending.
- [ ] Add an `IntentRouter` with explicit routes for direct chat, RAG, logs/MCP, Prometheus, controlled database queries, and plan/execute tasks.
- [ ] Add a `ModelRouter` for quick/deep/fallback models with per-tenant policy, token budget, timeout, and cost metadata.
- [x] Treat Graph construction errors as fatal errors; stop ignoring `AddLambdaNode`, `AddRetrieverNode`, and `AddEdge` return values.
- [x] Add tests for the database tool's read-only and server-configured-DSN policy. Tool allow/deny registry, model route selection, and broader runtime parallel tests remain pending.

### Phase 4: High-Concurrency and Streaming Governance

- [x] Add an instance-level Agent turn limiter; cluster-level Redis tenant quotas remain a later multi-instance step.
- [x] Add Redis-backed conversation locks with lease, renewal, bounded wait, token-safe release, and cancellation behavior. Queue depth/load testing remains pending.
- [x] Refactor SSE to a single writer with a bounded queue; stop Agent/model/tool work when the client disconnects. The current queue is instance-local and bounded by event count; byte-based limits remain a tuning item.
- [x] Ensure incomplete streams are not committed as assistant history; commit before sending the terminal `done` event.
- [x] Fix the generic response middleware so it cannot append JSON to an SSE response.
- [ ] Add load tests for 100+ independent conversations, slow clients, downstream timeouts, and retry/idempotency behavior.
- [ ] Run `go test -race ./...` in CI or a development image that provides a C compiler; the current Windows host lacks `gcc`.

#### Eino TurnLoop Decision

The dependency is Eino `v0.9.15`. Its `adk.TurnLoop` provides a push-based,
per-turn runtime with input buffering, preemption, and checkpoint/resume. The
HTTP chat path now uses one TurnLoop per authenticated `SessionKey`; the
in-process registry is still only a runtime cache, while durable checkpoint and
transcript storage remain a later phase. The startup `Runtime` owns reusable
model/retriever/tool dependencies. No global loop is shared across users.

### Phase 5: Production Identity and Operations

- [ ] Replace local users with PostgreSQL or enterprise OIDC/JWT verification.
- [ ] Rotate local signing secrets and disable local-password login in production.
- [ ] Add audit logs for login, conversation access, tool authorization, and administrative actions.
- [ ] Add key rotation, token revocation, refresh-token strategy, and account disable behavior.
- [ ] Add dashboards for request latency, model/tool errors, retrieval quality, queue wait, rate-limit rejection, and token/cost usage.
- [ ] Add multi-instance deployment checks proving any instance can serve any authenticated conversation.

## Agent Acceptance Gates

The Agent redesign is complete only when all of the following are demonstrated:

- A user cannot retrieve another tenant's documents even when the query, document ID, and conversation ID are identical.
- A request that lacks an allowed tool receives a policy denial before the model can invoke that tool.
- A database tool cannot receive an arbitrary DSN, cannot execute write operations without an explicit policy, and cannot terminate the server on query failure.
- A client disconnect cancels the active model, retriever, and MCP calls within the configured timeout.
- Two independent conversations can execute concurrently while two turns in the same conversation remain ordered.
- The Agent Graph and its model/retriever/tool dependencies are initialized once per runtime, not once per request.
- A low-quality or empty retrieval result is visible to evaluation and does not become a fabricated document citation.
- All Graph construction errors are returned with the failing node or edge name.
