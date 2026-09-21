package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Identity struct {
	Subject  string
	Roles    []string
	IssuedAt time.Time
}
type Authenticator interface {
	Authenticate(context.Context, string) (Identity, error)
}

type Keycloak struct {
	issuer, audience, jwksURL string
	http                      *http.Client
	mu                        sync.RWMutex
	keys                      map[string]*rsa.PublicKey
	expires                   time.Time
}

func NewKeycloak(issuer, audience string) *Keycloak {
	issuer = strings.TrimRight(issuer, "/")
	return &Keycloak{issuer: issuer, audience: audience, jwksURL: issuer + "/protocol/openid-connect/certs", http: &http.Client{Timeout: 10 * time.Second}, keys: map[string]*rsa.PublicKey{}}
}

func (k *Keycloak) Authenticate(ctx context.Context, token string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("invalid token format")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Identity{}, errors.New("invalid token header")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if json.Unmarshal(headerJSON, &header) != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return Identity{}, errors.New("unsupported token signature")
	}
	key, err := k.key(ctx, header.KeyID)
	if err != nil {
		return Identity{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, errors.New("invalid token signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return Identity{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, errors.New("invalid token payload")
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(payload, &raw) != nil {
		return Identity{}, errors.New("invalid token claims")
	}
	var subject, issuer string
	var expires int64
	var audience any
	var access struct {
		Roles []string `json:"roles"`
	}
	_ = json.Unmarshal(raw["sub"], &subject)
	_ = json.Unmarshal(raw["iss"], &issuer)
	_ = json.Unmarshal(raw["exp"], &expires)
	_ = json.Unmarshal(raw["aud"], &audience)
	_ = json.Unmarshal(raw["realm_access"], &access)
	if subject == "" || issuer != k.issuer || time.Now().Unix() >= expires || !hasAudience(audience, k.audience) {
		return Identity{}, errors.New("token claims rejected")
	}
	return Identity{Subject: subject, Roles: access.Roles}, nil
}

func (k *Keycloak) key(ctx context.Context, id string) (*rsa.PublicKey, error) {
	k.mu.RLock()
	key, ok := k.keys[id]
	fresh := time.Now().Before(k.expires)
	k.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}
	if err := k.refresh(ctx); err != nil {
		return nil, err
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	key, ok = k.keys[id]
	if !ok {
		return nil, errors.New("token key not found")
	}
	return key, nil
}

func (k *Keycloak) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch keycloak keys: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("keycloak keys returned %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			ID   string `json:"kid"`
			Type string `json:"kty"`
			Use  string `json:"use"`
			N    string `json:"n"`
			E    string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, item := range set.Keys {
		if item.Type != "RSA" || item.Use != "sig" {
			continue
		}
		nBytes, nErr := base64.RawURLEncoding.DecodeString(item.N)
		eBytes, eErr := base64.RawURLEncoding.DecodeString(item.E)
		if nErr != nil || eErr != nil {
			continue
		}
		e := 0
		for _, value := range eBytes {
			e = e<<8 + int(value)
		}
		keys[item.ID] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	if len(keys) == 0 {
		return errors.New("keycloak returned no signing keys")
	}
	k.mu.Lock()
	k.keys, k.expires = keys, time.Now().Add(15*time.Minute)
	k.mu.Unlock()
	return nil
}

func hasAudience(value any, expected string) bool {
	if expected == "" {
		return true
	}
	switch audience := value.(type) {
	case string:
		return audience == expected
	case []any:
		for _, item := range audience {
			if item == expected {
				return true
			}
		}
	}
	return false
}
