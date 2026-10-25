package identity

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"learning-setu/app/internal/config"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type User struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Company bool   `json:"companyIdentity"`
	TeamOrg string `json:"teamOrg,omitempty"`
}

var Accounts = []User{{"company", "company@example.test", true, ""}, {"applicant", "applicant@example.test", false, ""}, {"alpha-admin", "alpha@example.test", false, ""}, {"beta-admin", "beta@example.test", false, ""}, {"teacher", "teacher@example.test", false, ""}, {"viewer", "viewer@example.test", false, ""}, {"invitee", "invitee@example.test", false, ""}}

type Auth struct {
	Config  config.Config
	HTTP    *http.Client
	mu      sync.Mutex
	keys    map[string]map[string]*rsa.PublicKey
	expires map[string]time.Time
}

func New(c config.Config) *Auth {
	return &Auth{Config: c, HTTP: &http.Client{Timeout: 5 * time.Second}, keys: map[string]map[string]*rsa.PublicKey{}, expires: map[string]time.Time{}}
}
func (a *Auth) LocalToken(account string) (string, error) {
	if !a.Config.Local() || a.Config.AuthMode != "local" {
		return "", fmt.Errorf("local auth disabled")
	}
	for _, u := range Accounts {
		if u.Sub == account {
			return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": u.Sub, "email": u.Email, "email_verified": true, "iss": "local", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(a.Config.LocalSecret))
		}
	}
	return "", fmt.Errorf("unknown synthetic account")
}
func (a *Auth) Verify(ctx context.Context, header string) (User, error) {
	var zero User
	if !strings.HasPrefix(header, "Bearer ") || len(header) > 16000 {
		return zero, fmt.Errorf("sign in required")
	}
	raw := strings.TrimPrefix(header, "Bearer ")
	claims := jwt.MapClaims{}
	methods := []string{"RS256"}
	if a.Config.AuthMode == "local" {
		methods = []string{"HS256"}
	}
	token, e := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if a.Config.AuthMode == "local" {
			return []byte(a.Config.LocalSecret), nil
		}
		issuer, _ := claims["iss"].(string)
		if issuer != a.Config.Issuer && issuer != a.Config.CompanyIssuer {
			return nil, fmt.Errorf("invalid issuer")
		}
		kid, _ := t.Header["kid"].(string)
		return a.key(ctx, issuer, kid)
	}, jwt.WithValidMethods(methods), jwt.WithExpirationRequired())
	if e != nil || !token.Valid {
		return zero, fmt.Errorf("invalid or expired sign-in")
	}
	sub, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	verified, _ := claims["email_verified"].(bool)
	issuer, _ := claims["iss"].(string)
	teamOrg, _ := claims["custom:team_org"].(string)
	if teamOrg != "" && (issuer != a.Config.Issuer || len(teamOrg) != 36) {
		return zero, fmt.Errorf("invalid team scope")
	}
	if sub == "" || email == "" || (!verified && teamOrg == "") {
		return zero, fmt.Errorf("verified sign-in required")
	}
	if a.Config.AuthMode == "local" {
		if issuer != "local" {
			return zero, fmt.Errorf("invalid issuer")
		}
		for _, u := range Accounts {
			if u.Sub == sub && u.Email == email {
				return u, nil
			}
		}
		return zero, fmt.Errorf("unknown account")
	}
	audience, e := claims.GetAudience()
	client := a.Config.Client
	if issuer == a.Config.CompanyIssuer {
		client = a.Config.CompanyClient
	}
	if e != nil || len(audience) != 1 || audience[0] != client || claims["token_use"] != "id" {
		return zero, fmt.Errorf("invalid token audience or type")
	}
	return User{sub, strings.ToLower(email), issuer == a.Config.CompanyIssuer, teamOrg}, nil
}
func (a *Auth) key(ctx context.Context, issuer, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().Before(a.expires[issuer]) {
		if k := a.keys[issuer][kid]; k != nil {
			return k, nil
		}
	}
	req, e := http.NewRequestWithContext(ctx, "GET", issuer+"/.well-known/jwks.json", nil)
	if e != nil {
		return nil, e
	}
	r, e := a.HTTP.Do(req)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("identity service unavailable")
	}
	var body struct{ Keys []struct{ Kid, N, E string } }
	if e = json.NewDecoder(http.MaxBytesReader(nil, r.Body, 100000)).Decode(&body); e != nil {
		return nil, e
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range body.Keys {
		n, ne := base64.RawURLEncoding.DecodeString(k.N)
		exp, ee := base64.RawURLEncoding.DecodeString(k.E)
		if ne != nil || ee != nil || len(exp) > 4 {
			continue
		}
		v := 0
		for _, b := range exp {
			v = v*256 + int(b)
		}
		if v > 1 {
			keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: v}
		}
	}
	a.keys[issuer] = keys
	a.expires[issuer] = time.Now().Add(5 * time.Minute)
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("unknown signing key")
}
