//go:build !js || !wasm

package http

import (
	"net"
	"net/http"
	"time"
)

// NewNoRedirectHTTPClient creates a client that returns redirect responses instead of following them.
func NewNoRedirectHTTPClient() HTTPClient {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// NewHTTPClient creates a new HTTP client for regular environments
func NewHTTPClient() HTTPClient {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			// Leave response compression enabled so net/http sends the same
			// Accept-Encoding: gzip header as Antigravity CLI and transparently
			// decompresses upstream responses.
			DisableCompression: false,
			ForceAttemptHTTP2:  false,
		},
	}
}
