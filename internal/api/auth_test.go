package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

const issuer = "https://idp.example.com/realms/tre"

type signer struct{ jose.Signer }

func newSigner(t *testing.T) (*signer, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	return &signer{s}, key
}

func (s *signer) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, _ := json.Marshal(claims)
	jws, err := s.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := jws.CompactSerialize()
	return raw
}

func authenticator(key *rsa.PrivateKey, rolesClaim string) *OIDCAuthenticator {
	ks := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{key.Public()}}
	return newOIDCAuthenticator(oidc.NewVerifier(issuer, ks, &oidc.Config{ClientID: "kubetre-api"}), rolesClaim)
}

func bearer(tok string) *http.Request {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	return r
}

func claims(extra map[string]any) map[string]any {
	c := map[string]any{
		"iss": issuer, "aud": "kubetre-api", "sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"email": "rita@example.com", "name": "Rita",
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

func TestOIDCAcceptsValidTokenAndReadsRoles(t *testing.T) {
	s, key := newSigner(t)
	id, err := authenticator(key, "roles").Authenticate(bearer(s.token(t, claims(map[string]any{"roles": []string{"TREAdmin"}}))))
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "user-123" || id.Email != "rita@example.com" || !id.HasRole("TREAdmin") {
		t.Fatalf("identity = %+v", id)
	}
}

func TestOIDCReadsNestedRolesClaim(t *testing.T) {
	s, key := newSigner(t)
	tok := s.token(t, claims(map[string]any{"realm_access": map[string]any{"roles": []string{"TREAdmin", "offline_access"}}}))
	id, err := authenticator(key, "realm_access.roles").Authenticate(bearer(tok))
	if err != nil || !id.HasRole("TREAdmin") {
		t.Fatalf("id = %+v err = %v", id, err)
	}
}

func TestOIDCRejectsBadTokens(t *testing.T) {
	s, key := newSigner(t)
	other, _ := newSigner(t)
	a := authenticator(key, "roles")
	cases := map[string]string{
		"wrong audience": s.token(t, claims(map[string]any{"aud": "someone-else"})),
		"wrong issuer":   s.token(t, claims(map[string]any{"iss": "https://evil.example.com"})),
		"expired":        s.token(t, claims(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})),
		"wrong key":      other.token(t, claims(nil)),
		"garbage":        "not.a.jwt",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := a.Authenticate(bearer(tok)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	r, _ := http.NewRequest("GET", "/", nil)
	if _, err := a.Authenticate(r); err == nil {
		t.Fatal("missing header accepted")
	}
}
