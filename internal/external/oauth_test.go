package external

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseOAuthResourceMetadataFromWWWAuthenticate(t *testing.T) {
	got, ok := parseOAuthResourceMetadata(http.Header{
		"WWW-Authenticate": []string{`Bearer realm="mcp", resource_metadata="https://mcp.example/.well-known/oauth-protected-resource/mcp"`},
	})
	if !ok {
		t.Fatalf("resource metadata not found")
	}
	if got != "https://mcp.example/.well-known/oauth-protected-resource/mcp" {
		t.Fatalf("resource metadata = %q", got)
	}
}

func TestOAuthTokenStoreUsesPrivateHashedFiles(t *testing.T) {
	home := t.TempDir()
	store := NewOAuthTokenStore(home)
	record := OAuthTokenRecord{
		AccessToken:   "access-token-value",
		RefreshToken:  "refresh-token-value",
		TokenType:     "Bearer",
		ExpiresAt:     time.Now().Add(time.Hour),
		ClientID:      "client-123",
		TokenEndpoint: "https://auth.example/token",
	}
	if err := store.Save("https://mcp.example/mcp", record); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	dirInfo, err := os.Stat(filepath.Join(home, ".mewcode", "oauth"))
	if err != nil {
		t.Fatalf("stat store dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("store dir mode = %o, want 0700", got)
	}
	files, err := os.ReadDir(filepath.Join(home, ".mewcode", "oauth"))
	if err != nil {
		t.Fatalf("read store dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("store files = %d, want 1", len(files))
	}
	if strings.Contains(files[0].Name(), "mcp.example") {
		t.Fatalf("store filename leaks server URL: %s", files[0].Name())
	}
	fileInfo, err := files[0].Info()
	if err != nil {
		t.Fatalf("file info: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("store file mode = %o, want 0600", got)
	}
	loaded, ok, err := store.Load("https://mcp.example/mcp")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !ok || loaded.AccessToken != record.AccessToken || loaded.RefreshToken != record.RefreshToken {
		t.Fatalf("loaded = %#v ok=%v", loaded, ok)
	}
}

func TestMCPOAuthProviderAuthorizationCodeFlowUsesPKCEAndResource(t *testing.T) {
	ctx := context.Background()
	var openedURL string
	var tokenBody string
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://mcp.example/.well-known/oauth-protected-resource/mcp":
			return jsonResponse(http.StatusOK, map[string]any{
				"authorization_servers": []string{"https://auth.example"},
			}), nil
		case "https://auth.example/.well-known/oauth-authorization-server":
			return jsonResponse(http.StatusOK, map[string]any{
				"authorization_endpoint": "https://auth.example/authorize",
				"token_endpoint":         "https://auth.example/token",
				"registration_endpoint":  "https://auth.example/register",
			}), nil
		case "https://auth.example/register":
			if req.Method != http.MethodPost {
				t.Fatalf("registration method = %s", req.Method)
			}
			return jsonResponse(http.StatusCreated, map[string]any{"client_id": "client-123"}), nil
		case "https://auth.example/token":
			raw, _ := io.ReadAll(req.Body)
			tokenBody = string(raw)
			values, err := url.ParseQuery(tokenBody)
			if err != nil {
				t.Fatalf("token body parse: %v", err)
			}
			for _, key := range []string{"grant_type", "code", "redirect_uri", "client_id", "code_verifier", "resource"} {
				if values.Get(key) == "" {
					t.Fatalf("token request missing %s in %s", key, tokenBody)
				}
			}
			if values.Get("grant_type") != "authorization_code" {
				t.Fatalf("grant_type = %q", values.Get("grant_type"))
			}
			if values.Get("resource") != "https://mcp.example/mcp" {
				t.Fatalf("resource = %q", values.Get("resource"))
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"access_token":  "new-access-token",
				"refresh_token": "new-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
			}), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return nil, nil
	})

	provider := NewMCPOAuthProvider("https://mcp.example/mcp", client, NewOAuthTokenStore(t.TempDir()))
	provider.openURL = func(raw string) error {
		openedURL = raw
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("authorization URL parse: %v", err)
		}
		query := parsed.Query()
		for _, key := range []string{"response_type", "client_id", "redirect_uri", "code_challenge", "code_challenge_method", "state", "resource"} {
			if query.Get(key) == "" {
				t.Fatalf("authorization URL missing %s in %s", key, raw)
			}
		}
		if query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" {
			t.Fatalf("authorization URL query = %s", parsed.RawQuery)
		}
		if query.Get("resource") != "https://mcp.example/mcp" {
			t.Fatalf("authorization resource = %q", query.Get("resource"))
		}
		return nil
	}
	provider.redirectServer = fakeOAuthRedirectServer{code: "auth-code-from-browser"}

	token, err := provider.HandleUnauthorized(ctx, http.Header{
		"WWW-Authenticate": []string{`Bearer resource_metadata="https://mcp.example/.well-known/oauth-protected-resource/mcp"`},
	})
	if err != nil {
		t.Fatalf("HandleUnauthorized returned error: %v", err)
	}
	if token != "new-access-token" {
		t.Fatalf("token = %q", token)
	}
	if openedURL == "" {
		t.Fatalf("browser was not opened")
	}
	if strings.Contains(tokenBody, "new-access-token") || strings.Contains(openedURL, "auth-code-from-browser") {
		t.Fatalf("flow leaked values into wrong request")
	}
}

func TestMCPOAuthProviderRefreshesExpiredCachedToken(t *testing.T) {
	store := NewOAuthTokenStore(t.TempDir())
	if err := store.Save("https://mcp.example/mcp", OAuthTokenRecord{
		AccessToken:   "old-access-token",
		RefreshToken:  "refresh-token-value",
		TokenType:     "Bearer",
		ExpiresAt:     time.Now().Add(-time.Hour),
		ClientID:      "client-123",
		TokenEndpoint: "https://auth.example/token",
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	var refreshBody string
	provider := NewMCPOAuthProvider("https://mcp.example/mcp", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://auth.example/token" {
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		raw, _ := io.ReadAll(req.Body)
		refreshBody = string(raw)
		values, err := url.ParseQuery(refreshBody)
		if err != nil {
			t.Fatalf("refresh body parse: %v", err)
		}
		for _, key := range []string{"grant_type", "refresh_token", "client_id", "resource"} {
			if values.Get(key) == "" {
				t.Fatalf("refresh request missing %s in %s", key, refreshBody)
			}
		}
		if values.Get("grant_type") != "refresh_token" || values.Get("resource") != "https://mcp.example/mcp" {
			t.Fatalf("refresh body = %s", refreshBody)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"access_token":  "refreshed-access-token",
			"refresh_token": "rotated-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		}), nil
	}), store)

	token, err := provider.BearerToken(context.Background())
	if err != nil {
		t.Fatalf("BearerToken returned error: %v", err)
	}
	if token != "refreshed-access-token" {
		t.Fatalf("token = %q", token)
	}
	if strings.Contains(refreshBody, "old-access-token") {
		t.Fatalf("refresh request leaked old access token: %s", refreshBody)
	}
}

func TestMCPOAuthProviderFallsBackToBrowserLoginWhenRefreshFails(t *testing.T) {
	store := NewOAuthTokenStore(t.TempDir())
	if err := store.Save("https://mcp.example/mcp", OAuthTokenRecord{
		AccessToken:   "expired-access-token",
		RefreshToken:  "stale-refresh-token",
		TokenType:     "Bearer",
		ExpiresAt:     time.Now().Add(-time.Hour),
		ClientID:      "client-123",
		TokenEndpoint: "https://auth.example/token",
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	tokenRequests := 0
	provider := NewMCPOAuthProvider("https://mcp.example/mcp", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://auth.example/token":
			tokenRequests++
			if tokenRequests == 1 {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid_grant"}), nil
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"access_token":  "browser-access-token",
				"refresh_token": "browser-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
			}), nil
		case "https://mcp.example/.well-known/oauth-protected-resource/mcp":
			return jsonResponse(http.StatusOK, map[string]any{"authorization_servers": []string{"https://auth.example"}}), nil
		case "https://auth.example/.well-known/oauth-authorization-server":
			return jsonResponse(http.StatusOK, map[string]any{
				"authorization_endpoint": "https://auth.example/authorize",
				"token_endpoint":         "https://auth.example/token",
				"registration_endpoint":  "https://auth.example/register",
			}), nil
		case "https://auth.example/register":
			return jsonResponse(http.StatusCreated, map[string]any{"client_id": "browser-client"}), nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		return nil, nil
	}), store)
	opened := false
	provider.openURL = func(raw string) error {
		opened = true
		return nil
	}
	provider.redirectServer = fakeOAuthRedirectServer{code: "browser-code"}

	token, err := provider.HandleUnauthorized(context.Background(), http.Header{
		"WWW-Authenticate": []string{`Bearer resource_metadata="https://mcp.example/.well-known/oauth-protected-resource/mcp"`},
	})
	if err != nil {
		t.Fatalf("HandleUnauthorized returned error: %v", err)
	}
	if token != "browser-access-token" {
		t.Fatalf("token = %q", token)
	}
	if !opened {
		t.Fatalf("browser fallback was not opened")
	}
	if tokenRequests != 2 {
		t.Fatalf("token requests = %d, want refresh attempt plus code exchange", tokenRequests)
	}
}

type fakeOAuthRedirectServer struct {
	code string
}

func (f fakeOAuthRedirectServer) Start(ctx context.Context, state string) (string, func(context.Context) (string, error), func() error, error) {
	return "http://127.0.0.1:12345/callback", func(context.Context) (string, error) {
		return f.code, nil
	}, func() error { return nil }, nil
}

func jsonResponse(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
