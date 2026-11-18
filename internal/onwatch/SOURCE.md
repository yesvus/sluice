# onWatch HTTP source contract

Pinned onWatch v2.14.13 at dcc803348ef089068470214bf7556454656b6ace. Synthetic
fixtures only, never deployed onWatch or provider accounts.

- [Routes](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/server.go#L54-L107): /api/providers and /api/current registered; no /health route. Health first tries /health and falls back on 404 or 302 to authenticated discovery; reachability only. [Auth middleware](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/middleware.go#L280-L325) accepts Basic Auth only for API paths and redirects unauthenticated browser paths to login. Never follow redirects; discovery must independently succeed.
- [Discovery](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2361-L2389), [selection](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L1040-L1070): bare current returns first provider; request both when advertised.
- [Combined](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2905-L3000), [Codex accounts](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L604-L711): codexAccounts array or single codex object. Single-provider Codex uses /api/codex/usage?all=true.
- [Anthropic](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L6397-L6468), [Codex](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9350-L9490): utilization is percentage used; per-quota lastUpdatedAt.
- [OpenCode](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/opencode_handlers.go#L193-L287): percentOnly distinguishes placeholder used/limit from actual counters.
- [Antigravity](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9521-L9605), [CLI bucket rows](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L9626-L9676): modelId is bucket identity, usagePercent used, resetTime optional. Never use display-card remaining.

Supported providers: anthropic, codex, opencode, antigravity, minimax,
synthetic, zai, openrouter, gemini, copilot, cursor, ollama, commandcode,
muse, mistral. Other
enabled providers return explicit unsupported errors, not fabricated healthy
samples. Unsupported shapes are listed below. Explicit paused/authRequired/isStale
flags and wholly empty quota results fail the observation rather than
manufacturing fresh values. Actual captured timestamps are preserved; no
arbitrary default wall-clock freshness cutoff is assumed. WithMaxAge lets the
caller apply a positive observation-age limit and reject future timestamps. A failed poll is not a
quota reset. Consumers must use observation age and the registry before
any later steering. Zero/unknown-limit counter rows are unavailable, not
zero utilization; valid finite sibling counters are preserved.

- [Synthetic](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L3007-L3032), [counter fields](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L3265-L3288), [Z.ai](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L3115-L3200): named quota objects with usage/limit/percent; Z.ai toolCalls is only breakdown, N/A reset is not a time.
- [OpenRouter](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L4012-L4075): credits object, nullable finite limit; no reset timestamp.
- [MiniMax](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L10525-L10650), [account usage endpoint](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L10478-L10501): accounts array, grouped daily/weekly quotas in one list, total/used/usagePercent/resetAt. Do not duplicate weeklyQuotas.
Raw window identities preserved; no routing, credentials, raw response or
account logs, no redirect following.

## Additional provider shapes

- [Gemini](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/gemini_handlers.go#L19-L74): family modelId, usagePercent, resetTime. Do not mix with AGY buckets.
- [Copilot](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L8806-L8860): entitlement minus remaining yields finite used; skip unlimited/non-finite rows, use resetDate.
- [Cursor](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/cursor_handlers.go#L191-L280), [Ollama](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/ollama_handlers.go#L177-L272), [Command Code](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/commandcode_handlers.go#L174-L265), [Muse](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/muse_handlers.go#L198-L285), [Mistral](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/mistral_handlers.go#L16-L69): name/utilization/resetsAt, format-specific absolute counters only when finite. Percent rows remain utilization-only; unknown cap or unlimited is not zero usage. Muse cliActive and Mistral reconnect/stale/waiting are unavailable.

Unsupported by the Snapshot contract at this pin: [DeepSeek](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/deepseek_handlers.go#L18-L79) and [Moonshot](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/moonshot_handlers.go#L17-L70) expose balances without a finite usage denominator; [Grok](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/handlers.go#L2570-L2657) and [Kimi](https://github.com/onllm-dev/onWatch/blob/dcc803348ef089068470214bf7556454656b6ace/internal/web/kimi_handlers.go#L42-L120) initialize capturedAt to response build time and do not replace it with the stored snapshot timestamp. Do not fake utilization or freshness. Unknown providers fail explicitly by default. Use WithProviders to select a nonempty list of supported provider IDs when the daemon also serves unsupported or intentionally excluded providers. Every selected provider must appear in discovery and produce an observation; partial or empty results fail rather than dropping a required provider. Combined responses are filtered only when selection is explicit; individual requests are not used to bypass missing combined telemetry. Health checks remain reachability only. Discovery reflects dashboard visibility, and combined telemetry has separate visibility flags; single-provider account-list endpoints do not attest polling state. This is observation, not authorization to steer. Registry, required per-account/window coverage and consumer freshness policy must be reviewed before act-stage use.
