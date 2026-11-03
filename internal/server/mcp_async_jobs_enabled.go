//go:build !js || !wasm

package server

// asyncJobsEnabled registers the async MCP tools. They need jobs to outlive the
// request in one shared process.
const asyncJobsEnabled = true
