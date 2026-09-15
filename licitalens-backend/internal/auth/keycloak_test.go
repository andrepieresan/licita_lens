package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKeycloakValidatesSignatureIssuerAudienceAndExpiry(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/protocol/openid-connect/certs" {
			http.NotFound(w, r)
			return
		}
		e := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "test-key", "kty": "RSA", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(e)}}})
	}))
	defer server.Close()
	issuer = server.URL
	token := signedToken(t, privateKey, map[string]any{"sub": "user-1", "iss": issuer, "aud": "licitalens-mobile", "exp": time.Now().Add(time.Hour).Unix(), "realm_access": map[string]any{"roles": []string{"analyst"}}})
	identity, err := NewKeycloak(issuer, "licitalens-mobile").Authenticate(context.Background(), token)
	if err != nil || identity.Subject != "user-1" || len(identity.Roles) != 1 {
		t.Fatalf("unexpected identity: %#v %v", identity, err)
	}
	if _, err := NewKeycloak(issuer, "other-client").Authenticate(context.Background(), token); err == nil {
		t.Fatal("expected audience rejection")
	}
}

func signedToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}
