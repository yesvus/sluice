# onWatch HTTP source contract

Pinned onWatch v2.14.13 at dcc803348ef089068470214bf7556454656b6ace. Synthetic
fixtures only, never deployed onWatch or provider accounts.

- [Routes](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/server.go#L54-L107): /api/providers and /api/current registered; no /health route. Health first tries /health and falls back on 404 to authenticated discovery; reachability only.
- [Discovery](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2361-L2389), [selection](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L1040-L1070): bare current returns first provider; request both when advertised.
- [Combined](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2905-L3000), [Codex accounts](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L604-L711): codexAccounts array or single codex object. Single-provider Codex uses /api/codex/usage?all=true.
- [Anthropic](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L6397-L6468), [Codex](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9350-L9490): utilization is percentage used; per-quota lastUpdatedAt.
- [OpenCode](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/opencode_handlers.go#L193-L287): percentOnly distinguishes placeholder used/limit from actual counters.
- [Antigravity](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9521-L9605), [CLI bucket rows](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9626-L9676): modelId is bucket identity, usagePercent used, resetTime optional. Never use display-card remaining.

Initial supported providers: anthropic, codex, opencode, antigravity. Other
enabled providers return explicit unsupported errors, not fabricated healthy
samples. Further provider shapes and freshness handling remain pending #3.
Raw window identities preserved; no routing, credentials, raw response or
account logs, no redirect following.
