# Antigravity OAuth Proxy

Antigravity OAuth Proxy makes the models available to your Google Antigravity account usable through the Gemini API, an OpenAI-compatible endpoint, or MCP. It handles Google OAuth credentials and translates requests to the internal Cloud Code API used by Antigravity.

The MCP server at `/mcp` gives agents tools to discover models available to your Antigravity account and send one-shot prompts without configuring a Gemini or OpenAI API client.

```text
  ┌───────────────┐          ┌───────────────────┐          ┌───────────────────────┐
  │ External Tool │          │ Antigravity Proxy │          │ Google Cloud Endpoint │
  │ (OpenCode/etc)│          │ (Local or Worker) │          │      (Cloud Code)     │
  └───────┬───────┘          └─────────┬─────────┘          └───────────┬───────────┘
          │                            │                                │
          │  API or MCP request        │    Cloud Code request          │
          │ ─────────────────────────▶ │ ─────────────────────────────▶ │
          │                            │    OAuth access token          │
          │                            │                                │
          │  API or MCP response       │    Cloud Code response         │
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
| `POST /admin/auth/start` | Start Workers Google authorization |
| `GET /admin/auth/status` | Read the Workers authorization state |
| `POST /admin/auth/status` | Exchange an authorization code and store tokens |
| `POST /admin/tokens` | Store OAuth tokens manually in Workers KV |
| `GET /admin/status` | Check whether Workers credentials are configured |

## MCP clients

The `/mcp` endpoint uses stateless streamable HTTP with JSON responses. It keeps no conversation or session state between calls.

MCP requests use the proxy's Google OAuth credentials upstream and the same `ADMIN_API_KEY` as the generation endpoints. No separate Gemini API key is required.

MCP configuration varies by client. Configure a streamable HTTP server with:

| Setting | Value |
| --- | --- |
| URL | `http://localhost:9878/mcp` |
| Header | `Authorization: Bearer replace-with-your-admin-key` |

For clients that use an `mcpServers` JSON object:

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

The client discovers these tools after it connects:

| Tool | Input | Result |
| --- | --- | --- |
| `ask_gemini_models` | None | The default model and available model IDs |
| `ask_gemini` | `model`, `prompt` | The requested model, model that served the request, and response text |

Call `ask_gemini_models` first when the model ID is not already known. Its results reflect the models currently available to the signed-in Antigravity account.

`ask_gemini` is one-shot. It does not retain conversation history, so `prompt` must include all context needed for that call. The returned `model` may differ from `requested_model` when the proxy resolves a model variant or falls back after an upstream 404.

## Cloudflare Workers

Workers deployments store OAuth credentials in KV and use a [Workers VPC](https://developers.cloudflare.com/workers-vpc/) tunnel for provider egress. Install Go 1.25 or newer, [mise](https://mise.jdx.dev/), and Wrangler 4, then run `mise install` and `wrangler login`. The account, namespace, and tunnel IDs checked into `wrangler.toml` belong to the maintainer deployment and must be replaced for another Cloudflare account.

1. Create a tunnel in **Cloudflare Dashboard > Workers VPC > Tunnels** and run `cloudflared` on a machine with normal Internet access.
2. Create a KV namespace with `wrangler kv namespace create ANTIGRAVITY_OAUTH_PROXY_KV`.
3. Set your account ID, the returned KV ID, and the tunnel UUID in `wrangler.toml`:

   ```toml
   account_id = "<ACCOUNT_ID>"
   kv_namespaces = [
     { binding = "ANTIGRAVITY_AUTH", id = "<KV_NAMESPACE_ID>" }
   ]
   vpc_networks = [
     { binding = "ANTIGRAVITY_EGRESS", tunnel_id = "<TUNNEL_ID>", remote = true }
   ]
   ```

4. Deploy and set the client-facing key at Wrangler's secure prompt:

   ```bash
   wrangler deploy
   wrangler secret put ADMIN_API_KEY
   ```

5. Optional: add a [Workers Custom Domain](https://developers.cloudflare.com/workers/configuration/routing/custom-domains/) to `wrangler.toml`. Do not create a CNAME to `workers.dev`:

   ```toml
   routes = [
     { pattern = "antigravity.example.com", custom_domain = true }
   ]
   ```

   Run `wrangler deploy` again after adding the route. See Cloudflare's [tunnel setup](https://developers.cloudflare.com/workers-vpc/configuration/tunnel/) and [VPC Networks guide](https://developers.cloudflare.com/workers-vpc/configuration/vpc-networks/).

### Authorize Google on Workers

All admin requests require `Authorization: Bearer <ADMIN_API_KEY>`, `X-API-Key: <ADMIN_API_KEY>`, or `X-Goog-Api-Key: <ADMIN_API_KEY>`.

```bash
BASE_URL="https://antigravity-oauth-proxy.<SUBDOMAIN>.workers.dev"

curl -X POST "$BASE_URL/admin/auth/start" \
  -H "Authorization: Bearer $ADMIN_API_KEY"
```

Open the returned `authorizationUrl` and approve access. The browser redirects to `localhost`; if no callback server is running, copy the final URL from the address bar and submit it as `code`:

```bash
curl -X POST "$BASE_URL/admin/auth/status" \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"code":"http://localhost:51121/oauth-callback?code=...&state=..."}'

curl "$BASE_URL/admin/status" \
  -H "Authorization: Bearer $ADMIN_API_KEY"
```

`POST /admin/tokens` supports manual setup with `accessToken`, `refreshToken`, `expiresAt` in Unix milliseconds, and optional `tokenType`, `scope`, and `idToken`. Existing `/admin/credentials` endpoints remain available for compatibility. Access tokens are refreshed five minutes before expiry or after an upstream 401 and saved back to KV.

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
