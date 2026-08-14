package jira

import (
	"strings"
	"testing"
	"time"
)

func TestOAuthTokens_Expiry(t *testing.T) {
	tokens := &OAuthTokens{ExpiresIn: 3600}
	before := time.Now().Add(3599 * time.Second)
	expiry := tokens.Expiry()
	after := time.Now().Add(3601 * time.Second)
	if expiry.Before(before) || expiry.After(after) {
		t.Errorf("expiry %v not within expected range [%v, %v]", expiry, before, after)
	}
}

func TestCloudBaseURL(t *testing.T) {
	got := CloudBaseURL("abc-123")
	want := "https://api.atlassian.com/ex/jira/abc-123"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestNewOAuthService(t *testing.T) {
	svc := NewOAuthService(OAuthConfig{ClientID: "id", ClientSecret: "secret", CallbackURL: "https://cb"})
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestOAuthService_AuthorizeURL(t *testing.T) {
	svc := NewOAuthService(OAuthConfig{ClientID: "my-client-id", ClientSecret: "secret", CallbackURL: "https://myplanner.local/callback"})
	url := svc.AuthorizeURL("state-token-123")

	if !strings.HasPrefix(url, "https://auth.atlassian.com/authorize?") {
		t.Errorf("unexpected URL prefix: %s", url)
	}
	if !strings.Contains(url, "client_id=my-client-id") {
		t.Errorf("expected client_id in URL: %s", url)
	}
	if !strings.Contains(url, "state=state-token-123") {
		t.Errorf("expected state in URL: %s", url)
	}
	if !strings.Contains(url, "response_type=code") {
		t.Errorf("expected response_type=code in URL: %s", url)
	}
}
