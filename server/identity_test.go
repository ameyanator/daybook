package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
)

// These tests sign real RS256 JWTs with a local key, the same shape Google
// and Apple issue, so the actual verification path is exercised.

func signToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	obj, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOIDCVerifier(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	keys := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{key.Public()}}
	google := newOIDCVerifier("google", googleIssuer, keys, []string{"web-client.apps.googleusercontent.com"})
	apple := newOIDCVerifier("apple", appleIssuer, keys, []string{"com.ameyanator.daybook"})

	now := time.Now()
	base := func(iss, aud string) map[string]any {
		return map[string]any{
			"iss": iss, "aud": aud, "sub": "user-123",
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
			"email": "a@example.com", "email_verified": true, "name": "A",
		}
	}
	with := func(c map[string]any, k string, v any) map[string]any { c[k] = v; return c }
	ctx := context.Background()

	tests := []struct {
		name      string
		v         IdentityVerifier
		token     string
		wantErr   bool
		wantEmail string
	}{
		{"google ok", google, signToken(t, key, base(googleIssuer, "web-client.apps.googleusercontent.com")), false, "a@example.com"},
		{"google issuer without scheme", google, signToken(t, key, base("accounts.google.com", "web-client.apps.googleusercontent.com")), false, "a@example.com"},
		{"wrong audience", google, signToken(t, key, base(googleIssuer, "someone-elses-app")), true, ""},
		{"wrong issuer", google, signToken(t, key, base(appleIssuer, "web-client.apps.googleusercontent.com")), true, ""},
		{"expired", google, signToken(t, key, with(base(googleIssuer, "web-client.apps.googleusercontent.com"), "exp", now.Add(-time.Hour).Unix())), true, ""},
		{"signed by unknown key", google, signToken(t, otherKey, base(googleIssuer, "web-client.apps.googleusercontent.com")), true, ""},
		{"unverified email dropped", google, signToken(t, key, with(base(googleIssuer, "web-client.apps.googleusercontent.com"), "email_verified", false)), false, ""},
		{"apple ok, string bool", apple, signToken(t, key, with(base(appleIssuer, "com.ameyanator.daybook"), "email_verified", "true")), false, "a@example.com"},
		{"garbage", google, "not.a.jwt", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := tc.v.Verify(ctx, tc.token)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && (id.Subject != "user-123" || id.Email != tc.wantEmail) {
				t.Errorf("identity = %+v", id)
			}
		})
	}
}
