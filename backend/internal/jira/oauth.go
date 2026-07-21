package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	atlassianAuthURL     = "https://auth.atlassian.com/authorize"
	atlassianTokenURL    = "https://auth.atlassian.com/oauth/token"
	atlassianResourceURL = "https://api.atlassian.com/oauth/token/accessible-resources"

	oauthScopes = "read:jira-work read:jira-user read:board-scope:jira-software read:sprint:jira-software read:project:jira write:sprint:jira-software offline_access"
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
}

type OAuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func (t *OAuthTokens) Expiry() time.Time {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
}

type CloudResource struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	AvatarURL string   `json:"avatarUrl"`
}

func CloudBaseURL(cloudID string) string {
	return "https://api.atlassian.com/ex/jira/" + cloudID
}

type OAuthService struct {
	config     OAuthConfig
	httpClient *http.Client
}

func NewOAuthService(cfg OAuthConfig) *OAuthService {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
	}
	return &OAuthService{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

func (s *OAuthService) AuthorizeURL(state string) string {
	params := url.Values{
		"audience":      {"api.atlassian.com"},
		"client_id":     {s.config.ClientID},
		"scope":         {oauthScopes},
		"redirect_uri":  {s.config.CallbackURL},
		"state":         {state},
		"response_type": {"code"},
		"prompt":        {"consent"},
	}
	return atlassianAuthURL + "?" + params.Encode()
}

func (s *OAuthService) ExchangeCode(ctx context.Context, code string) (*OAuthTokens, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {s.config.ClientID},
		"client_secret": {s.config.ClientSecret},
		"code":          {code},
		"redirect_uri":  {s.config.CallbackURL},
	}
	return s.tokenRequest(ctx, body)
}

func (s *OAuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {s.config.ClientID},
		"client_secret": {s.config.ClientSecret},
		"refresh_token": {refreshToken},
	}
	return s.tokenRequest(ctx, body)
}

func (s *OAuthService) GetAccessibleResources(ctx context.Context, accessToken string) ([]CloudResource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, atlassianResourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching accessible resources: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("accessible resources error: status %d, body: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var resources []CloudResource
	if err := json.Unmarshal(respBody, &resources); err != nil {
		return nil, fmt.Errorf("decoding resources: %w", err)
	}
	return resources, nil
}

func (s *OAuthService) tokenRequest(ctx context.Context, params url.Values) (*OAuthTokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, atlassianTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("token error: status %d, body: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var tokens OAuthTokens
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		return nil, fmt.Errorf("decoding tokens: %w", err)
	}
	return &tokens, nil
}
