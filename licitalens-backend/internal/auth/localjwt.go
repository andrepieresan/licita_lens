package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

type LocalJWT struct {
	secret []byte
	issuer string
}

func NewLocalJWTFromEnv() *LocalJWT {
	secret := strings.TrimSpace(os.Getenv("AUTH_JWT_SECRET"))
	if secret == "" {
		secret = "licitalens-local-dev-secret-change-me"
	}
	issuer := strings.TrimSpace(os.Getenv("AUTH_JWT_ISSUER"))
	if issuer == "" {
		issuer = "licitalens-auth"
	}
	return &LocalJWT{secret: []byte(secret), issuer: issuer}
}

func (l *LocalJWT) Issue(subject, email string, ttl time.Duration) (string, error) {
	if subject == "" || email == "" {
		return "", errors.New("invalid token subject")
	}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{
		"sub": subject,
		"email": email,
		"iss": l.issuer,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

func (l *LocalJWT) Authenticate(_ context.Context, token string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("invalid token format")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, errors.New("invalid token payload")
	}
	var claims struct {
		Subject string `json:"sub"`
		Issuer  string `json:"iss"`
		Expires int64  `json:"exp"`
		Email   string `json:"email"`
	}
	if json.Unmarshal(payloadJSON, &claims) != nil || claims.Subject == "" {
		return Identity{}, errors.New("invalid token claims")
	}
	if claims.Issuer != l.issuer || time.Now().UTC().Unix() >= claims.Expires {
		return Identity{}, errors.New("token claims rejected")
	}
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, errors.New("invalid token signature")
	}
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(signingInput))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return Identity{}, errors.New("invalid token signature")
	}
	return Identity{Subject: claims.Subject, Roles: []string{"owner"}}, nil
}

type Chain struct {
	providers []Authenticator
}

func ChainAuthenticators(providers ...Authenticator) *Chain {
	filtered := make([]Authenticator, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &Chain{providers: filtered}
}

func (c *Chain) Authenticate(ctx context.Context, token string) (Identity, error) {
	if strings.TrimSpace(token) == "" {
		return Identity{}, errors.New("missing token")
	}
	var last error
	for _, provider := range c.providers {
		identity, err := provider.Authenticate(ctx, token)
		if err == nil {
			return identity, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("no authenticator configured")
	}
	return Identity{}, last
}
