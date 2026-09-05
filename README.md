# Antigravity OAuth Proxy

Antigravity OAuth Proxy makes the models available to your Google Antigravity account usable through the Gemini API, an OpenAI-compatible endpoint, or MCP. It handles Google OAuth credentials and translates requests to the internal Cloud Code API used by Antigravity.

```text
  ┌───────────────┐          ┌───────────────────┐          ┌───────────────────────┐
  │ External Tool │          │ Antigravity Proxy │          │ Google Cloud Endpoint │
  │ (OpenCode/etc)│          │ (Local or Worker) │          │      (Cloud Code)     │
  └───────┬───────┘          └─────────┬─────────┘          └───────────┬───────────┘
          │                            │                                │
          │  Standard API request      │    Cloud Code request          │
          │ ─────────────────────────▶ │ ─────────────────────────────▶ │
          │                            │    OAuth access token          │
          │                            │                                │
          │  Standard API response     │    Cloud Code response         │
          │ ◀───────────────────────── │ ◀───────────────────────────── │
          │    JSON or SSE stream      │                                │
          │                            │                                │
          ▼                            ▼                                ▼
```

Use the native Gemini endpoint when your client supports it. The OpenAI-compatible endpoint is available for clients that only speak the OpenAI protocol.

## Quick start

Install the proxy with npm:

```bash
npm install -g antigravity-oauth-proxy
```

Other installation options:

```bash
# mise
mise use -g go:github.com/dvcrn/antigravity-oauth-proxy/cmd/antigravity-oauth-proxy@latest

# Go
go install github.com/dvcrn/antigravity-oauth-proxy/cmd/antigravity-oauth-proxy@latest
```

The OAuth helper currently runs from the source tree. Clone the repository once and complete the browser login:

```bash
git clone https://github.com/dvcrn/antigravity-oauth-proxy.git
cd antigravity-oauth-proxy
go run ./cmd/auth
```

This saves credentials to `~/.config/antigravity-oauth-proxy/oauth_creds.json`. Start the installed proxy with a key of your choice:

```bash
ADMIN_API_KEY="replace-with-a-long-random-value" antigravity-oauth-proxy
```

The server listens on `http://localhost:9878` by default. Test the native Gemini endpoint with:

```bash
curl "http://localhost:9878/v1beta/models/gemini-3-flash:generateContent" \
  -H "X-Goog-Api-Key: replace-with-your-admin-key" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "Say hello in one sentence."}]}]
  }'
```

## Client setup

For Gemini clients, use `http://localhost:9878/v1beta` as the base URL. For OpenAI-compatible clients, use `http://localhost:9878/v1`.

An OpenCode provider using the native Gemini protocol looks like this:

```json
{
  "provider": {
    "antigravity": {
      "npm": "@ai-sdk/google",
      "name": "Antigravity",
      "options": {
        "baseURL": "http://localhost:9878/v1beta",
        "apiKey": "replace-with-your-admin-key"
      },
      "models": {
        "gemini-3-flash": {
          "name": "Gemini 3 Flash (Antigravity)"
        }
      }
    }
  }
}
```

Query `GET /v1/models` to see the models available to the signed-in account.

## Authentication

The OAuth helper opens a Google sign-in flow and saves an access token and refresh token. The proxy reads that file and refreshes expired access tokens automatically.

Set `CLOUDCODE_OAUTH_CREDS_PATH` to use a different credentials file, or provide the complete credentials JSON through `CLOUDCODE_OAUTH_CREDS` in environments without a persistent file. You can set `CLOUDCODE_GCP_PROJECT_ID` when automatic project discovery is unsuitable.

`ADMIN_API_KEY` protects generation, admin, and MCP requests. Clients may send it as a bearer token or an `X-Goog-Api-Key` header. The `/v1/models` endpoint is public.

## Endpoints

| Endpoint | Purpose |
| --- | --- |
| `POST /v1beta/models/{model}:generateContent` | Gemini-compatible non-streaming generation |
| `POST /v1beta/models/{model}:streamGenerateContent` | Gemini-compatible streaming generation |
| `POST /v1/chat/completions` | OpenAI-compatible chat completions |
| `GET /v1/models` | Models available to the signed-in account |
| `POST /mcp` | Stateless MCP server with `ask_gemini` and `ask_gemini_models` |

## MCP clients

```json
{
  "mcpServers": {
    "ask-antigravity": {
      "type": "http",
      "url": "http://localhost:9878/mcp",
      "headers": {
        "Authorization": "Bearer replace-with-your-admin-key"
      }
    }
  }
}
```

The server exposes `ask_gemini(model, prompt)` for one-shot prompts and `ask_gemini_models()` for model discovery.

## Cloudflare Workers

Workers deployments store OAuth credentials in KV. The checked-in `wrangler.toml` contains the maintainer's account and namespace IDs. Remove or replace `account_id`, create a namespace in your account, and replace the `gemini_code_assist_proxy_kv` binding's `id` with the new namespace ID before deploying:

```bash
wrangler kv namespace create antigravity-oauth-proxy-kv
mise run build-worker
wrangler secret put ADMIN_API_KEY
wrangler deploy
```

Generate credentials locally with `go run ./cmd/auth`, then upload the resulting file to the Worker:

```bash
curl -X POST "https://your-worker.example/admin/credentials" \
  -H "Authorization: Bearer replace-with-your-admin-key" \
  -H "Content-Type: application/json" \
  --data-binary @"$HOME/.config/antigravity-oauth-proxy/oauth_creds.json"
```

Check the stored credential status with:

```bash
curl "https://your-worker.example/admin/credentials/status" \
  -H "Authorization: Bearer replace-with-your-admin-key"
```

## Configuration

| Setting | Default | Description |
| --- | --- | --- |
| `ADMIN_API_KEY` | required | Key used to protect generation, admin, and MCP requests |
| `PORT` | `9878` | Listening port for local deployments |
| `CLOUDCODE_OAUTH_CREDS_PATH` | config directory | Path to the OAuth credentials file |
| `CLOUDCODE_OAUTH_CREDS` | unset | OAuth credentials as JSON |
| `CLOUDCODE_GCP_PROJECT_ID` | discovered | Cloud Code project override |
| `DEBUG_SSE` | `false` locally | Log SSE timing details; the checked-in Workers configuration enables it |

## Development

```bash
mise run format
mise run test
mise run build
```
