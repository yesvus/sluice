# OpenCode managed configuration: source-only pilot contract

Status: proposed runtime contract for #14. No Tezgah deployment, account trial,
or claim of runtime isolation. OpenHands owns the runtime.

- Start in a dedicated operator-controlled Linux worker environment, with the
  pinned OpenCode 2.3.0 binary and a separate operator-provisioned, private
  OpenCode home (0700), distinct from Codex and other users. Do not copy CLI
  login files or pass subscription tokens to sluice. The config file and
  executable paths must be outside writable task worktrees.
- Mount only the exact profile-specific managed JSON, as a read-only
  directory mount at `/etc/opencode`. The same mount must cover
  `opencode acp` and its standalone server child. Do not write to the
  shared Canvas system config or change existing ACP profiles. No alternate
  `opencode.jsonc` is allowed. The runtime must prevent a task from
  replacing the mount, its source, or the launcher; file inspection is not
  a substitute for the runtime isolation boundary.
- For a synthetic profile with model `example/example-b`, the exact file
  contents (including final newline) are:

```json
{
  "model": "example/example-b",
  "small_model": "example/example-b"
}
```

The launcher opens the fixed path with `os.Root`, verifies read-only
filesystem flags on the opened directory and file (Linux only), rejects
alternate config, and compares the bounded file contents to the renderer.
It never writes the managed path or reads credential files. This is a
fail-closed precondition, not proof of task isolation or a runtime
attestation. Explicit child HOME and XDG directories are set from the
profile; parent inline config, tokens, and arbitrary PATH are not inherited.

Upstream pinned sources:
[managed discovery](https://github.com/anomalyco/opencode/blob/v2.3.0/packages/core/src/config/discovery.ts),
[load precedence](https://github.com/anomalyco/opencode/blob/v2.3.0/packages/core/src/config.ts),
[small_model normalization](https://github.com/anomalyco/opencode/blob/v2.3.0/packages/core/src/config/normalize.ts),
[ACP standalone child](https://github.com/anomalyco/opencode/blob/v2.3.0/packages/cli/src/services/standalone.ts).

Acceptance before rollout: review the dedicated runtime with OpenHands and
Secrets, demonstrate mount and home isolation and negative tests, and prove
the precedence and initial reported model using synthetic workspace config
and no real account. A project model or title-agent override must not change
the effective pinned model. The Go fixture verifies the launch preconditions,
not the real OpenCode merge algorithm. No live rollout without separate
approval. Preserve #14 observe-mode scope: dynamic model changes are not
yet an enforcement boundary.
