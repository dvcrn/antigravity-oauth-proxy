package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	asyncJobCapacity = 10
	// asyncJobTimeout bounds how long a running job holds a slot, because the
	// upstream client has no overall request timeout.
	asyncJobTimeout = 15 * time.Minute
)

// asyncJobPollWait is how long get_async_result waits for a job to finish. It
// stays under client tool timeouts (~60s and 240s). A var so tests can shorten it.
var asyncJobPollWait = 20 * time.Second

var (
	errAsyncJobCapacity = errors.New("too many async jobs in progress; wait for one to finish or use the sync tool")
	errAsyncJobUnknown  = errors.New("unknown or expired job_id; submit the request again")
)

type asyncJobStatus string

const (
	asyncJobRunning   asyncJobStatus = "running"
	asyncJobSucceeded asyncJobStatus = "succeeded"
	asyncJobFailed    asyncJobStatus = "failed"
)

// asyncJob is one background call of a sync MCP tool.
type asyncJob struct {
	id         string
	tool       string
	status     asyncJobStatus
	startedAt  time.Time
	finishedAt time.Time
	done       chan struct{} // closed by finish
	result     map[string]any
	err        error
}

// expired reports whether a running job has outlived asyncJobTimeout.
func (j *asyncJob) expired(now time.Time) bool {
	return j.status == asyncJobRunning && now.Sub(j.startedAt) > asyncJobTimeout
}

// failIfExpired marks an expired running job as failed so the timeout is final
// even if upstream answers later. Callers hold the store lock.
func (j *asyncJob) failIfExpired(now time.Time) {
	if j.expired(now) {
		j.status = asyncJobFailed
		j.finishedAt = now
		j.err = fmt.Errorf("job timed out after %dm", int(asyncJobTimeout.Minutes()))
	}
}

// asyncJobStore keeps at most asyncJobCapacity jobs in memory, shared by all async tools.
type asyncJobStore struct {
	mu   sync.Mutex
	jobs map[string]*asyncJob
}

func newAsyncJobStore() *asyncJobStore {
	return &asyncJobStore{jobs: make(map[string]*asyncJob, asyncJobCapacity)}
}

// start registers a running job, evicting the oldest finished or expired job when full.
func (s *asyncJobStore) start(tool string) (*asyncJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if len(s.jobs) >= asyncJobCapacity {
		var oldest *asyncJob
		for _, job := range s.jobs {
			if job.status == asyncJobRunning && !job.expired(now) {
				continue
			}
			if oldest == nil || job.startedAt.Before(oldest.startedAt) {
				oldest = job
			}
		}
		if oldest == nil {
			return nil, errAsyncJobCapacity
		}
		delete(s.jobs, oldest.id)
	}

	job := &asyncJob{
		id:        uuid.NewString(),
		tool:      tool,
		status:    asyncJobRunning,
		startedAt: now,
		done:      make(chan struct{}),
	}
	s.jobs[job.id] = job
	return job, nil
}

// finish records the outcome of a job. Finishing an evicted job is harmless.
func (s *asyncJobStore) finish(job *asyncJob, result map[string]any, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	defer close(job.done)
	job.failIfExpired(time.Now())
	if job.status != asyncJobRunning {
		return
	}
	job.finishedAt = time.Now()
	if err != nil {
		job.status = asyncJobFailed
		job.err = err
	} else {
		job.status = asyncJobSucceeded
		job.result = result
	}
}

// wait blocks until the job finishes, asyncJobPollWait passes, or ctx is done,
// and returns a snapshot of the job.
func (s *asyncJobStore) wait(ctx context.Context, id string) (asyncJob, bool) {
	s.mu.Lock()
	job, ok := s.jobs[id]
	if ok {
		job.failIfExpired(time.Now())
	}
	var snapshot asyncJob
	if ok {
		snapshot = *job
	}
	s.mu.Unlock()
	if !ok {
		return asyncJob{}, false
	}
	if snapshot.status != asyncJobRunning {
		return snapshot, true
	}

	timer := time.NewTimer(asyncJobPollWait)
	defer timer.Stop()
	select {
	case <-job.done:
	case <-timer.C:
	case <-ctx.Done():
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	job.failIfExpired(time.Now())
	return *job, true
}

type asyncJobSubmitOutput struct {
	JobID   string `json:"job_id"`
	Tool    string `json:"tool"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type getAsyncResultInput struct {
	JobID string `json:"job_id"`
}

// getAsyncResultOutput reports a job's progress. Result is the unchanged output
// of the sync tool named in Tool, nested so its fields cannot collide with the job fields.
type getAsyncResultOutput struct {
	JobID          string         `json:"job_id"`
	Tool           string         `json:"tool"`
	Done           bool           `json:"done"`
	Status         string         `json:"status"`
	ElapsedSeconds int            `json:"elapsed_seconds"`
	Message        string         `json:"message,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
}

// addAsyncMCPTools registers the async variants of the generation tools and
// get_async_result, all sharing one in-memory job store.
func (s *Server) addAsyncMCPTools(srv *mcpsdk.Server) {
	store := newAsyncJobStore()

	addAsyncMCPTool(srv, store, &mcpsdk.Tool{
		Name: "ask_gemini_async",
		Description: "Start an ask_gemini request to the Antigravity (agy) CLI backend in the background and " +
			"return a job_id immediately. Use instead of ask_gemini for slow models or long prompts that might " +
			"exceed the client's tool timeout. Get the answer with get_async_result.",
		InputSchema: askGeminiInputSchema(),
	}, "ask_gemini", validateAskGeminiInput, s.mcpAskGemini)

	addAsyncMCPTool(srv, store, &mcpsdk.Tool{
		Name: "google_search_async",
		Description: "Start a google_search request to the Antigravity (agy) CLI backend in the background and " +
			"return a job_id immediately. Use instead of google_search with slow models that might exceed the " +
			"client's tool timeout. Get the answer with get_async_result.",
		InputSchema: googleSearchInputSchema(),
	}, "google_search", validateGoogleSearchInput, s.mcpGoogleSearch)

	addMCPTool(srv, &mcpsdk.Tool{
		Name: "get_async_result",
		Description: "Get the Antigravity (agy) CLI backend's answer for a job started by an *_async tool. " +
			"Waits up to 20 seconds, then returns done=false if the model is still working. Once finished, " +
			"result holds the same fields the sync tool returns. Jobs live in memory, at most 10 are kept, " +
			"and they are lost when the proxy restarts.",
		InputSchema: mcpObjectSchema(map[string]any{
			"job_id": mcpStringSchema("The job_id returned by an *_async tool."),
		}, "job_id"),
	}, func(ctx context.Context, in getAsyncResultInput) (getAsyncResultOutput, error) {
		return getAsyncResult(ctx, store, in)
	})
}

// addAsyncMCPTool registers a tool that validates its input synchronously, runs
// the sync tool's handler in the background, and returns a job ID for get_async_result.
func addAsyncMCPTool[In, Out any](
	srv *mcpsdk.Server,
	store *asyncJobStore,
	tool *mcpsdk.Tool,
	syncName string,
	validate func(In) error,
	run func(context.Context, In) (Out, error),
) {
	addMCPTool(srv, tool, func(_ context.Context, in In) (asyncJobSubmitOutput, error) {
		if err := validate(in); err != nil {
			return asyncJobSubmitOutput{}, err
		}
		job, err := store.start(syncName)
		if err != nil {
			return asyncJobSubmitOutput{}, err
		}

		// The tool call's ctx is cancelled once the submit response is written, so
		// the job runs on its own context.
		go func() {
			var result map[string]any
			var err error
			defer func() {
				if r := recover(); r != nil {
					logger.Get().Error().Interface("panic", r).Str("tool", syncName).Str("job_id", job.id).Msg("MCP async job panicked")
					result, err = nil, fmt.Errorf("%s panicked: %v", syncName, r)
				}
				store.finish(job, result, err)
			}()

			out, runErr := run(context.Background(), in)
			if runErr != nil {
				err = runErr
				return
			}
			result, err = toJSONObject(out)
		}()

		return asyncJobSubmitOutput{
			JobID:   job.id,
			Tool:    syncName,
			Status:  string(asyncJobRunning),
			Message: "Call get_async_result with this job_id to get the answer.",
		}, nil
	})
}

func getAsyncResult(ctx context.Context, store *asyncJobStore, in getAsyncResultInput) (getAsyncResultOutput, error) {
	job, ok := store.wait(ctx, in.JobID)
	if !ok {
		return getAsyncResultOutput{}, errAsyncJobUnknown
	}
	if job.status == asyncJobFailed {
		return getAsyncResultOutput{}, job.err
	}

	end := job.finishedAt
	if job.status == asyncJobRunning {
		end = time.Now()
	}
	out := getAsyncResultOutput{
		JobID:          job.id,
		Tool:           job.tool,
		Done:           job.status != asyncJobRunning,
		Status:         string(job.status),
		ElapsedSeconds: int(end.Sub(job.startedAt).Seconds()),
		Result:         job.result,
	}
	if !out.Done {
		out.Message = "Still running. Call get_async_result again with the same job_id."
	}
	return out, nil
}

func toJSONObject(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return obj, nil
}
