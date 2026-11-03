package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitMCPCallsRejectsWhenSlotsStayFull(t *testing.T) {
	previous := mcpSyncCallWait
	mcpSyncCallWait = 20 * time.Millisecond
	t.Cleanup(func() { mcpSyncCallWait = previous })

	slots := make(chan struct{}, 1)
	release := make(chan struct{})
	call := limitMCPCalls(slots, func(context.Context, string) (string, error) {
		<-release
		return "done", nil
	})

	first := make(chan string)
	go func() {
		out, _ := call(context.Background(), "a")
		first <- out
	}()
	require.Eventually(t, func() bool { return len(slots) == 1 }, time.Second, time.Millisecond)

	_, err := call(context.Background(), "b")
	assert.ErrorIs(t, err, errMCPBusy)

	close(release)
	assert.Equal(t, "done", <-first)
	out, err := call(context.Background(), "c")
	require.NoError(t, err)
	assert.Equal(t, "done", out)
}
