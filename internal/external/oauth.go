package external

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const oauthExpirySkew = time.Minute

type OAuthTokenRecord struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	TokenType     string    `json:"token_type,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	ClientID      string    `json:"client_id,omitempty"`
	ClientSecret  string    `json:"client_secret,omitempty"`
	TokenEndpoint string    `json:"token_endpoint,omitempty"`
	MetadataURL   string    `json:"metadata_url,omitempty"`
	AuthServerURL string    `json:"auth_server_url,omitempty"`
}

func (r OAuthTokenRecord) valid(now time.Time) bool {
	if r.AccessToken == "" {
		return false
	}
	if r.ExpiresAt.IsZero() {
		return true
	}
	return now.Add(oauthExpirySkew).Before(r.ExpiresAt)
}

type OAuthTokenStore struct {
	home string
}

func NewOAuthTokenStore(home string) *OAuthTokenStore {
	return &OAuthTokenStore{home: home}
}

func (s *OAuthTokenStore) Load(serverURL string) (OAuthTokenRecord, bool, error) {
	path, err := s.path(serverURL)
	if err != nil {
		return OAuthTokenRecord{}, false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OAuthTokenRecord{}, false, nil
		}
		return OAuthTokenRecord{}, false, err
	}
	var record OAuthTokenRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return OAuthTokenRecord{}, false, err
	}
	return record, true, nil
}

func (s *OAuthTokenStore) Save(serverURL string, record OAuthTokenRecord) error {
	path, err := s.path(serverURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Dir(path), 0700)
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func (s *OAuthTokenStore) path(serverURL string) (string, error) {
	home := s.home
	if home == "" {
		detected, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = detected
	}
	sum := sha256.Sum256([]byte(serverURL))
	return filepath.Join(home, ".mewcode", "oauth", hex.EncodeToString(sum[:])+".json"), nil
}

type oauthRedirectServer interface {
	Start(ctx context.Context, state string) (string, func(context.Context) (string, error), func() error, error)
}

type MCPOAuthProvider struct {
	serverURL      string
	client         HTTPDoer
	store          *OAuthTokenStore
	openURL        func(string) error
	redirectServer oauthRedirectServer
	mu             sync.Mutex
}

func NewMCPOAuthProvider(serverURL string, client HTTPDoer, store *OAuthTokenStore) *MCPOAuthProvider {
	if client == nil {
		client = http.DefaultClient
	}
	if store == nil {
		store = NewOAuthTokenStore("")
	}
	return &MCPOAuthProvider{
		serverURL:      serverURL,
		client:         client,
		store:          store,
		openURL:        openBrowser,
		redirectServer: loopbackOAuthRedirectServer{},
	}
}

func (p *MCPOAuthProvider) BearerToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	record, ok, err := p.store.Load(p.serverURL)
	if err != nil || !ok {
		return "", err
	}
	if record.valid(time.Now()) {
		return record.AccessToken, nil
	}
	refreshed, err := p.refresh(ctx, record)
	if err != nil {
		return "", nil
	}
	return refreshed.AccessToken, nil
}

func (p *MCPOAuthProvider) HandleUnauthorized(ctx context.Context, header http.Header) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	record, ok, _ := p.store.Load(p.serverURL)
	if ok && record.RefreshToken != "" && record.TokenEndpoint != "" && record.ClientID != "" {
		refreshed, err := p.refresh(ctx, record)
		if err == nil && refreshed.AccessToken != "" {
			return refreshed.AccessToken, nil
		}
	}
	metadataURL, ok := parseOAuthResourceMetadata(header)
	if !ok {
		return "", fmt.Errorf("http status 401: OAuth resource metadata missing")
	}
	return p.authorize(ctx, metadataURL)
}

func (p *MCPOAuthProvider) authorize(ctx context.Context, resourceMetadataURL string) (string, error) {
	resourceMetadata, err := p.fetchResourceMetadata(ctx, resourceMetadataURL)
	if err != nil {
		return "", err
	}
	if len(resourceMetadata.AuthorizationServers) == 0 {
		return "", fmt.Errorf("OAuth resource metadata has no authorization servers")
	}
	authServerURL := resourceMetadata.AuthorizationServers[0]
	authMetadata, err := p.fetchAuthorizationServerMetadata(ctx, authServerURL)
	if err != nil {
		return "", err
	}
	if authMetadata.AuthorizationEndpoint == "" || authMetadata.TokenEndpoint == "" {
		return "", fmt.Errorf("OAuth authorization server metadata is missing endpoints")
	}

	state, err := randomURLSafe(32)
	if err != nil {
		return "", err
	}
	verifier, challenge, err := pkcePair()
	if err != nil {
		return "", err
	}
	redirectURI, waitForCode, closeRedirect, err := p.redirectServer.Start(ctx, state)
	if err != nil {
		return "", fmt.Errorf("start OAuth callback: %w", err)
	}
	defer closeRedirect()

	clientID := ""
	clientSecret := ""
	if authMetadata.RegistrationEndpoint != "" {
		registration, err := p.registerClient(ctx, authMetadata.RegistrationEndpoint, redirectURI)
		if err != nil {
			return "", err
		}
		clientID = registration.ClientID
		clientSecret = registration.ClientSecret
	}
	if clientID == "" {
		return "", fmt.Errorf("OAuth server does not support dynamic client registration and no client_id is configured")
	}

	authURL, err := buildAuthorizationURL(authMetadata.AuthorizationEndpoint, oauthAuthorizationRequest{
		ResponseType:        "code",
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		State:               state,
		Resource:            p.serverURL,
	})
	if err != nil {
		return "", err
	}
	if err := p.openURL(authURL); err != nil {
		return "", fmt.Errorf("open OAuth browser: %w", err)
	}
	code, err := waitForCode(ctx)
	if err != nil {
		return "", err
	}
	record, err := p.exchangeCode(ctx, authMetadata.TokenEndpoint, clientID, clientSecret, redirectURI, code, verifier)
	if err != nil {
		return "", err
	}
	record.ClientID = clientID
	record.ClientSecret = clientSecret
	record.TokenEndpoint = authMetadata.TokenEndpoint
	record.MetadataURL = resourceMetadataURL
	record.AuthServerURL = authServerURL
	if err := p.store.Save(p.serverURL, record); err != nil {
		return "", err
	}
	return record.AccessToken, nil
}

type oauthResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
}

func (p *MCPOAuthProvider) fetchResourceMetadata(ctx context.Context, metadataURL string) (oauthResourceMetadata, error) {
	var result oauthResourceMetadata
	if err := p.getJSON(ctx, metadataURL, &result); err != nil {
		return result, fmt.Errorf("fetch OAuth resource metadata: %w", err)
	}
	return result, nil
}

type oauthAuthorizationServerMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

func (p *MCPOAuthProvider) fetchAuthorizationServerMetadata(ctx context.Context, issuer string) (oauthAuthorizationServerMetadata, error) {
	var result oauthAuthorizationServerMetadata
	metadataURL, err := authorizationServerMetadataURL(issuer)
	if err != nil {
		return result, err
	}
	if err := p.getJSON(ctx, metadataURL, &result); err != nil {
		return result, fmt.Errorf("fetch OAuth authorization server metadata: %w", err)
	}
	return result, nil
}

func authorizationServerMetadataURL(issuer string) (string, error) {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return "", err
	}
	if strings.Contains(parsed.Path, ".well-known/oauth-authorization-server") {
		return parsed.String(), nil
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "" {
		parsed.Path = "/.well-known/oauth-authorization-server"
	} else {
		parsed.Path = "/.well-known/oauth-authorization-server" + path
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (p *MCPOAuthProvider) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type oauthClientRegistration struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

func (p *MCPOAuthProvider) registerClient(ctx context.Context, endpoint string, redirectURI string) (oauthClientRegistration, error) {
	payload := map[string]any{
		"client_name":                "MewCode",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return oauthClientRegistration{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return oauthClientRegistration{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return oauthClientRegistration{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthClientRegistration{}, fmt.Errorf("OAuth client registration failed: http status %d", resp.StatusCode)
	}
	var result oauthClientRegistration
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oauthClientRegistration{}, err
	}
	if result.ClientID == "" {
		return oauthClientRegistration{}, fmt.Errorf("OAuth client registration returned no client_id")
	}
	return result, nil
}

type oauthAuthorizationRequest struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
	Resource            string
}

func buildAuthorizationURL(endpoint string, request oauthAuthorizationRequest) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	values.Set("response_type", request.ResponseType)
	values.Set("client_id", request.ClientID)
	values.Set("redirect_uri", request.RedirectURI)
	values.Set("code_challenge", request.CodeChallenge)
	values.Set("code_challenge_method", request.CodeChallengeMethod)
	values.Set("state", request.State)
	values.Set("resource", request.Resource)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func (p *MCPOAuthProvider) exchangeCode(ctx context.Context, tokenEndpoint, clientID, clientSecret, redirectURI, code, verifier string) (OAuthTokenRecord, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", redirectURI)
	values.Set("client_id", clientID)
	values.Set("code_verifier", verifier)
	values.Set("resource", p.serverURL)
	if clientSecret != "" {
		values.Set("client_secret", clientSecret)
	}
	return p.postToken(ctx, tokenEndpoint, values)
}

func (p *MCPOAuthProvider) refresh(ctx context.Context, record OAuthTokenRecord) (OAuthTokenRecord, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", record.RefreshToken)
	values.Set("client_id", record.ClientID)
	values.Set("resource", p.serverURL)
	if record.ClientSecret != "" {
		values.Set("client_secret", record.ClientSecret)
	}
	refreshed, err := p.postToken(ctx, record.TokenEndpoint, values)
	if err != nil {
		return OAuthTokenRecord{}, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = record.RefreshToken
	}
	refreshed.ClientID = record.ClientID
	refreshed.ClientSecret = record.ClientSecret
	refreshed.TokenEndpoint = record.TokenEndpoint
	refreshed.MetadataURL = record.MetadataURL
	refreshed.AuthServerURL = record.AuthServerURL
	if err := p.store.Save(p.serverURL, refreshed); err != nil {
		return OAuthTokenRecord{}, err
	}
	return refreshed, nil
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

func (p *MCPOAuthProvider) postToken(ctx context.Context, endpoint string, values url.Values) (OAuthTokenRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return OAuthTokenRecord{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return OAuthTokenRecord{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OAuthTokenRecord{}, fmt.Errorf("OAuth token request failed: http status %d", resp.StatusCode)
	}
	var token oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return OAuthTokenRecord{}, err
	}
	if token.AccessToken == "" {
		return OAuthTokenRecord{}, fmt.Errorf("OAuth token response missing access_token")
	}
	record := OAuthTokenRecord{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
	}
	if record.TokenType == "" {
		record.TokenType = "Bearer"
	}
	if token.ExpiresIn > 0 {
		record.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	return record, nil
}

func parseOAuthResourceMetadata(header http.Header) (string, bool) {
	for name, values := range header {
		if !strings.EqualFold(name, "WWW-Authenticate") {
			continue
		}
		for _, value := range values {
			if metadata, ok := parseAuthParam(value, "resource_metadata"); ok {
				return metadata, true
			}
		}
	}
	return "", false
}

func parseAuthParam(challenge string, key string) (string, bool) {
	challenge = strings.TrimSpace(challenge)
	if strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		challenge = strings.TrimSpace(challenge[len("bearer "):])
	}
	for _, part := range splitAuthParams(challenge) {
		name, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconvUnquote(value); err == nil {
			value = unquoted
		}
		if value != "" {
			return value, true
		}
	}
	return "", false
}

func splitAuthParams(value string) []string {
	var parts []string
	start := 0
	inQuote := false
	escaped := false
	for index, char := range value {
		switch {
		case escaped:
			escaped = false
		case char == '\\' && inQuote:
			escaped = true
		case char == '"':
			inQuote = !inQuote
		case char == ',' && !inQuote:
			parts = append(parts, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func strconvUnquote(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return value, errors.New("not quoted")
	}
	var out strings.Builder
	escaped := false
	for _, char := range value[1 : len(value)-1] {
		if escaped {
			out.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		out.WriteRune(char)
	}
	if escaped {
		out.WriteRune('\\')
	}
	return out.String(), nil
}

func pkcePair() (string, string, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomURLSafe(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

type loopbackOAuthRedirectServer struct{}

func (loopbackOAuthRedirectServer) Start(ctx context.Context, state string) (string, func(context.Context) (string, error), func() error, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "OAuth state mismatch. You can close this tab.", http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("OAuth state mismatch"):
			default:
			}
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "OAuth callback missing code. You can close this tab.", http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("OAuth callback missing code"):
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "MewCode OAuth login complete. You can close this tab.")
		select {
		case codeCh <- code:
		default:
		}
	})
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()
	redirectURI := "http://" + listener.Addr().String() + "/callback"
	wait := func(waitCtx context.Context) (string, error) {
		if waitCtx == nil {
			waitCtx = ctx
		}
		select {
		case code := <-codeCh:
			return code, nil
		case err := <-errCh:
			return "", err
		case <-waitCtx.Done():
			return "", waitCtx.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	closeFn := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = listener.Close()
		return server.Shutdown(shutdownCtx)
	}
	return redirectURI, wait, closeFn, nil
}

func openBrowser(rawURL string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{rawURL}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command = "xdg-open"
		args = []string{rawURL}
	}
	return exec.Command(command, args...).Start()
}
