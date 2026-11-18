# Steering LiteLLM Proxy deployments from sluice

This answers issue #7: how a Go service can take a LiteLLM Proxy deployment out of rotation before it reaches its quota limit and put it back at reset.

All claims below were checked against LiteLLM source at one pinned version. No blog posts or released documentation were used as evidence. Every link is a permalink to the exact commit, so line numbers stay valid.

## Pinned version

- Release tag: `v1.110.0`
- Commit: [`096ff016817c1d26928ef984e5ee16ee285ccd28`](https://github.com/BerriAI/litellm/commit/096ff016817c1d26928ef984e5ee16ee285ccd28)
- Container image used by the proof: `ghcr.io/berriai/litellm:v1.110.0`

References in the rest of this document use this base:

```
https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/
```

## Verdict

| # | Candidate | Works for external runtime control? | Time to effect | Sluice down | Auth required |
|---|-----------|-------------------------------------|----------------|-------------|---------------|
| 1 | Proxy management HTTP API | **Yes**, for DB-stored deployments | Handling instance: same request; peers: next periodic reconcile (default 30s), subject to successful reconciliation | No request-time dependency; persisted blocks remain until explicitly restored (not full fail-open) | Master key / proxy admin key; `PROXY_ADMIN` for pause; Postgres required |
| 2 | Built-in cooldown | **No** external trigger | Only starts when LiteLLM itself sees a failure; default 5s duration | Not applicable, no dependency on sluice | None |
| 3 | Custom callback / pre-call hook | **Yes**, but needs a thin Python shim in the proxy | Next request | Fail open only if the shim catches HTTP errors and returns the unfiltered list; an uncaught error becomes an attempt failure | No LiteLLM management auth for the hook; shim-to-sluice call needs its own token |
| 4 | Config rewrite + reload | Technically yes, unsuitable for steering | YAML edit needs a worker restart (seconds, drops in-flight work); `/config/update` is DB/UI-only | No runtime dependency, but state is stale until reload | `PROXY_ADMIN` and DB for `/config/update`; filesystem access for YAML |

**Recommendation: candidate 1**, the management HTTP API. It keeps decisions in Go, needs no request-time shim, and reconciles the handling proxy instance within the API call. It does not by itself satisfy automatic fail-open restoration of persistent blocks. Details and caveats are in [Recommendation](#recommendation).

## Candidate 1: Proxy management HTTP API

Works: **yes**, with the constraint that deployments must be stored in the database (`store_model_in_db: true`).

### What exists at this version

The management router registers these routes:

- `PATCH /model/{model_id}/update` in [`model_management_endpoints.py#L1677-L1682`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1677-L1682). The handler is `patch_model`.
- `POST /model/block` in [`model_management_endpoints.py#L2064-L2077`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L2064-L2077).
- `POST /model/unblock` in [`model_management_endpoints.py#L2093-L2106`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L2093-L2106).
- `POST /model/new` in [`model_management_endpoints.py#L3114-L3121`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L3114-L3121), used to create DB-stored deployments.
- `POST /model/update` (older PUT-style endpoint) in [`model_management_endpoints.py#L3342-L3355`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L3342-L3355).

`block_model` and `unblock_model` are explicit pause/resume operations. `_set_model_blocked_status` writes `blocked` to the `LiteLLM_ProxyModelTable` row and then calls `clear_cache()` in the same request: [`model_management_endpoints.py#L2013-L2023`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L2013-L2023). The same pattern is used by `patch_model` at [`model_management_endpoints.py#L1914-L1916`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1914-L1916).

The router actually excludes paused deployments. `blocked` is mirrored into `model_info` ([`types/router.py#L907-L909`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L907-L909), [`types/router.py#L317-L318`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L317-L318)), and `is_blocked` checks that flag when selecting a deployment: [`router_rust/surface/health_routing.py#L95-L100`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_rust/surface/health_routing.py#L95-L100).

### Changing rpm, tpm and weight

`PatchModelRequest` extends `updateDeployment` ([`types/proxy/management_endpoints/model_management_endpoints.py#L12-L20`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/proxy/management_endpoints/model_management_endpoints.py#L12-L20)). `updateDeployment` carries `litellm_params`, `model_info` and `blocked`: [`types/router.py#L801-L805`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L801-L805).

- `rpm` and `tpm` are first-class writable fields on `updateLiteLLMParams` via `GenericLiteLLMParams`: [`types/router.py#L594-L595`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L594-L595).
- `weight` is a routing parameter in `LiteLLMParamsTypedDict`: [`types/router.py#L821`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L821). It is not a declared field of `updateLiteLLMParams`, so treat weight updates as best-effort and verify them against `/model/info` for this version; `rpm`, `tpm` and `blocked` are the supported knobs.
- `litellm_params` are merged into the stored row and written back as JSON in `update_db_model`: [`model_management_endpoints.py#L1509-L1521`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1509-L1521), [`model_management_endpoints.py#L1594-L1601`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1594-L1601).

`clear_cache()` reconciles DB models into the live router. It returns early when there is no router or no Prisma client ([`model_management_endpoints.py#L4059-L4061`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L4059-L4061)) and reloads only DB models ([`model_management_endpoints.py#L4134-L4139`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L4134-L4139)). Ordinary deployments are upserted in place rather than deleted first, which keeps the change visible without a serving hole ([`model_management_endpoints.py#L4094-L4103`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L4094-L4103)).

### Time until a change takes effect

`clear_cache()` is awaited inside the control request, so a successful response confirms reconciliation of the handling instance, not every proxy replica. Other replicas can still route to the deployment until their next successful periodic reconciliation; the interval is not a strict upper bound under failures. When `store_model_in_db` is on, a periodic reconcile every `proxy_config_reload_interval_seconds` (default 30 seconds) also pulls DB state into the router: [`proxy_server.py#L11809-L11821`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/proxy_server.py#L11809-L11821), default in [`constants.py#L1992`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/constants.py#L1992).

### Behaviour when sluice is down

LiteLLM continues serving from its last-applied router state and using its own cooldowns without contacting sluice. The persisted `blocked` flag has no demonstrated expiry: if sluice stops after blocking, the deployment stays blocked until a successful explicit unblock. A failed response after the DB update can also leave changed state; reconcile actual state before retrying. The strict fail-open requirement is therefore not met by persistent block/unblock alone. Keep act mode gated until a tested independent recovery/expiry mechanism or the request-time fail-open shim is accepted. Never automatically route a free request to a paid account to recover capacity.

### Auth needed

Every route above declares `dependencies=[Depends(user_api_key_auth)]`. Pausing is additionally restricted to `PROXY_ADMIN`: [`model_management_endpoints.py#L1985-L1991`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1985-L1991) for `/model/block`, and [`model_management_endpoints.py#L1802-L1811`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1802-L1811) for a PATCH that sets `blocked`. Config-based deployments cannot be edited this way; the API returns a validation error telling the caller to store the model in the DB first ([`model_management_endpoints.py#L1998-L2005`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1998-L2005)).

## Candidate 2: LiteLLM's built-in cooldown

Works: **no**, not as an externally triggerable control surface. Cooldown is a failure-reaction mechanism inside the router.

### What exists at this version

Cooldown is applied after a failed deployment attempt. `set_cooldown_deployments` calls `add_deployment_to_cooldown`, passing `time_to_cooldown`: [`router_utils/cooldown_handlers.py#L623-L635`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_utils/cooldown_handlers.py#L623-L635). The duration is resolved from an override, then router settings, then the default: [`router_utils/cooldown_handlers.py#L787-L794`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_utils/cooldown_handlers.py#L787-L794). The default is five seconds: [`constants.py#L160`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/constants.py#L160). A per-deployment override is read from `model_info` or `litellm_params` ([`router_utils/cooldown_handlers.py#L234-L245`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_utils/cooldown_handlers.py#L234-L245), [`types/router.py#L890-L891`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L890-L891)).

The cooldown store itself is in-process or Redis-backed: [`router_utils/cooldown_cache.py#L17-L36`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_utils/cooldown_cache.py#L17-L36).

There is no HTTP route that puts a chosen deployment into cooldown. The management routes are model CRUD, block/unblock and config routes ([`management_endpoints/model_management_endpoints.py#L1606-L1682`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/management_endpoints/model_management_endpoints.py#L1606-L1682)), and the health routes are reads plus a connection test ([`proxy/health_endpoints/_health_endpoints.py#L1557-L1557`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/health_endpoints/_health_endpoints.py#L1557-L1557), [`proxy/health_endpoints/_health_endpoints.py#L2490-L2495`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/health_endpoints/_health_endpoints.py#L2490-L2495)). Cooldown state changes go through Python functions, not an external API. An external process can only influence cooldown indirectly by provoking failures, which is not usable for proactive steering.

### Time until a change takes effect

Not applicable to external control. When LiteLLM does trigger a cooldown itself, it applies on the next routing decision and lasts for the resolved `cooldown_time` (default 5 seconds).

### Behaviour when sluice is down

Not applicable. This mechanism is internal to LiteLLM and is exactly the fallback that must keep working when sluice is unavailable.

### Auth needed

None. There is no external entry point to authenticate against.

## Candidate 3: Custom callback / pre-call hook

Works: **yes**, but it requires a thin Python shim running inside the proxy. All decisions still live in the Go service.

### What exists at this version

`CustomLogger.async_filter_deployments` receives the healthy deployments and returns the list to keep. The base implementation returns the input unchanged: [`integrations/custom_logger.py#L270-L278`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/integrations/custom_logger.py#L270-L278).

The router invokes this hook in a callback filter stage, passing the remaining deployments to each registered callback in turn: [`router_rust/steps/callbacks.py#L466-L484`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_rust/steps/callbacks.py#L466-L484). The stage keeps only the ids that survive the filters ([`router_rust/steps/callbacks.py#L706-L728`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_rust/steps/callbacks.py#L706-L728)), so a shim can drop a deployment from consideration for one request.

A dotted module path under `litellm_settings.callbacks` is resolved to a module-level instance at startup: [`proxy/common_utils/callback_utils.py#L137-L143`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/common_utils/callback_utils.py#L137-L143) and [`proxy/common_utils/callback_utils.py#L385-L401`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/common_utils/callback_utils.py#L385-L401). The proxy treats a resolvable non-`CustomLogger` value as a load error, so the shim must expose a `CustomLogger` instance, not just a class ([`proxy/common_utils/callback_utils.py#L94-L128`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/common_utils/callback_utils.py#L94-L128)).

The shim stays small: one `CustomLogger` subclass that overrides `async_filter_deployments`, calls sluice over HTTP with a short timeout, returns the original deployment objects whose `model_info.id` is permitted by the decision on success (not a list of string IDs), and returns the input list on any error. There is also a newer `RoutingPlugin` protocol whose `RoutingContext.candidate_models` narrows during a plugin pipeline ([`types/router.py#L1469-L1496`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/types/router.py#L1469-L1496)), but the `CustomLogger` hook is the documented load path from config at this version.

### Time until a change takes effect

The next request. The shim reads the decision at request time, so no reload or DB write is involved.

### Behaviour when sluice is down

Fail open is **not** automatic. If the shim's HTTP call raises, the router turns the exception into an attempt failure rather than silently continuing ([`router_rust/steps/callbacks.py#L715-L723`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/router_rust/steps/callbacks.py#L715-L723)). The shim must wrap the call in `try/except` and return `healthy_deployments` unchanged on timeout, connection error, non-2xx response or malformed JSON. With that guard, LiteLLM keeps routing with its own cooldowns.

### Auth needed

LiteLLM needs no management credential for an internal callback. The shim needs its own scoped token to call sluice; keep it in the proxy's environment, not in the config or repository.

## Candidate 4: Rewriting config and reloading

Works: **technically yes, unsuitable for steering**. There is no evidence of a `/config/reload` route in this version's source.

### What exists at this version

Editing `config.yaml` is picked up by uvicorn's `--reload` file watcher, which restarts the worker process. The CLI builds reload directories and include patterns for the config file in [`proxy/proxy_cli.py#L510-L525`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/proxy_cli.py#L510-L525). That is a process restart, not a hot swap.

There is a `POST /config/update` route that writes config sections to the DB and calls `add_deployment` to reconcile: [`proxy_server.py#L20463-L20469`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/proxy_server.py#L20463-L20469) and [`proxy_server.py#L20660-L20662`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/proxy_server.py#L20660-L20662). It is hidden from the schema, intended for the admin UI, and requires `PROXY_ADMIN` ([`proxy_server.py#L20484-L20485`](https://github.com/BerriAI/litellm/blob/096ff016817c1d26928ef984e5ee16ee285ccd28/litellm/proxy/proxy_server.py#L20484-L20485)).

### Time until a change takes effect

A YAML edit plus `--reload` needs a process restart (seconds) and drops in-flight requests. `/config/update` reconciles inside the request, but is a DB/UI path, not a general runtime control surface.

### Behaviour when sluice is down

No runtime dependency, but the deployed configuration is only as fresh as sluice's last successful write. There is no per-request decision point and no interaction with LiteLLM's cooldowns.

### Auth needed

`PROXY_ADMIN` and a DB for `/config/update`; filesystem write access for a YAML edit.

## Recommendation

Use candidate 1, the management HTTP API, for roadmap step 3 (#8 calibrate, #9 act):

1. Register each steerable account deployment with `POST /model/new` and a stable `model_info.id`, with `store_model_in_db: true`.
2. For a soft throttle, `PATCH /model/{id}/update` with `litellm_params: {"rpm": ..., "tpm": ...}`.
3. For a hard pause before a limit, `POST /model/block` with `{"model_id": "..."}`; restore with `POST /model/unblock` at the reported reset only once an independent restoration/reconciliation path is accepted. No automatic expiry is provided.
4. The local proof uses a synthetic master key. A master key is full administrative access, not scoped. Before act mode, establish a dedicated identity and prove the minimum supported authorization for block/unblock/update; the pinned handlers require `PROXY_ADMIN` for block. Do not claim route-level least privilege is available until verified; isolate the management surface and never reuse the master key in production Sluice. Do not pass subscription credentials; only `api_key` and `free_tier` accounts are steerable, `subscription_observed` accounts are never routed to, and a free route must never spill into a paid one.

Why this option: it keeps decision logic in Go, has no request-time dependency on sluice, reconciles the handling instance within the control request, and reuses LiteLLM's cooldowns. The costs are Postgres, replica convergence and persistent stale decisions. Candidate 1 is the research/proof recommendation only; the hard fail-open acceptance gate is unresolved.

Keep candidate 3 as the fallback if a DB-free deployment or per-request decisions become a hard requirement. If that path is taken, the shim must be a tens-of-lines `CustomLogger.async_filter_deployments` override with explicit fail-open error handling, and all decisions must remain in Go.

Do not use candidate 2 as a control surface and do not use candidate 4 for steering.

## Runnable proof

[`examples/litellm-control/`](../examples/litellm-control/) contains a Compose stack with pinned LiteLLM, Postgres and a synthetic OpenAI-compatible fake upstream. `run.sh` is intended to create two DB-stored deployments, assert both appear in baseline traffic, assert only the unblocked deployment responds during the blocked phase, and assert both appear again after restoration. It fails on any required HTTP/transport or response parsing error. The sampling proof is probabilistic, not a proof of zero leakage on all requests. The complete Compose stack has not been executed; block/unblock and optional rpm/tpm PATCH behavior against the pinned running proxy remain unverified. The curl calls in the script are the exact HTTP calls the Go controller will make. No real provider key, token or account identifier is used.
