package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FetchAvailableModelsResponse struct {
	Models              map[string]AvailableModel `json:"models"`
	DefaultAgentModelID string                    `json:"defaultAgentModelId,omitempty"`
}

type AvailableModel struct {
	// DisplayName is upstream's human label. Do not surface it to clients: it is
	// neither unique nor accurate. Four distinct IDs (gemini-2.5-flash,
	// -flash-lite, -flash-thinking, gemini-3.1-flash-lite) all report "Gemini 3.1
	// Flash Lite" and two report "Gemini 3.1 Pro (High)", so clients keying off
	// the label see duplicates; the legacy gemini-2.5-flash* aliases are labelled
	// for a different model entirely; and the *-flash-tiered IDs have no label at
	// all. Report the model ID instead — it is unique and is what callers must
	// pass back to us anyway.
	DisplayName string          `json:"displayName"`
	QuotaInfo   json.RawMessage `json:"quotaInfo,omitempty"`
}

func (c *Client) FetchAvailableModels(ctx context.Context) (*FetchAvailableModelsResponse, error) {
	c.mu.RLock()
	if c.modelsCache != nil && time.Since(c.modelsCacheTime) < time.Hour {
		defer c.mu.RUnlock()
		return c.modelsCache, nil
	}
	c.mu.RUnlock()

	bodyBytes, err := json.Marshal(map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var lastErr error
	for _, endpoint := range Endpoints {
		url := fmt.Sprintf("%s/v1internal:fetchAvailableModels", endpoint)
		resp, err := c.doRequest(ctx, http.MethodPost, url, bodyBytes, "application/json")
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("could not read response body: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("fetchAvailableModels failed with status %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		var result FetchAvailableModelsResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("could not unmarshal response body: %w", err)
		}

		c.mu.Lock()
		c.modelsCache = &result
		c.modelsCacheTime = time.Now()
		c.mu.Unlock()

		return &result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("fetchAvailableModels failed with no endpoints available")
}
