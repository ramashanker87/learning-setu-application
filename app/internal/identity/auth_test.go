package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/golang-jwt/jwt/v5"
	"learning-setu/app/internal/config"
	"strings"
	"testing"
	"time"
)

func TestLocalTokensRejectTamperingAndExpiration(t *testing.T) {
	a := New(config.Config{Env: "test", AuthMode: "local", LocalSecret: strings.Repeat("s", 32)})
	token, e := a.LocalToken("company")
	if e != nil {
		t.Fatal(e)
	}
	u, e := a.Verify(context.Background(), "Bearer "+token)
	if e != nil || !u.Company {
		t.Fatal("valid synthetic token rejected")
	}
	parts := strings.Split(token, ".")
	parts[2] = strings.Repeat("a", len(parts[2]))
	if _, e = a.Verify(context.Background(), "Bearer "+strings.Join(parts, ".")); e == nil {
		t.Fatal("accepted forged token")
	}
	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "company", "email": "company@example.test", "email_verified": true, "iss": "local", "exp": time.Now().Add(-time.Minute).Unix()}).SignedString([]byte(a.Config.LocalSecret))
	if _, e = a.Verify(context.Background(), "Bearer "+expired); e == nil {
		t.Fatal("accepted expired token")
	}
}

func TestScopedTeamTokensAndAudience(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	a := New(config.Config{AuthMode: "cognito", Issuer: "https://tenant.test", Client: "tenant-client", CompanyIssuer: "https://company.test", CompanyClient: "company-client"})
	a.keys[a.Config.Issuer] = map[string]*rsa.PublicKey{"key": &key.PublicKey}
	a.expires[a.Config.Issuer] = time.Now().Add(time.Hour)
	a.keys[a.Config.CompanyIssuer] = map[string]*rsa.PublicKey{"key": &key.PublicKey}
	a.expires[a.Config.CompanyIssuer] = time.Now().Add(time.Hour)
	scope := "12345678-1234-1234-1234-123456789012"
	for _, tc := range []struct {
		scope, issuer, audience string
		verified, want          bool
	}{
		{scope, a.Config.Issuer, a.Config.Client, false, true},
		{"", a.Config.Issuer, a.Config.Client, false, false},
		{scope, a.Config.CompanyIssuer, a.Config.CompanyClient, true, false},
		{scope, a.Config.Issuer, a.Config.CompanyClient, false, false},
		{"invalid", a.Config.Issuer, a.Config.Client, false, false},
		{"", a.Config.Issuer, a.Config.Client, true, true},
	} {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": "team-sub", "email": "member@example.test", "email_verified": tc.verified, "custom:team_org": tc.scope, "iss": tc.issuer, "aud": tc.audience, "token_use": "id", "exp": time.Now().Add(time.Hour).Unix()})
		token.Header["kid"] = "key"
		raw, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		u, err := a.Verify(context.Background(), "Bearer "+raw)
		if (err == nil) != tc.want {
			t.Fatalf("%+v: %v", tc, err)
		}
		if tc.want && u.TeamOrg != tc.scope {
			t.Fatal("team scope lost")
		}
	}
}
