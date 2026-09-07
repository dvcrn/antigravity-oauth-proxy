package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const googleAuthTestAdminKey = "test-admin-key"

func TestGoogleAuthHandlers(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", googleAuthTestAdminKey)

	store := &googleAuthTestStore{}
	server := NewServer(store, "test-project", WithGoogleAuth(store))
	server.googleAuth.client = googleAuthHTTPClientFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"access_token":"access-token","refresh_token":"refresh-token","expires_in":3600,"token_type":"Bearer"}`,
			)),
			Header: make(http.Header),
		}, nil
	})

	unauthorized := httptest.NewRequest(http.MethodPost, "/admin/auth/start", nil)
	unauthorizedResponse := httptest.NewRecorder()
	server.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedResponse.Code)
	}

	startResponse := performGoogleAdminRequest(server, http.MethodPost, "/admin/auth/start", "", "")
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}
	var started googleAuthStatus
	if err := json.NewDecoder(startResponse.Body).Decode(&started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if started.Status != "pending" || started.AuthorizationURL == "" {
		t.Errorf("start response = %+v", started)
	}
	var session googleAuthSession
	if err := json.Unmarshal(store.session, &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}

	completeBody := `{"code":"authorization-code#` + session.State + `"}`
	completeResponse := performGoogleAdminRequest(server, http.MethodPost, "/admin/auth/status", completeBody, "application/json")
	if completeResponse.Code != http.StatusOK {
		t.Fatalf("complete status = %d, body = %s", completeResponse.Code, completeResponse.Body.String())
	}
	var completed googleAuthStatus
	if err := json.NewDecoder(completeResponse.Body).Decode(&completed); err != nil {
		t.Fatalf("decode complete response: %v", err)
	}
	if completed.Status != "authenticated" {
		t.Errorf("complete response = %+v", completed)
	}

	statusResponse := performGoogleAdminRequest(server, http.MethodGet, "/admin/status", "", "")
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"configured":true`) {
		t.Errorf("status = %d, body = %s", statusResponse.Code, statusResponse.Body.String())
	}
	if cacheControl := statusResponse.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Errorf("Cache-Control = %q", cacheControl)
	}
}

func TestTokensHandler(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", googleAuthTestAdminKey)

	store := &googleAuthTestStore{}
	server := NewServer(store, "test-project", WithGoogleAuth(store))
	expiresAt := time.Now().Add(time.Hour).UnixMilli()
	body := `{"accessToken":"manual-access","refreshToken":"manual-refresh","expiresAt":` + jsonNumber(expiresAt) + `,"tokenType":"Bearer","scope":"scope","idToken":"id-token"}`
	response := performGoogleAdminRequest(server, http.MethodPost, "/admin/tokens", body, "application/json")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if store.creds == nil || store.creds.AccessToken != "manual-access" || store.creds.RefreshToken != "manual-refresh" || store.creds.ExpiryDate != expiresAt || store.creds.IDToken != "id-token" {
		t.Errorf("stored credentials = %+v", store.creds)
	}
}

func TestTokensHandlerRejectsUnknownFields(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", googleAuthTestAdminKey)

	store := &googleAuthTestStore{}
	server := NewServer(store, "test-project", WithGoogleAuth(store))
	body := `{"accessToken":"manual-access","refreshToken":"manual-refresh","expiresAt":1,"unexpected":true}`
	response := performGoogleAdminRequest(server, http.MethodPost, "/admin/tokens", body, "application/json")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func performGoogleAdminRequest(server http.Handler, method, target, body, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+googleAuthTestAdminKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}
