# LiteLLM control proof

Runnable proof for [docs/litellm-control.md](../../docs/litellm-control.md). It demonstrates the recommended mechanism: driving a LiteLLM Proxy deployment out of rotation and back with the management HTTP API.

## What runs

- `litellm`: `ghcr.io/berriai/litellm:v1.110.0`, with `store_model_in_db: true` and a Postgres `DATABASE_URL`.
- `postgres`: local database for DB-stored deployments.
- `fake-upstream`: a synthetic OpenAI-compatible server that echoes `served-by:<model>` so you can see which deployment handled each request. It is not a real provider.

Every key and credential in this stack is a local fixture. Nothing here connects to a real provider or to onWatch.

## Run

Requires a working Docker daemon with Compose, `curl` and `python3`, with host port 4000 free.

```bash
bash run.sh
```

The script:

1. Starts the stack and waits for health.
2. Creates two DB-stored deployments, `dep-a` and `dep-b`, in model group `fake-route`.
3. Sends six chat completions and counts which deployment served them.
4. Calls `POST /model/block` for `dep-a` and repeats the traffic check.
5. Calls `POST /model/unblock` and repeats the traffic check.
6. Sends one optional `PATCH /model/{id}/update` with new `rpm`/`tpm` values.

A successful run shows both `dep-a` and `dep-b` before the block, only `dep-b` after the block, and both again after the unblock. Those curl calls are the calls the Go controller makes.

Cleanup:

```bash
docker compose down -v
```
