package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/auth"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

var defaultScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

func main() {
	var (
		noBrowser = flag.Bool("no-browser", false, "Don\"t attempt to open a browser; paste code/URL manually")
		verify    = flag.Bool("verify", true, "Verify credentials via loadCodeAssist after saving")
		printRaw  = flag.Bool("print", false, "Print oauth_creds.json to stdout instead of saving")
	)
	flag.Parse()

	logger.Get().Info().Msg("Starting OAuth login flow")

	cfg := auth.Config{
		ClientID:     credentials.OAuthClientID,
		ClientSecret: credentials.OAuthClientSecret,
		RedirectURI:  credentials.OAuthRedirectURI,
		Scopes:       defaultScopes,
	}

	state, err := auth.GenerateState()
	fatalIf(err)

	verifier, challenge, err := auth.GeneratePKCEVerifier()
	fatalIf(err)

	authURL, err := auth.AuthorizationURL(cfg, state, challenge)
	fatalIf(err)

	fmt.Println()
	fmt.Println("Open this URL to authenticate:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()

	var code string
	var gotState string
	fromCallback := false

	if *noBrowser {
		code, gotState = readCodeFromStdin()
	} else {
		tryOpenBrowser(authURL)
		ctx, cancel := auth.DefaultTimeoutContext()
		defer cancel()
		res, err := auth.WaitForCallback(ctx, cfg.RedirectURI)
		if err != nil {
			logger.Get().Warn().Err(err).Msg("Callback server failed; falling back to manual paste mode")
			code, gotState = readCodeFromStdin()
		} else {
			fromCallback = true
			code = res.Code
			gotState = res.State
		}
	}

	if gotState != "" && subtle.ConstantTimeCompare([]byte(gotState), []byte(state)) != 1 {
		if fromCallback {
			logger.Get().Fatal().Str("expected", state).Str("got", gotState).Msg("State mismatch")
		}
		logger.Get().Warn().Str("expected", state).Str("got", gotState).Msg("State mismatch; continuing anyway (manual mode may omit/alter state)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tokens, err := auth.ExchangeCode(ctx, cfg, code, verifier)
	fatalIf(err)
	if tokens.RefreshToken == "" {
		logger.Get().Fatal().Msg("No refresh_token returned; re-run and ensure consent is granted")
	}

	ui, err := auth.FetchUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		logger.Get().Warn().Err(err).Msg("Failed to fetch user info")
	} else if ui.Email != "" {
		logger.Get().Info().Str("email", ui.Email).Msg("Authenticated")
	}

	creds := &credentials.OAuthCredentials{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiryDate:   time.Now().Add(time.Duration(tokens.ExpiresIn)*time.Second).Unix() * 1000,
		TokenType:    tokens.TokenType,
		Scope:        tokens.Scope,
		IDToken:      tokens.IDToken,
	}

	if *printRaw {
		b, err := json.MarshalIndent(creds, "", "  ")
		fatalIf(err)
		fmt.Println(string(b))
		return
	}

	provider, err := credentials.NewFileProvider()
	fatalIf(err)
	fatalIf(provider.SaveCredentials(creds))

	logger.Get().Info().Str("provider", provider.Name()).Msg("Saved credentials")

	if *verify {
		client := antigravity.NewClient(provider)
		_, err := client.LoadCodeAssist()
		fatalIf(err)
		logger.Get().Info().Msg("loadCodeAssist succeeded")
	}
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	logger.Get().Fatal().Err(err).Msg("auth failed")
}

func tryOpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		logger.Get().Warn().Err(err).Msg("Could not open browser; open URL manually")
	}
}

func readCodeFromStdin() (code string, state string) {
	fmt.Println("Paste the full redirect URL or just the authorization code:")
	fmt.Print("> ")
	in := bufio.NewReader(os.Stdin)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
		u, err := url.Parse(line)
		if err != nil {
			return line, ""
		}
		q := u.Query()
		return q.Get("code"), q.Get("state")
	}
	return line, ""
}
