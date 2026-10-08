package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// googleSearchDefaultModel serves google_search when the caller names no model.
const googleSearchDefaultModel = "gemini-3.5-flash-lite"

// groundingRedirectPrefix marks source URLs that Google Search grounding wraps in a redirect.
const groundingRedirectPrefix = "https://vertexaisearch.cloud.google.com/grounding-api-redirect/"

// groundingRedirectTimeout bounds resolving all source redirects of one search.
const groundingRedirectTimeout = 5 * time.Second

// googleSearchInput is the input for the google_search tool.
type googleSearchInput struct {
	Query string `json:"query"`
	Model string `json:"model,omitempty"`
}

type googleSearchSource struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

// googleSearchOutput is the grounded answer of the google_search tool with the
// web sources and search queries Google Search used to produce it.
type googleSearchOutput struct {
	Model   string               `json:"model"`
	Answer  string               `json:"answer"`
	Sources []googleSearchSource `json:"sources,omitempty"`
	Queries []string             `json:"queries,omitempty"`
}

func (s *Server) addGoogleSearchTool(srv *mcpsdk.Server) {
	addMCPTool(srv, &mcpsdk.Tool{
		Name: "google_search",
		Description: "Search the web with Google Search and get a grounded answer with its source URLs. " +
			"Use for current events, recent releases, documentation lookups, or anything that needs " +
			"up-to-date information. Requests are served by the Antigravity (agy) CLI backend with Google " +
			"Search grounding. Each call is one-shot, so phrase the query as a complete question.",
		InputSchema: mcpObjectSchema(map[string]any{
			"query": mcpStringSchema("The question or search request to answer from the web."),
			"model": mcpStringSchema("Optional model ID to answer with. Defaults to " + googleSearchDefaultModel + "."),
		}, "query"),
	}, s.mcpGoogleSearch)
}

func (s *Server) mcpGoogleSearch(ctx context.Context, in googleSearchInput) (googleSearchOutput, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return googleSearchOutput{}, fmt.Errorf("query is required")
	}
	requestedModel := strings.TrimSpace(in.Model)
	if requestedModel == "" {
		requestedModel = googleSearchDefaultModel
	}
	if isRemovedModel(requestedModel) {
		return googleSearchOutput{}, fmt.Errorf("model has been removed from this proxy: %s", requestedModel)
	}

	// CloudCode rejects googleSearch alongside function declarations, so search runs as its own request.
	request := antigravity.GeminiInternalRequest{
		SystemInstruction: &antigravity.SystemInstruction{Parts: []antigravity.ContentPart{{
			Text: "Answer using Google Search results. Be concise and factual, and say when the results do not answer the question.",
		}}},
		Contents: []antigravity.Content{{
			Role:  "user",
			Parts: []antigravity.ContentPart{{Text: query}},
		}},
		Tools: []antigravity.Tool{{GoogleSearch: map[string]interface{}{}}},
	}
	applyModelThinkingDefaults(requestedModel, &request)
	resolvedModel := resolveModelForThinking(requestedModel, request)

	apiCallStart := time.Now()
	resp, err := s.antigravityClient.GenerateContent(&antigravity.GenerateContentRequest{
		Model:   resolvedModel,
		Project: s.projectID,
		Request: request,
	})
	if err != nil {
		logger.Get().Error().
			Err(err).
			Str("model", resolvedModel).
			Dur("api_call_duration", time.Since(apiCallStart)).
			Msg("MCP google_search GenerateContent failed")
		return googleSearchOutput{}, fmt.Errorf("google_search failed for model %q: %w", requestedModel, err)
	}

	servedModel := resp.Model
	if servedModel == "" {
		servedModel = resolvedModel
	}
	answer := extractGeminiText(resp.Response)
	if answer == "" {
		return googleSearchOutput{}, fmt.Errorf("model %q returned no answer", servedModel)
	}
	sources, queries := extractGrounding(resp.Response)
	sources = s.resolveGroundingRedirects(ctx, sources)

	logger.Get().Info().
		Str("model", servedModel).
		Int("query_len", len(query)).
		Int("sources", len(sources)).
		Int("search_queries", len(queries)).
		Dur("api_call_duration", time.Since(apiCallStart)).
		Msg("MCP google_search completed")

	return googleSearchOutput{
		Model:   servedModel,
		Answer:  answer,
		Sources: sources,
		Queries: queries,
	}, nil
}

// extractGrounding returns the deduplicated web sources and search queries from
// the groundingMetadata of an unwrapped CloudCode response.
func extractGrounding(response map[string]interface{}) ([]googleSearchSource, []string) {
	candidates, _ := response["candidates"].([]interface{})
	var sources []googleSearchSource
	var queries []string
	seen := map[string]bool{}
	seenQueries := map[string]bool{}
	for _, candidate := range candidates {
		candidateMap, _ := candidate.(map[string]interface{})
		grounding, _ := candidateMap["groundingMetadata"].(map[string]interface{})
		chunks, _ := grounding["groundingChunks"].([]interface{})
		for _, chunk := range chunks {
			chunkMap, _ := chunk.(map[string]interface{})
			web, _ := chunkMap["web"].(map[string]interface{})
			uri, _ := web["uri"].(string)
			if uri == "" || seen[uri] {
				continue
			}
			seen[uri] = true
			title, _ := web["title"].(string)
			sources = append(sources, googleSearchSource{Title: title, URL: uri})
		}
		rawQueries, _ := grounding["webSearchQueries"].([]interface{})
		for _, q := range rawQueries {
			if qs, ok := q.(string); ok && qs != "" && !seenQueries[qs] {
				seenQueries[qs] = true
				queries = append(queries, qs)
			}
		}
	}
	return sources, queries
}

// resolveGroundingRedirects replaces grounding redirect links with the page URLs they point to,
// keeping the redirect link when it cannot be resolved, and drops sources that resolve to the same page.
func (s *Server) resolveGroundingRedirects(ctx context.Context, sources []googleSearchSource) []googleSearchSource {
	ctx, cancel := context.WithTimeout(ctx, groundingRedirectTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for i := range sources {
		if !strings.HasPrefix(sources[i].URL, groundingRedirectPrefix) {
			continue
		}
		wg.Add(1)
		go func(source *googleSearchSource) {
			defer wg.Done()
			if target := s.redirectTarget(ctx, source.URL); target != "" {
				source.URL = target
			}
		}(&sources[i])
	}
	wg.Wait()

	seen := make(map[string]bool, len(sources))
	resolved := sources[:0]
	for _, source := range sources {
		if !seen[source.URL] {
			seen[source.URL] = true
			resolved = append(resolved, source)
		}
	}
	return resolved
}

func (s *Server) redirectTarget(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return ""
	}
	resp, err := s.redirectClient.Do(req)
	if err != nil {
		logger.Get().Warn().Err(err).Msg("Failed to resolve grounding redirect")
		return ""
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		logger.Get().Warn().Int("status", resp.StatusCode).Msg("Grounding redirect did not return a redirect")
		return ""
	}
	return resp.Header.Get("Location")
}
