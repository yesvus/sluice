# ACP worker profile (pilot): `sluice/worker`

Status: source pilot in review; no deployment or real account acceptance.

## Why

Leads start OpenHands workers through ACP. Telling a worker which model to use
does not hold: on 2026-11-18 some OpenCode workers drifted to Claude Haiku and
DeepSeek on opencode-go, because every agent shares every CLI login and picks
its own model. The choice has to be made by the launch path, not the prompt.

## What

sluice adds an ACP front: one ACP agent, `sluice/worker`, that OpenHands
launches as a custom agent command. Behind it sluice starts one of the backends
the profile allows and relays the session.

```
OpenHands ──ACP──> sluice acp --profile worker ──ACP──> codex-acp        (model gpt-6-luna, effort high)
                                               └─ACP──> opencode acp     (model opencode-go/deepseek-v4.1-flash-1017)
```

- ACP on both sides with [coder/acp-go-sdk](https://github.com/coder/acp-go-sdk):
  agent side towards OpenHands, client side towards the backend.
- The backend is chosen once, at `session/new`, and never switched inside a
  session (the conversation state lives in the backend).
- Each backend starts with a pinned model:
  - Codex: `CODEX_HOME` is a per-profile home that the operator provisions
    outside sluice by logging the official CLI in there once. sluice writes
    only `config.toml` (model, effort) into it and never reads, copies or
    moves the credential store, whichever store the operator configured
    (file in that home or the OS keyring).
  - OpenCode: `opencode acp` reads its model from config, not a flag, and
    project config overrides global config. sluice therefore sets `model` and
    `small_model` through the highest-precedence mechanism the pinned OpenCode
    version supports (managed or system config), not a plain global config
    dir. The operator provides a read-only mount in the dedicated runtime;
    sluice verifies it and never writes there. See [managed config contract](acp-managed-config.md).
- After `session/new`, sluice checks the model the backend reports for the
  session against the profile. On a mismatch it ends the session with an
  error instead of relaying it (observe mode logs and ends; it does not fall
  through to another backend).
- After choosing a backend, sluice advertises only that backend's pinned model,
  so the Canvas dropdown advertises only that model. Observe mode does not
  refuse explicit model-change requests, and preserves the reported current
  model in dynamic updates; refusals belong to Act.
- Every session gets one audit line: profile, backend, initial verified model, ACP
  client session ID, start, end, turns, end reason.

## Profile

```yaml
profiles:
  worker:
    backends:              # tried in this order at session start
      - id: codex
        command: [codex-acp]
        home: /var/lib/sluice/homes/worker-codex   # provisioned by operator login
        model: gpt-6-luna
        effort: high
        account_id: codex-main                      # onWatch account key
      - id: opencode-go
        command: [opencode, acp]
        home: /var/lib/sluice/homes/worker-opencode # independent operator-provisioned home
        config: managed                             # precedence mechanism used for the pin
        model: opencode-go/deepseek-v4.1-flash-1017
        account_id: opencode-go-main
    limit_patterns: [usage limit] # synthetic example; confirm provider errors before deployment
    limits:
      max_turns: 0         # 0 = no limit during observe
      max_minutes: 0
```

## Switching

1. **Pilot (static).** Use the first backend whose start succeeds. If a
   backend answers with a usage-limit error at session start, try the next.
   Usage-limit detection matches the explicit JSON-RPC error message only;
   generic SDK errors, error data, invalid requests, authentication and
   cancellation cannot trigger fallback. Real provider message patterns
   remain unverified. Detection is pattern based (ACP has no quota error code; prior
   art: [acp2api](https://github.com/neprel/acp2api) `limitPatterns`).
2. **With onWatch.** Before `session/new`, read headroom and reset time for
   each backend's `account_id` from onWatch and skip a backend that is near its
   limit; prefer it again after its reset. A backend without an `account_id`
   is never skipped on quota grounds. Same signal sluice already uses for
   LiteLLM.

## Scope rules, as they apply here

- No subscription token is read, stored or replayed. Each backend is the
  official CLI with its own login; sluice only starts it and relays ACP.
- Switching happens only between backends the profile lists. It never rotates
  several accounts of one provider to get past that account's limit.
- The caller chooses the tier by choosing the profile. A profile that lists a
  paid backend says so; nothing spills into a backend the profile omits.

## Rollout

1. Observe: relay and audit only, both limits off.
2. Calibrate: compare audit lines with onWatch and provider usage.
3. Act: turn on refusals and limits.

## Open questions

- Where the binary runs: in the Tezgah Pod beside the CLIs, registered as the
  OpenHands custom agent command.
- Until shared logins are removed from the worker runtime (ops #1108, #731), a
  worker can still start a CLI directly; sluice cannot observe or prevent that bypass.
