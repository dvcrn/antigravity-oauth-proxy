package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stubAnswerBody = `{"response":{"candidates":[{"content":{"parts":[{"text":"stub answer"}]}}]}}`

// stubUpstream serves generateContent and holds each request until released.
type stubUpstream struct {
	gate      chan struct{}
	closeOnce sync.Once
}

func newStubUpstream(t *testing.T, status int, body string) *stubUpstream {
	t.Helper()
	u := &stubUpstream{gate: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-u.gate
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	previous := antigravity.Endpoints
	antigravity.Endpoints = []string{server.URL}
	// Cleanups run last-in first-out: release blocked requests before closing the server.
	t.Cleanup(server.Close)
	t.Cleanup(func() { antigravity.Endpoints = previous })
	t.Cleanup(u.releaseAll)
	return u
}

func (u *stubUpstream) releaseOne() { u.gate <- struct{}{} }
func (u *stubUpstream) releaseAll() { u.closeOnce.Do(func() { close(u.gate) }) }

func setAsyncPollWait(t *testing.T, d time.Duration) {
	t.Helper()
	previous := asyncJobPollWait
	asyncJobPollWait = d
	t.Cleanup(func() { asyncJobPollWait = previous })
}

func callMCPTool(t *testing.T, srv *Server, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	rec, resp := postMCP(t, srv, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]interface{}{"name": name, "arguments": args},
	}, true)
	require.Equal(t, http.StatusOK, rec.Code)
	result, ok := resp["result"].(map[string]interface{})
	require.True(t, ok, "expected a result object, got %v", resp)
	return result
}

func structuredContent(t *testing.T, result map[string]interface{}) map[string]interface{} {
	t.Helper()
	require.NotEqual(t, true, result["isError"], "unexpected tool error: %s", mcpResultText(t, result))
	content, ok := result["structuredContent"].(map[string]interface{})
	require.True(t, ok, "expected structured content, got %v", result)
	return content
}

func submitAsyncJob(t *testing.T, srv *Server, tool string, args map[string]interface{}) string {
	t.Helper()
	content := structuredContent(t, callMCPTool(t, srv, tool, args))
	assert.Equal(t, "running", content["status"])
	jobID, ok := content["job_id"].(string)
	require.True(t, ok && jobID != "", "expected a job_id, got %v", content)
	return jobID
}

func pollAsyncJob(t *testing.T, srv *Server, jobID string) map[string]interface{} {
	t.Helper()
	return callMCPTool(t, srv, "get_async_result", map[string]interface{}{"job_id": jobID})
}

// waitForAsyncJobs polls until every job has finished, so no job goroutine
// outlives the test and its upstream stub.
func waitForAsyncJobs(t *testing.T, srv *Server, jobIDs ...string) {
	t.Helper()
	setAsyncPollWait(t, 5*time.Second)
	for _, jobID := range jobIDs {
		result := pollAsyncJob(t, srv, jobID)
		if result["isError"] != true {
			require.Equal(t, true, structuredContent(t, result)["done"], "job %s did not finish", jobID)
		}
	}
}

func TestMCPAsyncToolsRoundTrip(t *testing.T) {
	testCases := []struct {
		tool      string
		syncTool  string
		args      map[string]interface{}
		answerKey string
	}{
		{
			tool:      "ask_gemini_async",
			syncTool:  "ask_gemini",
			args:      map[string]interface{}{"model": "gemini-3.5-flash-lite", "prompt": "hello"},
			answerKey: "text",
		},
		{
			tool:      "google_search_async",
			syncTool:  "google_search",
			args:      map[string]interface{}{"query": "hello"},
			answerKey: "answer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.tool, func(t *testing.T) {
			upstream := newStubUpstream(t, http.StatusOK, stubAnswerBody)
			srv := newMCPTestServer(t)

			jobID := submitAsyncJob(t, srv, tc.tool, tc.args)

			setAsyncPollWait(t, 50*time.Millisecond)
			running := structuredContent(t, pollAsyncJob(t, srv, jobID))
			assert.Equal(t, false, running["done"])
			assert.Equal(t, "running", running["status"])
			assert.Equal(t, tc.syncTool, running["tool"])
			assert.Contains(t, running["message"], "Call get_async_result again")
			assert.NotContains(t, running, "result")

			// The poll returns once the job finishes instead of waiting out the window.
			setAsyncPollWait(t, 10*time.Second)
			go func() {
				time.Sleep(100 * time.Millisecond)
				upstream.releaseOne()
			}()
			pollStart := time.Now()
			finished := structuredContent(t, pollAsyncJob(t, srv, jobID))
			assert.Less(t, time.Since(pollStart), 5*time.Second)

			assert.Equal(t, true, finished["done"])
			assert.Equal(t, "succeeded", finished["status"])
			assert.Equal(t, jobID, finished["job_id"])
			result, ok := finished["result"].(map[string]interface{})
			require.True(t, ok, "expected a result object, got %v", finished)
			assert.Equal(t, "stub answer", result[tc.answerKey])
			assert.NotEmpty(t, result["model"])

			// Reading a result does not consume it.
			again := structuredContent(t, pollAsyncJob(t, srv, jobID))
			assert.Equal(t, finished, again)
		})
	}
}

func TestMCPAsyncToolsRejectInvalidInputOnSubmit(t *testing.T) {
	srv := newMCPTestServer(t)

	testCases := []struct {
		tool        string
		args        map[string]interface{}
		wantMessage string
	}{
		{"ask_gemini_async", map[string]interface{}{"model": "gemini-3.1-pro-high", "prompt": "  "}, "prompt is required"},
		{"ask_gemini_async", map[string]interface{}{"model": " ", "prompt": "hello"}, "model is required"},
		{"ask_gemini_async", map[string]interface{}{"model": "claude-sonnet-4-6", "prompt": "hello"}, "model has been removed from this proxy"},
		{"google_search_async", map[string]interface{}{"query": " "}, "query is required"},
		{"google_search_async", map[string]interface{}{"query": "hello", "model": "gpt-oss-120b-medium"}, "model has been removed from this proxy"},
	}

	for _, tc := range testCases {
		t.Run(tc.tool+"/"+tc.wantMessage, func(t *testing.T) {
			result := callMCPTool(t, srv, tc.tool, tc.args)
			assert.Equal(t, true, result["isError"])
			assert.Contains(t, mcpResultText(t, result), tc.wantMessage)
		})
	}
}

func TestMCPAsyncUpstreamErrorSurfacesOnPoll(t *testing.T) {
	upstream := newStubUpstream(t, http.StatusInternalServerError, `{"error":"boom"}`)
	upstream.releaseAll()
	srv := newMCPTestServer(t)
	setAsyncPollWait(t, 5*time.Second)

	jobID := submitAsyncJob(t, srv, "ask_gemini_async", map[string]interface{}{"model": "gemini-3.5-flash-lite", "prompt": "hello"})

	result := pollAsyncJob(t, srv, jobID)
	assert.Equal(t, true, result["isError"])
	assert.Contains(t, mcpResultText(t, result), `ask_gemini failed for model "gemini-3.5-flash-lite"`)
}

func TestMCPGetAsyncResultUnknownJob(t *testing.T) {
	srv := newMCPTestServer(t)

	result := pollAsyncJob(t, srv, "does-not-exist")
	assert.Equal(t, true, result["isError"])
	assert.Contains(t, mcpResultText(t, result), "unknown or expired job_id")
}

func TestMCPAsyncJobCapacity(t *testing.T) {
	upstream := newStubUpstream(t, http.StatusOK, stubAnswerBody)
	srv := newMCPTestServer(t)

	jobIDs := make([]string, 0, asyncJobCapacity)
	for i := 0; i < asyncJobCapacity; i++ {
		if i%2 == 0 {
			jobIDs = append(jobIDs, submitAsyncJob(t, srv, "ask_gemini_async", map[string]interface{}{"model": "gemini-3.5-flash-lite", "prompt": "hello"}))
		} else {
			jobIDs = append(jobIDs, submitAsyncJob(t, srv, "google_search_async", map[string]interface{}{"query": "hello"}))
		}
	}

	rejected := callMCPTool(t, srv, "google_search_async", map[string]interface{}{"query": "hello"})
	assert.Equal(t, true, rejected["isError"])
	assert.Contains(t, mcpResultText(t, rejected), "too many async jobs in progress")

	upstream.releaseOne()
	setAsyncPollWait(t, 10*time.Millisecond)
	var finishedID string
	deadline := time.Now().Add(5 * time.Second)
	for finishedID == "" && time.Now().Before(deadline) {
		for _, jobID := range jobIDs {
			if structuredContent(t, pollAsyncJob(t, srv, jobID))["done"] == true {
				finishedID = jobID
				break
			}
		}
	}
	require.NotEmpty(t, finishedID, "no job finished after releasing one upstream request")

	newJobID := submitAsyncJob(t, srv, "ask_gemini_async", map[string]interface{}{"model": "gemini-3.5-flash-lite", "prompt": "hello"})
	evicted := pollAsyncJob(t, srv, finishedID)
	assert.Equal(t, true, evicted["isError"])
	assert.Contains(t, mcpResultText(t, evicted), "unknown or expired job_id")

	upstream.releaseAll()
	remaining := []string{newJobID}
	for _, jobID := range jobIDs {
		if jobID != finishedID {
			remaining = append(remaining, jobID)
		}
	}
	waitForAsyncJobs(t, srv, remaining...)
}

func TestAsyncJobStoreTimedOutJobIsFailedAndEvictable(t *testing.T) {
	setAsyncPollWait(t, time.Second)
	store := newAsyncJobStore()

	stale, err := store.start("ask_gemini")
	require.NoError(t, err)
	store.mu.Lock()
	stale.startedAt = time.Now().Add(-asyncJobTimeout - time.Minute)
	store.mu.Unlock()

	waitStart := time.Now()
	job, ok := store.wait(context.Background(), stale.id)
	require.True(t, ok)
	assert.Equal(t, asyncJobFailed, job.status)
	assert.EqualError(t, job.err, "job timed out after 15m")
	assert.Less(t, time.Since(waitStart), asyncJobPollWait, "an expired job should not wait out the poll window")

	// A late upstream answer does not overturn the timeout.
	store.finish(stale, map[string]any{"text": "late"}, nil)
	job, ok = store.wait(context.Background(), stale.id)
	require.True(t, ok)
	assert.Equal(t, asyncJobFailed, job.status)
	assert.Nil(t, job.result)

	for i := 1; i < asyncJobCapacity; i++ {
		_, err := store.start("ask_gemini")
		require.NoError(t, err)
	}
	_, err = store.start("ask_gemini")
	require.NoError(t, err, "a timed-out job should be evicted to make room")
	_, ok = store.wait(context.Background(), stale.id)
	assert.False(t, ok)

	_, err = store.start("ask_gemini")
	assert.ErrorIs(t, err, errAsyncJobCapacity)
}

func TestAsyncJobStoreWaitReturnsOnCancel(t *testing.T) {
	setAsyncPollWait(t, time.Minute)
	store := newAsyncJobStore()
	job, err := store.start("ask_gemini")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot, ok := store.wait(ctx, job.id)
	require.True(t, ok)
	assert.Equal(t, asyncJobRunning, snapshot.status)
}

func TestAsyncMCPToolRecoversPanic(t *testing.T) {
	setAsyncPollWait(t, 5*time.Second)
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	store := newAsyncJobStore()
	addAsyncMCPTool(srv, store, &mcpsdk.Tool{Name: "boom_async", InputSchema: mcpObjectSchema(map[string]any{})}, "boom",
		func(struct{}) error { return nil },
		func(context.Context, struct{}) (struct{}, error) { panic("kaboom") },
	)

	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	_, err := srv.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	session, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	submit, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "boom_async", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, submit.IsError)
	jobID := submit.StructuredContent.(map[string]any)["job_id"].(string)

	job, ok := store.wait(ctx, jobID)
	require.True(t, ok)
	assert.Equal(t, asyncJobFailed, job.status)
	assert.ErrorContains(t, job.err, "boom panicked: kaboom")
}

func TestAsyncJobStoreEvictsReadJobsFirst(t *testing.T) {
	setAsyncPollWait(t, 10*time.Millisecond)
	store := newAsyncJobStore()

	unread, err := store.start("ask_gemini")
	require.NoError(t, err)
	read, err := store.start("ask_gemini")
	require.NoError(t, err)
	for i := 2; i < asyncJobCapacity; i++ {
		_, err := store.start("ask_gemini")
		require.NoError(t, err)
	}

	// unread finished earlier, so oldest-first alone would evict it.
	store.finish(unread, map[string]any{"text": "unread"}, nil)
	store.finish(read, map[string]any{"text": "read"}, nil)
	_, ok := store.wait(context.Background(), read.id)
	require.True(t, ok)

	_, err = store.start("ask_gemini")
	require.NoError(t, err)
	_, ok = store.wait(context.Background(), read.id)
	assert.False(t, ok, "the read job should be evicted")
	job, ok := store.wait(context.Background(), unread.id)
	require.True(t, ok, "the unread job should be kept")
	assert.Equal(t, "unread", job.result["text"])
}
