# LiteLLM control proof

Runnable proof for [docs/litellm-control.md](../../docs/litellm-control.md). It is intended to demonstrate the recommended mechanism: driving a LiteLLM Proxy deployment out of rotation and back with the management HTTP API.

## What runs

- `litellm`: `ghcr.io/berriai/litellm:v1.110.0`, with `store_model_in_db: true` and a Postgres `DATABASE_URL`.
- `postgres`: local database for DB-stored deployments.
- `fake-upstream`: a synthetic OpenAI-compatible server that echoes `served-by:<model>` so you can see which deployment handled each request. It is not a real provider.

Every key and credential in this stack is a local fixture. Nothing here connects to a real provider or to onWatch.

## Run

Requires a working Docker daemon with a Compose version supporting `up --wait --wait-timeout`, `curl` and `python3`, with loopback port 4000 free. This example uses disposable fixtures and a local admin key. The full pinned proxy path has not yet been executed.

```bash
bash run.sh
```

The script:

1. Starts the stack and waits for health.
2. Creates two DB-stored deployments, `dep-a` and `dep-b`, in model group `fake-route`.
3. Sends 32 chat completions, checks every response, and requires both fake deployments to be observed.
4. Calls `POST /model/block` for `dep-a` and requires all 32 requests to be served by `dep-b`.
5. Calls `POST /model/unblock` and requires both deployments to appear again.
6. Optionally, with `CHECK_LIMIT_UPDATE=1 bash run.sh` on a fresh demo, sends `PATCH /model/{id}/update` with new `rpm`/`tpm` values; HTTP success is not proof of effective limits.

Expected result: both `dep-a` (`fake-model-a`) and `dep-b` (`fake-model-b`) before block, only `dep-b` while blocked, and both again after restore. This is probabilistic sampling, not exhaustive proof, and every required request must succeed. The planned Go controller would use these calls. Runtime behavior remains unverified until the fixture actually passes.

Cleanup or reset before every retry (deletes only this Compose project's disposable demo database; do not use it against other projects):

```bash
docker compose down -v
```
