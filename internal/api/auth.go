// Package api implements the KubeTRE HTTP API.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/isBioku/kubetre/internal/access"
)

// Identity is the authenticated caller.
type Identity = access.Identity

// ErrUnauthenticated means the request carried no valid credentials.
var ErrUnauthenticated = errors.New("unauthenticated")

// Authenticator turns a request into an Identity.
type Authenticator interface {
	Authenticate(r *http.Request) (Identity, error)
}

// OIDCAuthenticator validates bearer tokens from any OpenID Connect provider.
// All clients request tokens for a single audience; there is no per-workspace audience.
type OIDCAuthenticator struct {
	verifier   *oidc.IDTokenVerifier
	rolesClaim []string
}

// NewOIDCAuthenticator discovers the issuer's signing keys.
// rolesClaim is a dot-separated claim path, for example "roles" or "realm_access.roles".
func NewOIDCAuthenticator(ctx context.Context, issuer, audience, rolesClaim string) (*OIDCAuthenticator, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", issuer, err)
	}
	return newOIDCAuthenticator(provider.Verifier(&oidc.Config{ClientID: audience}), rolesClaim), nil
}

func newOIDCAuthenticator(v *oidc.IDTokenVerifier, rolesClaim string) *OIDCAuthenticator {
	return &OIDCAuthenticator{verifier: v, rolesClaim: strings.Split(rolesClaim, ".")}
}

// Authenticate verifies the bearer token's signature, issuer, audience and expiry.
func (a *OIDCAuthenticator) Authenticate(r *http.Request) (Identity, error) {
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || raw == "" {
		return Identity{}, ErrUnauthenticated
	}
	tok, err := a.verifier.Verify(r.Context(), raw)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	var claims map[string]any
	if err := tok.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	id := Identity{Subject: tok.Subject, Name: str(claims["name"]), Roles: stringsAt(claims, a.rolesClaim)}
	if id.Email = str(claims["email"]); id.Email == "" {
		id.Email = str(claims["preferred_username"])
	}
	return id, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func stringsAt(claims map[string]any, path []string) []string {
	var cur any = claims
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	list, _ := cur.([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// DevAuthenticator trusts identity headers. It exists only for local development
// and must never be exposed to a network.
type DevAuthenticator struct{}

// Authenticate reads X-Dev-User and the comma-separated X-Dev-Roles header.
func (DevAuthenticator) Authenticate(r *http.Request) (Identity, error) {
	user := r.Header.Get("X-Dev-User")
	if user == "" {
		return Identity{}, ErrUnauthenticated
	}
	var roles []string
	for _, role := range strings.Split(r.Header.Get("X-Dev-Roles"), ",") {
		if role = strings.TrimSpace(role); role != "" {
			roles = append(roles, role)
		}
	}
	return Identity{Subject: user, Email: user, Name: user, Roles: roles}, nil
}
