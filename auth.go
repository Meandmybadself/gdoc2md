package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	docsv1 "google.golang.org/api/docs/v1"
)

const (
	configDir  = ".gdoc2md"
	configFile = "config.json"
	tokenFile  = "token.json"
)

// AppConfig holds user-supplied OAuth2 client credentials.
type AppConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func configDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir), nil
}

// LoadAppConfig reads client credentials from ~/.gdoc2md/config.json.
func LoadAppConfig() (*AppConfig, error) {
	dir, err := configDirPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		return nil, fmt.Errorf("no credentials found — run 'gdoc2md configure' first")
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config file: %w", err)
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("config missing client_id or client_secret")
	}
	return &cfg, nil
}

// SaveAppConfig writes client credentials to ~/.gdoc2md/config.json.
func SaveAppConfig(cfg *AppConfig) error {
	dir, err := configDirPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, configFile), data, 0600)
}

func tokenPath() (string, error) {
	dir, err := configDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenFile), nil
}

func loadToken() (*oauth2.Token, error) {
	path, err := tokenPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(tok *oauth2.Token) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func oauthConfig(appCfg *AppConfig, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     appCfg.ClientID,
		ClientSecret: appCfg.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{docsv1.DocumentsReadonlyScope},
		Endpoint:     google.Endpoint,
	}
}

// persistentTokenSource wraps a TokenSource and saves refreshed tokens to disk.
type persistentTokenSource struct {
	base      oauth2.TokenSource
	lastToken *oauth2.Token
}

func (p *persistentTokenSource) Token() (*oauth2.Token, error) {
	t, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != p.lastToken.AccessToken {
		_ = saveToken(t)
		p.lastToken = t
	}
	return t, nil
}

// GetAuthenticatedClient returns an HTTP client authenticated with Google OAuth2.
// It loads cached tokens when available and runs the browser OAuth flow on first use.
func GetAuthenticatedClient(ctx context.Context) (*http.Client, error) {
	appCfg, err := LoadAppConfig()
	if err != nil {
		return nil, err
	}

	tok, err := loadToken()
	if err != nil {
		tok, err = runOAuthFlow(ctx, appCfg)
		if err != nil {
			return nil, fmt.Errorf("authorization failed: %w", err)
		}
		if err := saveToken(tok); err != nil {
			return nil, fmt.Errorf("failed to save token: %w", err)
		}
	}

	cfg := oauthConfig(appCfg, "")
	ts := &persistentTokenSource{
		base:      cfg.TokenSource(ctx, tok),
		lastToken: tok,
	}
	return oauth2.NewClient(ctx, ts), nil
}

// runOAuthFlow starts a localhost server, opens the browser, and exchanges
// the authorization code for an OAuth2 token.
func runOAuthFlow(ctx context.Context, appCfg *AppConfig) (*oauth2.Token, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not start local server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	cfg := oauthConfig(appCfg, redirectURL)

	state := randomState()
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			http.Error(w, "authorization failed", http.StatusBadRequest)
			errCh <- fmt.Errorf("no auth code received: %s", errMsg)
			return
		}
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	fmt.Println("Opening browser for authorization...")
	fmt.Printf("If the browser does not open, visit:\n  %s\n\n", authURL)
	openBrowser(authURL)

	select {
	case code := <-codeCh:
		return cfg.Exchange(ctx, code)
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("timed out waiting for authorization (5 minutes)")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
