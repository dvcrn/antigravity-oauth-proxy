package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamGenerateContentRejectsRemovedModels(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-6-thinking", "claude-opus-4-6", "gpt-oss-120b"} {
		for _, action := range []string{"generateContent", "streamGenerateContent"} {
			t.Run(model+"/"+action, func(t *testing.T) {
				t.Parallel()
				req := httptest.NewRequest(http.MethodPost, "/v1beta/models/"+model+":"+action, strings.NewReader(`{}`))
				rec := httptest.NewRecorder()
				(&Server{}).streamGenerateContentHandler(rec, req)
				if rec.Code != http.StatusGone || !strings.Contains(rec.Body.String(), model) {
					t.Fatalf("expected removed-model error, got HTTP %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}
