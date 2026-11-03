package server

import (
	"context"
	"errors"
	"time"
)

// mcpSyncCallLimit bounds concurrent sync generation calls. On Workers, /mcp
// bypasses the per-isolate request limiter, so this is the only cap there.
const mcpSyncCallLimit = 5

// mcpSyncCallWait is how long a sync call waits for a free slot. A var so tests can shorten it.
var mcpSyncCallWait = 10 * time.Second

var errMCPBusy = errors.New("proxy busy; retry shortly or use the *_async tool")

// limitMCPCalls runs handler only while holding one of the slots.
func limitMCPCalls[In, Out any](slots chan struct{}, handler func(context.Context, In) (Out, error)) func(context.Context, In) (Out, error) {
	return func(ctx context.Context, in In) (Out, error) {
		var zero Out
		timer := time.NewTimer(mcpSyncCallWait)
		defer timer.Stop()
		select {
		case slots <- struct{}{}:
		case <-timer.C:
			return zero, errMCPBusy
		case <-ctx.Done():
			return zero, ctx.Err()
		}
		defer func() { <-slots }()
		return handler(ctx, in)
	}
}
