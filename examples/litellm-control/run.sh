#!/usr/bin/env bash
# Synthetic local-only demonstration; no real provider credentials.
set -Eeuo pipefail
cd "$(dirname "$0")"

API=http://127.0.0.1:4000
AUTH="Authorization: Bearer sk-local-master-key"
TMP=$(mktemp -d)
cleanup() { rm -rf -- "$TMP"; }
trap cleanup EXIT
trap 'echo "Demo failed. Inspect the local fixture; before retry, docker compose down -v deletes its disposable database." >&2' ERR

docker compose up -d --wait --wait-timeout 420

deadline=$((SECONDS + 60))
ready=0
while (( SECONDS < deadline )); do
  remaining=$((deadline - SECONDS))
  if [ "$(curl --connect-timeout 2 --max-time "$remaining" -sS -o /dev/null -w '%{http_code}' "$API/health/liveliness" || true)" = "200" ]; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" != 1 ]; then
  echo "LiteLLM did not become ready within 60s after Compose startup" >&2
  exit 1
fi

request() {
  local method="$1" endpoint="$2" body="$3" output="$4" status
  rm -f -- "$output"
  if ! status=$(curl --connect-timeout 3 --max-time 15 -sS -o "$output" -w '%{http_code}' \
    -X "$method" "$API$endpoint" -H "$AUTH" -H 'Content-Type: application/json' -d "$body"); then
    echo "Transport failure: $method $endpoint" >&2
    return 1
  fi
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    echo "HTTP $status: $method $endpoint" >&2
    return 1
  fi
  echo "$method $endpoint: HTTP $status"
}

request POST /model/new '{"model_name":"fake-route","litellm_params":{"model":"openai/fake-model-a","api_base":"http://fake-upstream:8080/v1","api_key":"fake-upstream-key"},"model_info":{"id":"dep-a"}}' "$TMP/create-a.json"
request POST /model/new '{"model_name":"fake-route","litellm_params":{"model":"openai/fake-model-b","api_base":"http://fake-upstream:8080/v1","api_key":"fake-upstream-key"},"model_info":{"id":"dep-b"}}' "$TMP/create-b.json"

traffic() {
  local phase="$1" expected="$2" i responder a=0 b=0
  for ((i=0; i<32; i++)); do
    request POST /v1/chat/completions '{"model":"fake-route","messages":[{"role":"user","content":"ping"}]}' "$TMP/chat.json" >/dev/null
    responder=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["choices"][0]["message"]["content"])' "$TMP/chat.json")
    case "$responder" in
      served-by:fake-model-a) a=$((a+1)) ;;
      served-by:fake-model-b) b=$((b+1)) ;;
      *) echo "Unexpected responder in $phase" >&2; return 1 ;;
    esac
    if [[ "$expected" == only-b && "$a" != 0 ]]; then
      echo "Blocked deployment responded in $phase" >&2
      return 1
    fi
  done
  printf '%s: dep-a=%d dep-b=%d\n' "$phase" "$a" "$b"
  if [[ "$expected" == both ]]; then
    (( a > 0 && b > 0 ))
  else
    (( a == 0 && b == 32 ))
  fi
}

traffic BEFORE both
request POST /model/block '{"model_id":"dep-a"}' "$TMP/block.json"
traffic BLOCKED only-b
request POST /model/unblock '{"model_id":"dep-a"}' "$TMP/unblock.json"
traffic RESTORED both

if [[ "${CHECK_LIMIT_UPDATE:-0}" == 1 ]]; then
  request PATCH /model/dep-a/update '{"litellm_params":{"rpm":1000,"tpm":1000000}}' "$TMP/update.json"
  echo "PATCH accepted; effective values and routing still require independent verification."
fi

echo "PASS: sampled block/unblock behavior."
echo "Stack left running. Cleanup: docker compose down -v (deletes demo database)."
