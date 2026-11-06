package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Identity is who Google or Apple says the person is, taken from a verified
// ID token.
type Identity struct {
	Provider string
	Subject  string
	Email    string
	Name     string
}

// IdentityVerifier checks an ID token's signature, issuer, audience and expiry.
type IdentityVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (Identity, error)
}

type oidcVerifier struct {
	provider  string
	verifier  *oidc.IDTokenVerifier
	audiences []string
}

const (
	googleIssuer = "https://accounts.google.com"
	googleJWKS   = "https://www.googleapis.com/oauth2/v3/certs"
	appleIssuer  = "https://appleid.apple.com"
	appleJWKS    = "https://appleid.apple.com/auth/keys"
)

// NewGoogleVerifier accepts tokens issued to any of the given OAuth client IDs
// (the app sends tokens whose audience is the "web" client ID).
func NewGoogleVerifier(ctx context.Context, clientIDs []string) IdentityVerifier {
	return newOIDCVerifier("google", googleIssuer, oidc.NewRemoteKeySet(ctx, googleJWKS), clientIDs)
}

// NewAppleVerifier accepts native Sign in with Apple tokens, whose audience is
// the iOS bundle identifier.
func NewAppleVerifier(ctx context.Context, bundleIDs []string) IdentityVerifier {
	return newOIDCVerifier("apple", appleIssuer, oidc.NewRemoteKeySet(ctx, appleJWKS), bundleIDs)
}

func newOIDCVerifier(provider, issuer string, keys oidc.KeySet, audiences []string) *oidcVerifier {
	return &oidcVerifier{
		provider: provider,
		// Audience is checked below against a list, which go-oidc doesn't support.
		verifier:  oidc.NewVerifier(issuer, keys, &oidc.Config{SkipClientIDCheck: true}),
		audiences: audiences,
	}
}

// flexBool decodes a claim Apple sometimes sends as "true" and sometimes as true.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	*b = string(data) == "true" || string(data) == `"true"`
	return nil
}

func (v *oidcVerifier) Verify(ctx context.Context, raw string) (Identity, error) {
	tok, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	if !slices.ContainsFunc(tok.Audience, func(a string) bool { return slices.Contains(v.audiences, a) }) {
		return Identity{}, fmt.Errorf("token audience %v not accepted", tok.Audience)
	}
	var claims struct {
		Email         string   `json:"email"`
		EmailVerified flexBool `json:"email_verified"`
		Name          string   `json:"name"`
	}
	if err := tok.Claims(&claims); err != nil {
		return Identity{}, err
	}
	if tok.Subject == "" {
		return Identity{}, errors.New("token has no subject")
	}
	id := Identity{Provider: v.provider, Subject: tok.Subject, Name: claims.Name}
	// Only trust emails the provider has verified; sharing is looked up by email.
	if claims.EmailVerified {
		id.Email = claims.Email
	}
	return id, nil
}
