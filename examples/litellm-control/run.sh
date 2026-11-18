#!/usr/bin/env bash
# Demo: steer LiteLLM Proxy deployments through its management HTTP API.
set -euo pipefail
cd "$(dirname "$0")"

API=http://localhost:4000
AUTH="Authorization: Bearer sk-local-master-key"
TMP=/tmp/opencode
mkdir -p "$TMP"

docker compose up -d --wait

ready=0
for _ in $(seq 1 60); do
  if [ "$(curl -sS -o /dev/null -w '%{http_code}' "$API/health/liveliness" || true)" = "200" ]; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" != 1 ]; then
  echo "LiteLLM did not become ready within 60s" >&2
  exit 1
fi
echo "LiteLLM ready"

create_dep() {
  local name="$1" body="$2" status
  status=$(curl -sS -o "$TMP/model-new-$name.json" -w '%{http_code}' \
    -X POST "$API/model/new" -H "$AUTH" -H 'Content-Type: application/json' -d "$body")
  echo "create $name: HTTP $status"
}

create_dep dep-a '{"model_name":"fake-route","litellm_params":{"model":"openai/fake-model-a","api_base":"http://fake-upstream:8080/v1","api_key":"fake-upstream-key"},"model_info":{"id":"dep-a"}}'
create_dep dep-b '{"model_name":"fake-route","litellm_params":{"model":"openai/fake-model-b","api_base":"http://fake-upstream:8080/v1","api_key":"fake-upstream-key"},"model_info":{"id":"dep-b"}}'

traffic() {
  local i
  for i in 1 2 3 4 5 6; do
    curl -sS -X POST "$API/v1/chat/completions" -H "$AUTH" -H 'Content-Type: application/json' \
      -d '{"model":"fake-route","messages":[{"role":"user","content":"ping"}]}' \
      -o "$TMP/chat-$i.json" || true
    python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["choices"][0]["message"]["content"])' \
      "$TMP/chat-$i.json" 2>/dev/null || echo "error"
  done | sort | uniq -c
}

echo "BEFORE"
traffic

echo "block dep-a: HTTP $(curl -sS -o "$TMP/block.json" -w '%{http_code}' -X POST "$API/model/block" \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{"model_id":"dep-a"}')"
echo "AFTER BLOCK"
traffic

echo "unblock dep-a: HTTP $(curl -sS -o "$TMP/unblock.json" -w '%{http_code}' -X POST "$API/model/unblock" \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{"model_id":"dep-a"}')"
echo "AFTER UNBLOCK"
traffic

echo "optional rpm/tpm/weight update for dep-a: HTTP $(curl -sS -o "$TMP/update.json" -w '%{http_code}' \
  -X PATCH "$API/model/dep-a/update" -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"litellm_params":{"rpm":1000,"tpm":1000000}}')"

echo
echo "Each served-by line shows which fake deployment (dep-a or dep-b) handled the request."
echo "These are the same HTTP calls a Go controller makes against the LiteLLM Proxy management API."
echo
echo "Stack left running. Cleanup: docker compose down -v"
