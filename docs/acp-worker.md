# ACP worker profile (pilot): `sluice/worker`

Status: design, owner direction 2026-11-18. Nothing runs yet.

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
- Each backend starts with a pinned config sluice writes itself:
  - Codex: isolated `CODEX_HOME` carrying only model and effort settings.
  - OpenCode: own config dir with `model` and `small_model` both set to the
    profile model. `opencode acp` reads the model from config, not a flag.
- The model list sluice advertises to OpenHands is the profile's, so the Canvas
  dropdown shows nothing else. Requests to switch to another model are refused.
- Every session gets one audit line: profile, backend, model, OpenHands
  conversation id, start, end, turns, end reason.

## Profile

```yaml
profiles:
  worker:
    backends:              # tried in this order at session start
      - id: codex
        agent: codex-acp
        model: gpt-6-luna
        effort: high
      - id: opencode-go
        agent: opencode-acp
        model: opencode-go/deepseek-v4.1-flash-1017
    limits:
      max_turns: 0         # 0 = no limit during observe
      max_minutes: 0
```

## Switching

1. **Pilot (static).** Use the first backend whose start succeeds. If a
   backend answers with a usage-limit error at session start, try the next.
   Usage-limit detection is pattern based (ACP has no quota error code; prior
   art: [acp2api](https://github.com/neprel/acp2api) `limitPatterns`).
2. **With onWatch.** Before `session/new`, read each backend's headroom and
   reset time from onWatch and skip a backend that is near its limit; prefer it
   again after its reset. Same signal sluice already uses for LiteLLM.

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
  worker can still start a CLI directly; the audit makes that visible, it does
  not prevent it.
