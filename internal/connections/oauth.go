package connections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// OAuthResult holds the token data returned after a successful OAuth flow.
type OAuthResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// RunOAuthFlow opens the browser to the provider's auth URL, starts a local
// callback server, waits for the redirect, exchanges the code for a token.
// Timeout defaults to 5 minutes if zero.
// progress, if non-nil, is called with (msg, kind) at key steps; kind is "info", "success", or "error".
func RunOAuthFlow(ctx context.Context, cfg OAuthConfig, timeout time.Duration, progress func(msg, kind string)) (*OAuthResult, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	state, err := randomState()
	if err != nil {
		return nil, fmt.Errorf("randomState: %w", err)
	}

	port := cfg.CallbackPort
	if port == 0 {
		port = 9876
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	authURL, err := buildAuthURL(cfg, redirectURI, state)
	if err != nil {
		return nil, fmt.Errorf("buildAuthURL: %w", err)
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if gotState := q.Get("state"); gotState != state {
			errCh <- fmt.Errorf("state mismatch: got %q, want %q", gotState, state)
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		if providerErr := q.Get("error"); providerErr != "" {
			desc := q.Get("error_description")
			if desc != "" {
				errCh <- fmt.Errorf("provider error: %s — %s", providerErr, desc)
			} else {
				errCh <- fmt.Errorf("provider error: %s", providerErr)
			}
			http.Error(w, providerErr, http.StatusBadRequest)
			return
		}

		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("missing code in callback")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h2>&#x2713; Connected! You can close this tab.</h2></body></html>`)
		codeCh <- code
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	fmt.Printf("→ Opening browser: %s\n", authURL)
	fmt.Printf("→ Waiting for authorization on http://localhost:%d/callback\n", port)

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("→ Could not open browser automatically. Please open this URL manually:\n  %s\n", authURL)
	}
	if progress != nil {
		progress("Waiting for you to authorize in the browser…", "info")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
		if progress != nil {
			progress("Code received — requesting token from provider…", "info")
		}
		// success
	case flowErr := <-errCh:
		_ = srv.Shutdown(context.Background())
		return nil, flowErr
	case <-timeoutCtx.Done():
		_ = srv.Shutdown(context.Background())
		return nil, fmt.Errorf("oauth flow timed out after %s", timeout)
	}

	_ = srv.Shutdown(context.Background())

	return exchangeCode(cfg, code, redirectURI)
}

// buildAuthURL builds the authorization URL with all required query params.
func buildAuthURL(cfg OAuthConfig, redirectURI, state string) (string, error) {
	u, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", fmt.Errorf("parse AuthURL: %w", err)
	}

	q := u.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	if len(cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// exchangeCode exchanges an authorization code for tokens via POST to TokenURL.
func exchangeCode(cfg OAuthConfig, code, redirectURI string) (*OAuthResult, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	req, err := http.NewRequest(http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var result OAuthResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	if result.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}

	return &result, nil
}

// randomState generates a cryptographically random base64 state string (16 bytes).
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// openBrowser opens the URL in the default browser using `open` (macOS).
func openBrowser(u string) error {
	return exec.Command("open", u).Start()
}
