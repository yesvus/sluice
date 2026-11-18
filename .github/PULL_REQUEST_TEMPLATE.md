## What and why

<!-- Summarize the change and motivation. -->

## Related work

<!-- Link an issue if applicable. Use Closes only when this PR completes it. -->

## Verification

<!-- Commands, CI evidence, manual checks, and anything not verified. -->

## Risk and rollout

<!-- Data, security, compatibility, rollout/rollback effects if applicable. Write "None" for trivial changes. -->

## Follow-up

<!-- Outstanding work or decisions, if any. -->

## Checklist

- [ ] No secrets, keys, tokens or account identifiers in the diff, logs or fixtures
- [ ] No subscription OAuth token is read, stored or replayed by sluice
- [ ] Each account stays within its own limits; nothing rotates accounts to get past one account's limit
- [ ] A free route never spills to a paid one
- [ ] Docs updated where behavior changed
