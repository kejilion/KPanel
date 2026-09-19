package mcpaccess

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func oauthFixture(t *testing.T) (*OAuth, *Store, OAuthRegistration, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	o := OpenOAuth(dir)
	base := Open(dir)
	_, err := base.SetEnabled(true, base.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := o.Register("Desktop", []string{"http://127.0.0.1:5317/callback"}, "none")
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 64)
	sum := sha256.Sum256([]byte(verifier))
	a, err := o.Authorize(client.ID, client.RedirectURIs[0], base64.RawURLEncoding.EncodeToString(sum[:]), "https://panel.test/mcp", "csrf-state")
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := o.Consent(a.ID, true, func(OAuthAuthorization) (Client, error) {
		c, _, err := base.Create("OAuth: Desktop", []HostGrant{{ID: "local", Identity: strings.Repeat("a", 64)}}, 24*time.Hour, base.Snapshot().ResourceVersion)
		return c, err
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(redirect)
	if u.Query().Get("state") != "csrf-state" {
		t.Fatal("state lost")
	}
	return o, base, client, u.Query().Get("code"), verifier, dir
}

func TestOAuthPKCEAudienceExactRedirectRotationAndRevocation(t *testing.T) {
	o, base, c, code, verifier, dir := oauthFixture(t)
	for _, item := range []struct{ redirect, verifier, resource string }{
		{c.RedirectURIs[0], strings.Repeat("w", 64), "https://panel.test/mcp"},
		{"http://127.0.0.1:5318/callback", verifier, "https://panel.test/mcp"},
		{c.RedirectURIs[0], verifier, "https://other.test/mcp"},
	} {
		if _, err := o.Exchange(base, c.ID, "", "none", code, item.redirect, item.verifier, item.resource); !errors.Is(err, ErrOAuth) {
			t.Fatalf("binding bypass: %v", err)
		}
	}
	tokens, err := o.Exchange(base, c.ID, "", "none", code, c.RedirectURIs[0], verifier, "https://panel.test/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.TokenType != "Bearer" || tokens.ExpiresIn < 3598 || tokens.RefreshToken == "" {
		t.Fatal("invalid token response")
	}
	data, err := os.ReadFile(o.path)
	if err != nil || strings.Contains(string(data), tokens.AccessToken) || strings.Contains(string(data), "Desktop") {
		t.Fatal("plaintext OAuth state")
	}
	o = OpenOAuth(dir)
	p, release, err := o.Begin(base, tokens.AccessToken, "https://panel.test/mcp")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !o.Active(base, tokens.AccessToken, "https://panel.test/mcp", p) {
		t.Fatal("valid access denied")
	}
	if _, _, err := o.Begin(base, tokens.AccessToken, "https://other.test/mcp"); err == nil {
		t.Fatal("wrong resource accepted")
	}
	refreshed, err := o.Refresh(base, c.ID, "", "none", tokens.RefreshToken, "https://panel.test/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if o.Active(base, tokens.AccessToken, "https://panel.test/mcp", p) {
		t.Fatal("old access survived rotation")
	}
	if _, err := o.Refresh(base, c.ID, "", "none", tokens.RefreshToken, "https://panel.test/mcp"); !errors.Is(err, ErrOAuth) {
		t.Fatal("refresh reuse accepted")
	}
	if o.Active(base, refreshed.AccessToken, "https://panel.test/mcp", p) {
		t.Fatal("refresh reuse did not revoke family")
	}
	if _, err := base.LookupClient(p.ID); err == nil {
		t.Fatal("revoked OAuth grant could approve old operations")
	}
}

func TestOAuthCodeReplayExpiryMissingKeyAndClientAuthentication(t *testing.T) {
	o, base, c, code, verifier, dir := oauthFixture(t)
	tokens, err := o.Exchange(base, c.ID, "", "none", code, c.RedirectURIs[0], verifier, "https://panel.test/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Exchange(base, c.ID, "", "none", code, c.RedirectURIs[0], verifier, "https://panel.test/mcp"); !errors.Is(err, ErrOAuth) {
		t.Fatal("authorization code replay accepted")
	}
	if _, _, err := o.Begin(base, tokens.AccessToken, "https://panel.test/mcp"); err == nil {
		t.Fatal("replay did not revoke issued grant")
	}
	if err := os.Remove(o.path + ".key"); err != nil {
		t.Fatal(err)
	}
	if OpenOAuth(dir).available {
		t.Fatal("missing key silently reset OAuth state")
	}
	o, base, c, code, verifier, _ = oauthFixture(t)
	now := time.Now().Add(6 * time.Minute)
	o.now = func() time.Time { return now }
	if _, err := o.Exchange(base, c.ID, "", "none", code, c.RedirectURIs[0], verifier, "https://panel.test/mcp"); !errors.Is(err, ErrOAuth) {
		t.Fatal("expired authorization code accepted")
	}
	o = OpenOAuth(t.TempDir())
	c, secret, err := o.Register("Remote", []string{"https://client.test/callback"}, "client_secret_post")
	if err != nil {
		t.Fatal(err)
	}
	if !o.clientAuth(c.ID, secret, "client_secret_post") || o.clientAuth(c.ID, "wrong", "client_secret_post") || o.clientAuth(c.ID, secret, "none") {
		t.Fatal("client authentication bypass")
	}
	for _, uri := range []string{"http://client.test/callback", "javascript:alert(1)", "https://user:password@client.test/cb", "https://client.test/cb#fragment", "https://client.test/cb?code=existing"} {
		if ValidOAuthRedirect(uri) {
			t.Fatalf("unsafe redirect %q", uri)
		}
	}
}

func TestOAuthIdleRegistrationCanAuthorizeAfterOneDay(t *testing.T) {
	o := OpenOAuth(t.TempDir())
	base := Open(t.TempDir())
	_, _ = base.SetEnabled(true, base.Snapshot().ResourceVersion)
	c, _, err := o.Register("Desktop", []string{"http://127.0.0.1:5317/callback"}, "none")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(25 * time.Hour)
	o.now = func() time.Time { return now }
	base.now = o.now
	verifier := strings.Repeat("v", 64)
	sum := sha256.Sum256([]byte(verifier))
	a, err := o.Authorize(c.ID, c.RedirectURIs[0], base64.RawURLEncoding.EncodeToString(sum[:]), "https://panel.test/mcp", "")
	if err != nil {
		t.Fatal(err)
	}
	callback, err := o.Consent(a.ID, true, func(OAuthAuthorization) (Client, error) {
		client, _, err := base.Create("Renewed", []HostGrant{{ID: "local", Identity: strings.Repeat("a", 64)}}, time.Hour, base.Snapshot().ResourceVersion)
		return client, err
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(callback)
	if _, err := o.Exchange(base, c.ID, "", "none", u.Query().Get("code"), c.RedirectURIs[0], verifier, "https://panel.test/mcp"); err != nil {
		t.Fatal("idle registration disappeared during authorization", err)
	}
}
