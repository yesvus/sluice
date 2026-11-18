# sluice

Quota-aware routing for LLM API traffic. sluice sits in front of [LiteLLM Proxy](https://docs.litellm.ai/docs/simple_proxy) and uses [onWatch](https://github.com/onllm-dev/onwatch) quota data to take a deployment out before it hits its limit and to bring it back when its quota resets, instead of reacting only after a 429.

Status: ACP observe-mode pilot in development; no production deployment.

## What it does

- **Routes** OpenAI-compatible requests across API keys, OpenRouter and free tiers, through LiteLLM's tiers, fallbacks and cooldowns.
- **Watches** quota per account through onWatch (`/api/current`, `/metrics`, webhooks), keyed by an `account_id`.
- **Acts early**: cools a deployment when its projected usage nears its own limit, and restores it at the reported reset rather than on a fixed date.
- **Shows** usage, headroom and resets per account in one place.
- **Fronts ACP workers**: one ACP agent per profile that starts an allowed backend agent (Codex, OpenCode) with a pinned model and switches backends at session start by quota. See [`docs/acp-worker.md`](docs/acp-worker.md).

## What it does not do

These are the project's boundary, not a roadmap gap.

- It does not read, store or replay subscription OAuth tokens (Claude Pro/Max, ChatGPT Plus/Pro and similar). Subscriptions stay with their official clients; sluice can display their usage as reported by onWatch, and that is all.
- It does not pool quota across accounts or rotate accounts to get past one account's limit. Each account is used within its own limits; when it is exhausted, traffic to it stops.
- It does not spill a free route into a paid one. The caller chooses the tier.

## Architecture

```
client (OpenAI-compatible)
        |
        v
   LiteLLM Proxy  <-- cooldown / weight updates --  sluice
     |-- API-key providers                            ^
     |-- OpenRouter                                   |  quota, resets, alerts
     `-- free tiers                                   |  per account_id
                                                   onWatch
```

onWatch runs separately, under the operator's own credentials, and is not bundled. sluice only talks to its HTTP API, metrics endpoint and webhooks. Check what onWatch reads for each provider before enabling it: for some providers it uses the vendor client's stored credentials to poll a usage endpoint, and whether that is acceptable is the operator's call under that provider's terms.

## Roadmap

1. **Observe**: ingest onWatch data per `account_id`, export it, no routing effect.
2. **Calibrate**: compare onWatch's view with real 429 and quota errors.
3. **Act**: drive LiteLLM cooldowns and weights from it.

## ACP worker pilot

Build with Go 1.25 using `make build`. The ACP command uses stdin/stdout for protocol traffic and stderr for content-free JSON session audit records. Use an operator-controlled profile file outside the launch working directory; select it with an absolute `--config` or `SLUICE_CONFIG`, defaulting to `/etc/sluice/profiles.yaml`. The launcher rejects workspace-local configs (including resolved symlinks), nonregular files, and group/world-writable files. The runtime must protect the file and its parent directory from task writes; these checks do not establish that boundary.

Synthetic example (not a live account or usable model):

```yaml
profiles:
  worker:
    backends:
      - id: synthetic-codex
        command: [codex-acp]
        home: /private/synthetic-codex
        model: example-a
        effort: high
      - id: synthetic-opencode
        command: [opencode, acp]
        home: /private/synthetic-opencode
        config: managed
        model: example/example-b
    limit_patterns: [usage limit]
    limits:
      max_turns: 0
      max_minutes: 0
```

```sh
./bin/sluice acp --profile worker --config /private/synthetic-profile.yaml
```

OpenCode launch requires a dedicated operator-provisioned Linux runtime with a read-only profile-specific mount at `/etc/opencode` (see [managed config contract](docs/acp-managed-config.md)); otherwise it fails closed. The runtime contract and real CLI precedence are not yet verified on Tezgah. Do not run it against real accounts during verification; `make check` uses fake ACP peers and a synthetic subprocess. Codex launch requires an operator-provisioned dedicated private home, with the official CLI login already provisioned independently. Sluice writes only its owned `config.toml`; do not point it at an existing personal home. Its child environment is explicit rather than inherited, so provider tokens, `CODEX_CONFIG` and `MODEL_PROVIDER` are not copied. The surrounding operator runtime must supply executable/workspace/process isolation and independent credential custody; the launcher itself is not a security boundary. No turn/time limits are enforced in observe mode.

## Contributing

See [`AGENTS.md`](AGENTS.md) for scope rules and process. Issue forms, contribution guidance and the security policy come from [`yesvus/.github`](https://github.com/yesvus/.github).
