package antigravity

import (
	"fmt"
	"net/http"
	"runtime"
)

const (
	endpointDaily    = "https://daily-cloudcode-pa.googleapis.com"
	userAgentVersion = "1.1.13"
	RequestUserAgent = "antigravity"
	RequestTypeAgent = "agent"
)

var Endpoints = []string{
	endpointDaily,
}

func platformUserAgent() string {
	return fmt.Sprintf("antigravity/cli/%s (aidev_client; os_type=%s; arch=%s; cl=964361259; auth_method=consumer)", userAgentVersion, runtime.GOOS, runtime.GOARCH)
}

func ApplyHeaders(header http.Header, token string, accept string) {
	header.Set("Authorization", "Bearer "+token)
	header.Set("Content-Type", "application/json")
	header.Set("User-Agent", platformUserAgent())
}
