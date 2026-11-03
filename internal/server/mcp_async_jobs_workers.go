//go:build js && wasm

package server

// asyncJobsEnabled is false on Workers because isolates share no memory and
// work after the response is not guaranteed to run.
const asyncJobsEnabled = false
