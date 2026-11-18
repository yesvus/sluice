# sluice

Quota-aware routing in front of LiteLLM, fed by onwatch. See `README.md` for scope.

## Scope rules

These are the project's boundary, not style preferences. A change that crosses one is out of scope.

- sluice never reads, stores or replays a subscription OAuth token. Subscriptions are used by their own official clients; sluice only observes their usage through onwatch.
- What sluice routes is API keys, OpenRouter and free tiers, through LiteLLM, and ACP worker sessions across the backends a profile lists (`docs/acp-worker.md`). For ACP, sluice starts the official agent CLI and relays the session; it never touches that CLI's credentials.
- Each account is used within its own limits. Routing never rotates accounts to get past one account's limit.
- A free route never spills to a paid one; the caller chooses the tier.

## Process

- `main` is PR-only. Branch, open a PR, merge after review.
- Address review findings in the branch with a fix and, where one fits, a regression test.
- Use the issue forms inherited from `yesvus/.github`. Do not add a local `.github/ISSUE_TEMPLATE`: it replaces the whole default set.

## Conventions

- No comments unless the why is non-obvious.
- Secrets live in `.env` and `config.local.yaml`, both gitignored. Nothing secret in the repo, the diff, the logs or test fixtures, and no real account identifiers.
