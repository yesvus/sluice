# sluice

Quota-aware routing for LLM API traffic. sluice sits in front of [LiteLLM Proxy](https://docs.litellm.ai/docs/simple_proxy) and uses [onWatch](https://github.com/onllm-dev/onwatch) quota data to take a deployment out before it hits its limit and to bring it back when its quota resets, instead of reacting only after a 429.

Status: design. Nothing runs yet.

## What it does

- **Routes** OpenAI-compatible requests across API keys, OpenRouter and free tiers, through LiteLLM's tiers, fallbacks and cooldowns.
- **Watches** quota per account through onWatch (`/api/current`, `/metrics`, webhooks), keyed by an `account_id`.
- **Acts early**: cools a deployment when its projected usage nears its own limit, and restores it at the reported reset rather than on a fixed date.
- **Shows** usage, headroom and resets per account in one place.

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

## Contributing

See [`AGENTS.md`](AGENTS.md) for scope rules and process. Issue forms, contribution guidance and the security policy come from [`yesvus/.github`](https://github.com/yesvus/.github).

## License

[Apache-2.0](LICENSE)
