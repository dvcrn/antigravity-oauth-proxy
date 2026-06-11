# Google Antigravity Proxy

Proxy to expose the Antigravity API through standard APIs (Gemini, OpenAI) that you can plug into different tools such as OpenCode or Xcode

```text
  ┌───────────────┐          ┌───────────────────┐          ┌───────────────────────┐
  │ External Tool │          │ Antigravity Proxy │          │ Google Cloud Endpoint │
  │ (OpenCode/etc)│          │ (Local or Worker) │          │      (CloudCode)      │
  └───────┬───────┘          └─────────┬─────────┘          └───────────┬───────────┘
          │                            │                                │
          │  Standard API Request      │    Internal API Request        │
          │ ─────────────────────────▶ │ ─────────────────────────────▶ │
          │ (Gemini or OpenAI format)  │ (Wrapped + OAuth credentials)  │
          │                            │                                │
          │                            │                                │
          │  Standard API Response     │    Internal API Response       │
          │ ◀───────────────────────── │ ◀───────────────────────────── │
          │ (Unwrapped + SSE Stream)   │ (JSON or Internal Stream)      │
          │                            │                                │
          ▼                            ▼                                ▼
```

This proxy exposes Antigravity endpoints through:

- `/v1beta/<model>:streamGenerateContent` for Gemini API compatible clients
- `/v1/chat/completions` for OpenAI API compatible clients (experimental)
- `/v1/models` to get available models

To run locally, or to deploy to Cloudflare Workers

## Installation

Option 1 (recommended): prebuilt binary via npm (macOS, Linux, Windows)

```bash
npm install -g antigravity-oauth-proxy
```

Option 2: install with mise

```bash
mise use -g go:github.com/dvcrn/antigravity-oauth-proxy/cmd/antigravity-oauth-proxy@latest
```

Option 3: install from source with Go

```bash
go install github.com/dvcrn/antigravity-oauth-proxy/cmd/antigravity-oauth-proxy@latest
```

Then to start:

```
ADMIN_API_KEY=123abc antigravity-oauth-proxy
```

## Auth

Run `go run cmd/auth/main.go` and follow instructions. This will create the oauth creds file at `~/.config/antigravity-oauth-proxy/oauth_creds.json`

You can also copy this file from your antigravity installation, but a new OAuth chain is recommended

## Development

```bash
# Clone the repository
git clone <repository-url>
cd antigravity-oauth-proxy

# Build the proxy
mise run build

# Or run directly
mise run run
```

**Note**: For Cloudflare Workers deployment, OAuth credentials are managed via the Admin API instead of environment variables or files.

## Config

Configurable with the following env variables:

- `PORT` (default 9878) - which port to run under
- `ADMIN_API_KEY` - the api key to authenticate against this server

## Usage in other tools

You can use either the native Gemini-supported API at `http://localhost:9878/v1beta`, or the OpenAI transform endpoint at `http://localhost:9878/v1/chat/completions`

Recommended to use the Google / Gemini API when available as it's native to Antigravity

### OpenCode (through Google plugin)

```json
 "antigravity": {
  "npm": "@ai-sdk/google",
  "name": "Antigravity",
  "options": {
    "baseURL": "http://localhost:9878/v1beta",
    "apiKey": "xxxx" # whatever you set as ADMIN_API_KEY
  },
  "models": {
    "gemini-3-flash": {
      "name": "Gemini 3 Flash (Antigravity)"
    },
    "claude-sonnet-4-5": {
      "name": "Claude Sonnet 4.5 (Antigravity)"
    }
  }
},
```

### OpenCode (through OpenAI)

```json
 "antigravity": {
  "name": "Antigravity",
  "options": {
    "baseURL": "http://localhost:9878/v1",
    "apiKey": "xxxx" # whatever you set as ADMIN_API_KEY
  },
  "models": {
    "gemini-3-flash": {
      "name": "Gemini 3 Flash (Antigravity)"
    },
    "claude-sonnet-4-5": {
      "name": "Claude Sonnet 4.5 (Antigravity)"
    }
  }
},
```

### Cloudflare Workers Deployment

For production deployment on Cloudflare Workers:

1. **Create KV namespace** (required for credential storage):

   ```bash
   wrangler kv namespace create "antigravity-oauth-proxy-kv"
   ```

   This will output a namespace ID. Add it to your `wrangler.toml`:

   ```toml
   kv_namespaces = [
     { binding = "gemini_code_assist_proxy_kv", id = "YOUR_NAMESPACE_ID_HERE" }
   ]
   ```

2. **Build for Workers**:

   ```bash
   mise run build-worker
   ```

3. **Deploy to Cloudflare**:

   ```bash
   wrangler deploy
   ```

4. **Set up Admin API Key** (required for credential management):

   ```bash
   # Generate a secure admin key (alphanumeric only, URL-safe)
   head -c 32 /dev/urandom | base64 | tr -d "=+/" | tr -d "\n" | head -c 32

   # Store it as a secret in Workers
   wrangler secret put ADMIN_API_KEY
   ```

5. **Upload OAuth credentials** (see [Admin API](#admin-api) section below)

## Admin API

The Admin API provides secure endpoints for managing OAuth credentials. This is essential for deployments that don't have access to the local filesystem.

### Authentication

All admin endpoints require authentication via one of these methods:

- `Authorization: Bearer YOUR_ADMIN_API_KEY` header
- `key=YOUR_ADMIN_API_KEY` query parameter

**Security Note**: The admin API key prevents unauthorized access to credential management endpoints. Keep this key secure and never commit it to version control.

### Endpoints

#### POST /admin/credentials

Updates OAuth credentials stored in Cloudflare KV. Accepts the exact same JSON format as `~/.config/antigravity-oauth-proxy/oauth_creds.json`:

```bash
curl -X POST https://your-worker.workers.dev/admin/credentials \
  -H "Authorization: Bearer YOUR_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d @~/.config/antigravity-oauth-proxy/oauth_creds.json
```

**Response**:

```json
{
  "success": true,
  "message": "Credentials saved successfully"
}
```

#### GET /admin/credentials/status

Check the status of stored OAuth credentials:

```bash
curl https://your-worker.workers.dev/admin/credentials/status \
  -H "Authorization: Bearer YOUR_ADMIN_API_KEY"
```

**Response**:

```json
{
  "type": "oauth",
  "hasCredentials": true,
  "provider": "CloudflareKVProvider",
  "is_expired": false,
  "expiry_date": 1752516043000,
  "expiry_date_formatted": "2025-07-14T17:53:04Z",
  "has_refresh_token": true
}
```

### Complete Workers Setup Workflow

1. **Generate and set admin key**:

   ```bash
   # Generate admin key (alphanumeric only, URL-safe)
   ADMIN_KEY=$(head -c 32 /dev/urandom | base64 | tr -d "=+/" | tr -d "\n" | head -c 32)
   echo "Generated admin key: $ADMIN_KEY"

   # Set it in Workers
   wrangler secret put ADMIN_API_KEY
   ```

2. **Upload OAuth credentials**:

   ```bash
   # Replace with your actual worker URL and admin key
   WORKER_URL="https://your-worker.workers.dev"
   ADMIN_KEY="YOUR_ADMIN_API_KEY"

   # Upload credentials from local file
   curl -X POST $WORKER_URL/admin/credentials \
     -H "Authorization: Bearer $ADMIN_KEY" \
     -H "Content-Type: application/json" \
     -d @~/.config/antigravity-oauth-proxy/oauth_creds.json
   ```

3. **Verify credentials**:
   ```bash
   # Check credential status
   curl $WORKER_URL/admin/credentials/status \
     -H "Authorization: Bearer $ADMIN_KEY"
   ```

## Core Transformations

### 1. URL Path Rewriting

Transforms standard Gemini API paths to CloudCode's internal format:

- **From:** `/v1beta/models/gemini-3-pro:generateContent`
- **To:** `/v1internal:generateContent`

### Connection Pooling

The proxy maintains persistent HTTP/2 connections to CloudCode:

- Max idle connections: 100
- Max idle connections per host: 10
- Idle connection timeout: 90 seconds

## Troubleshooting

### CloudCode Response Delays

CloudCode can take 7+ seconds to start streaming responses. This is normal behavior from the CloudCode API, not a proxy issue. Enable debug logging to see detailed timing:

```bash
export DEBUG_SSE=true
```

### Authentication Errors

If you receive 401 errors:

1. Check that your OAuth credentials file at `~/.config/antigravity-oauth-proxy/oauth_creds.json` contains valid tokens
2. Refresh your OAuth tokens if they've expired (run `go run cmd/auth/main.go`)

### Debugging

Enable detailed logging:

```bash
export DEBUG_SSE=true  # Show SSE event timing
```

Check logs for:

- Request transformation details
- CloudCode response times
- SSE event delivery timing
