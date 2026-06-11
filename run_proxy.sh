#!/bin/bash

set -euo pipefail

# LaunchAgent-friendly entrypoint.
# Prefer a pre-built binary for faster startup and fewer deps.
if [ -x "./antigravity-oauth-proxy" ]; then
  exec ./antigravity-oauth-proxy
fi

# Fallback for dev setups.
if command -v mise >/dev/null 2>&1; then
  exec mise run run
fi

echo "Error: missing ./antigravity-oauth-proxy binary and mise is not installed" >&2
exit 1
