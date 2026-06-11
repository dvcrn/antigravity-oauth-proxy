# Admin API Documentation

This document describes how to set up and use the Admin API for managing OAuth credentials in the Cloudflare Workers deployment of antigravity-oauth-proxy.

## Overview

The Admin API provides secure endpoints for managing OAuth credentials stored in Cloudflare KV. This is essential for the Workers deployment since it doesn't have access to the local filesystem.

## Setup

### 1. Generate Admin API Key

First, generate a secure random key for admin access:

```bash
openssl rand -base64 32
```

Example output:
```
ngtgAHsIo+7UQyAOooFp913u0crzY18qbGIwVImkW54=
```

### 2. Set Admin API Key in Cloudflare Workers

Use Wrangler to securely store the admin key as a secret:

```bash
wrangler secret put ADMIN_API_KEY
```

When prompted, paste the generated key. This will make it available as an environment variable in your Worker.

### 3. Set GCP Project ID (Optional)

If you want to specify a particular GCP project ID instead of using auto-discovery:

```bash
wrangler secret put CLOUDCODE_GCP_PROJECT_ID
```

Enter your GCP project ID when prompted (e.g., `my-project-123`).

## Admin API Endpoints

### Authentication

All admin endpoints require authentication via one of these headers:

- `Authorization: Bearer YOUR_ADMIN_API_KEY`
- `X-API-Key: YOUR_ADMIN_API_KEY`

**Security Note**: The admin API key is required to prevent unauthorized access to credential management endpoints. Keep this key secure and never commit it to version control.

### Available Endpoints

#### POST /admin/credentials

Updates the OAuth credentials stored in Cloudflare KV.

**Request Format**: Accepts the exact same JSON format as `~/.config/antigravity-oauth-proxy/oauth_creds.json`:

```json
{
  "access_token": "ya29.a0AS3H6...",
  "refresh_token": "1//0gzOvnefm8y...",
  "expiry_date": 1752516043000,
  "token_type": "Bearer",
  "scope": "https://www.googleapis.com/auth/cloud-platform ...",
  "id_token": "eyJhbGciOiJSUz..."
}
```

**Example**: Post credentials directly from your local OAuth file:

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

Check the status of stored OAuth credentials.

**Example**:

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

## Complete Setup Workflow

### Step 1: Generate and Configure Admin Key

```bash
# Generate admin key
ADMIN_KEY=$(openssl rand -base64 32)
echo "Generated admin key: $ADMIN_KEY"

# Set it in Wrangler (will prompt for the key)
wrangler secret put ADMIN_API_KEY
```

### Step 2: Upload OAuth Credentials

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

### Step 3: Verify Credentials

```bash
# Check credential status
curl $WORKER_URL/admin/credentials/status \
  -H "Authorization: Bearer $ADMIN_KEY"
```

## Environment Variables

### Required

- **`ADMIN_API_KEY`**: Secure key for admin endpoint access
  - Set via: `wrangler secret put ADMIN_API_KEY`
  - Generate with: `openssl rand -base64 32`

### Optional

- **`CLOUDCODE_GCP_PROJECT_ID`**: Specific GCP project ID to use
  - Set via: `wrangler secret put CLOUDCODE_GCP_PROJECT_ID`
  - If not set, the proxy will auto-discover the project ID

## Error Handling

### Common Error Responses

**401 Unauthorized** - Invalid or missing admin key:
```json
"Unauthorized"
```

**400 Bad Request** - Invalid JSON format:
```json
"Invalid request body"
```

**500 Internal Server Error** - Failed to save credentials:
```json
"Failed to save credentials"
```

### Troubleshooting

1. **"Admin API not configured"**: The `ADMIN_API_KEY` environment variable is not set
   - Solution: Use `wrangler secret put ADMIN_API_KEY` to set it

2. **"Unauthorized"**: Wrong admin key or missing Authorization header
   - Solution: Verify your admin key and header format

3. **"Failed to save credentials"**: Issue with Cloudflare KV storage
   - Solution: Check your KV namespace configuration in `wrangler.toml`

## Security Best Practices

1. **Secure Key Storage**: Never commit admin keys to version control
2. **Key Rotation**: Regularly rotate your admin API key
3. **Access Control**: Only use admin endpoints from secure environments
4. **HTTPS Only**: Always use HTTPS when making admin API calls
5. **Key Management**: Store admin keys securely (password manager, etc.)

## KV Storage Details

The proxy uses Cloudflare KV to store OAuth credentials with these keys:

- **KV Key**: `gemini_cli_oauth_credentials`
- **KV Namespace**: `gemini_code_assist_proxy_kv` (configured in `wrangler.toml`)
- **Data Format**: JSON matching the `oauth_creds.json` structure

The credentials are automatically refreshed when expired, and the updated tokens are saved back to KV storage.
