package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/isBioku/kubetre/internal/access"
)

// Login performs the OIDC authorization code flow.
type Login interface {
	AuthCodeURL(state, nonce, verifier string) string
	Exchange(ctx context.Context, code, verifier, nonce string) (access.Identity, error)
}

// OIDCLogin is the production Login, against any OIDC provider.
type OIDCLogin struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewOIDCLogin discovers the provider. clientSecret may be empty for a public client;
// PKCE is used either way.
func NewOIDCLogin(ctx context.Context, issuer, clientID, clientSecret, redirectURL string) (*OIDCLogin, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", issuer, err)
	}
	return &OIDCLogin{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

// AuthCodeURL returns the provider's sign-in URL with state, nonce and a PKCE challenge.
func (l *OIDCLogin) AuthCodeURL(state, nonce, verifier string) string {
	return l.config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

// Exchange redeems the code and verifies the ID token, including its nonce.
func (l *OIDCLogin) Exchange(ctx context.Context, code, verifier, nonce string) (access.Identity, error) {
	tok, err := l.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return access.Identity{}, fmt.Errorf("code exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return access.Identity{}, errors.New("no id_token in token response")
	}
	idt, err := l.verifier.Verify(ctx, raw)
	if err != nil {
		return access.Identity{}, fmt.Errorf("id token: %w", err)
	}
	if idt.Nonce != nonce {
		return access.Identity{}, errors.New("id token nonce mismatch")
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := idt.Claims(&claims); err != nil {
		return access.Identity{}, err
	}
	id := access.Identity{Subject: idt.Subject, Email: claims.Email, Name: claims.Name}
	if id.Email == "" {
		id.Email = claims.PreferredUsername
	}
	return id, nil
}
