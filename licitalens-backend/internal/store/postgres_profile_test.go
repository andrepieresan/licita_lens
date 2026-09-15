//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"licitalens.dev/backend/internal/domain"
)

func TestPutProfileIntegration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	p, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	var orgID string
	err = p.pool.QueryRow(ctx, `SELECT id::text FROM tenancy.organizations ORDER BY created_at DESC LIMIT 1`).Scan(&orgID)
	if err != nil {
		t.Fatal(err)
	}

	var id [16]byte
	_, _ = rand.Read(id[:])
	profileID := hex.EncodeToString(id[:])

	profile := domain.CommercialProfile{
		ID:             profileID,
		OrganizationID: orgID,
		Name:           "integration test",
		Description:    "desc",
		States:         []string{"PR"},
		CreatedAt:      time.Now().UTC(),
	}
	if err := p.PutProfile(profile); err != nil {
		t.Fatalf("PutProfile: %v", err)
	}
	t.Cleanup(func() {
		_ = p.DeleteProfile(profileID, orgID)
	})
}
