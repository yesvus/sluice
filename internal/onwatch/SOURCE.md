# onWatch HTTP source contract

Pinned onWatch v2.14.13 at dcc803348ef089068470214bf7556454656b6ace. Synthetic
fixtures only, never deployed onWatch or provider accounts.

- [Routes](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/server.go#L54-L107): /api/providers and /api/current registered; no /health route. Health first tries /health and falls back on 404 to authenticated discovery; reachability only.
- [Discovery](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2361-L2389), [selection](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L1040-L1070): bare current returns first provider; request both when advertised.
- [Combined](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2905-L3000), [Codex accounts](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L604-L711): codexAccounts array or single codex object. Single-provider Codex uses /api/codex/usage?all=true.
- [Anthropic](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L6397-L6468), [Codex](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9350-L9490): utilization is percentage used; per-quota lastUpdatedAt.
- [OpenCode](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/opencode_handlers.go#L193-L287): percentOnly distinguishes placeholder used/limit from actual counters.
- [Antigravity](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9521-L9605), [CLI bucket rows](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9626-L9676): modelId is bucket identity, usagePercent used, resetTime optional. Never use display-card remaining.

Supported providers: anthropic, codex, opencode, antigravity, minimax,
synthetic, zai, openrouter. Other
enabled providers return explicit unsupported errors, not fabricated healthy
samples. Other shapes remain pending #3. Explicit paused/authRequired/isStale
flags and wholly empty quota results fail the observation rather than
manufacturing fresh values. Actual captured timestamps are preserved; no
arbitrary wall-clock freshness cutoff is assumed. A failed poll is not a
quota reset. Consumers must use observation age and the registry before
any later steering. Zero/unknown-limit counter rows are unavailable, not
zero utilization; valid finite sibling counters are preserved.

- [Synthetic](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L3007-L3032), [counter fields](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L3265-L3288), [Z.ai](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L3115-L3200): named quota objects with usage/limit/percent; Z.ai toolCalls is only breakdown, N/A reset is not a time.
- [OpenRouter](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L4012-L4075): credits object, nullable finite limit; no reset timestamp.
- [MiniMax](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L10525-L10650), [account usage endpoint](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L10478-L10501): accounts array, grouped daily/weekly quotas in one list, total/used/usagePercent/resetAt. Do not duplicate weeklyQuotas.
Raw window identities preserved; no routing, credentials, raw response or
account logs, no redirect following.
