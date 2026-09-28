# CommandCode compatible-gateway integration

This is a first-stage frontend integration, not a native CommandCode provider.

Create an account using **CommandCode (compatible gateway)**. Supply the base URL and API key of an existing OpenAI-compatible adapter. Do not supply a browser cookie or the CommandCode website URL. The account uses the existing OpenAI API-key passthrough backend and stores `extra.provider=commandcode_gateway`. The list displays a CommandCode badge.

Creation and editing require an explicit adapter URL, preserve passthrough, disable WebSocket-v2, and disable account pool mode. Gateway URL validation here is frontend configuration validation, not a backend SSRF security boundary. Existing backend URL policies still apply.

Upstream account selection stays in the adapter. One local account represents one adapter gateway; avoid duplicating upstream pool membership across local gateway accounts. This change does not implement per-CommandCode-account scheduling, quota synchronization, native authentication, or billing reconciliation. The existing backend runtime logic is reused unchanged by this feature; a local HTTP integration test was added. No database, deployment, or live account was modified.

Verified locally on 2026-09-28: frozen-lockfile dependency installation, 25 frontend checks (`node tests/commandcode-gateway.cjs`), frontend typecheck and production build (`pnpm run build`), backend build (`go build ./cmd/server`), existing backend passthrough tests, and JSON/SSE HTTP tests (`go test ./internal/service -run TestCommandCodeGatewayHTTP -count=1`). The HTTP tests use a local TLS mock upstream, not a live CommandCode account. Live service-level streaming and tool-call checks subsequently passed (see acceptance below). Public authentication and database usage persistence remain unverified.

## Live GO gateway acceptance - 2026-09-28

Verified model: `glm-5.3-flash`, through the existing CPA CommandCode GO plugin.
This is a compatible-gateway integration, not a native CommandCode provider in Sub2API.
Use a CPA client API key for the Sub2API account; do not substitute a CommandCode credential JSON or its upstream key.
The GO-specific upstream protocol is handled by the CPA plugin.

Observed through the actual Sub2API `OpenAIGatewayService.Forward` with a loopback TLS bridge:
- JSON text response: completed, `pong`.
- SSE text response: `pong`, terminal `response.completed`.
- JSON and SSE function calls: correct function name, JSON arguments and preserved `call_id`.
- Tool-result continuation: both modes returned exactly `CC_VERIFIED_7319`.
- Input, output and cached token usage reached the Forward result.

Live checks are opt-in: `COMMANDCODE_LIVE_TEST=1`, `CPA_COMMANDCODE_API_KEY`,
`COMMANDCODE_LIVE_BASE_URL`, and `COMMANDCODE_LIVE_MODEL=glm-5.3-flash`.
From `backend`, run `go test ./internal/service -run '^TestCommandCodeGatewayLive(Tools)?$' -v -count=1 -timeout=240s`.
Do not put secrets into tracked files or shell command literals.

The initial 64-token live probe reached `response.incomplete` due to `max_output_tokens`;
512 tokens passed without weakening completion assertions. Model listing alone does not prove plan entitlement.

Not yet verified: public Sub2API authentication/routing, persistent database usage/billing,
admin UI account creation against a configured instance, or a production deployment.
Local TLS bridging does not establish direct production URL policy acceptance.
These passing service tests must not be described as a deployed full-stack E2E result.
