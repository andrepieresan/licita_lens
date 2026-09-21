package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

func TestPostgresAccountLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if !strings.Contains(strings.ToLower(url), "_test") {
		t.Fatal("TEST_DATABASE_URL must target an isolated _test database")
	}
	ctx := context.Background()
	db, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, current, _, _ := runtime.Caller(0)
	schema, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "..", "deploy", "self-hosted", "001_schema.psql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	email := "account-" + uuid.NewString() + "@example.com"
	password, _ := bcrypt.GenerateFromPassword([]byte("initial-password"), bcrypt.DefaultCost)
	account, organization, _, err := db.RegisterSaaSAccount(ctx, email, string(password), "Integration User", "Integration Org", "essential", domain.LegalAcceptance{TermsVersion: "test-terms", PrivacyVersion: "test-privacy", AcceptedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.DeleteAccount(context.Background(), account.SubjectID) })
	if verified, err := db.EmailVerified(ctx, account.SubjectID); err != nil || verified {
		t.Fatalf("unexpected verification state: %v %v", verified, err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		count, failureErr := db.RecordNotificationFailure(ctx, organization.ID, "opportunity", "integration-alert", email, "provider unavailable")
		if failureErr != nil || count != attempt {
			t.Fatalf("notification attempt %d: count=%d err=%v", attempt, count, failureErr)
		}
	}
	if blocked, blockErr := db.NotificationAlertSent(ctx, organization.ID, "opportunity", "integration-alert"); blockErr != nil || !blocked {
		t.Fatalf("suppressed alert not blocked: %v %v", blocked, blockErr)
	}
	now := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	if changed, applyErr := db.ApplySubscription(ctx, organization.ID, billing.Subscription{ExternalID: "sub-" + uuid.NewString(), Plan: "pro", Status: "active", UpdatedAt: now}, "evt-"+uuid.NewString(), "payload"); applyErr != nil || !changed {
		t.Fatalf("subscription update: changed=%v err=%v", changed, applyErr)
	}
	if changed, applyErr := db.ApplySubscription(ctx, organization.ID, billing.Subscription{ExternalID: "sub-old", Plan: "essential", Status: "canceled", UpdatedAt: now.Add(-time.Minute)}, "evt-"+uuid.NewString(), "payload"); applyErr != nil || changed {
		t.Fatalf("out-of-order subscription update: changed=%v err=%v", changed, applyErr)
	}
	if subscription, subscriptionErr := db.Subscription(ctx, organization.ID); subscriptionErr != nil || subscription.Plan != "pro" || subscription.Status != "active" {
		t.Fatalf("out-of-order event changed persisted subscription: %#v %v", subscription, subscriptionErr)
	}
	if err := db.CreateAccountToken(ctx, email, "verify_email", "hash-"+uuid.NewString(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	staleHash := "reset-" + uuid.NewString()
	if err := db.CreateAccountToken(ctx, email, "password_reset", staleHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	resetHash := "reset-" + uuid.NewString()
	if err := db.CreateAccountToken(ctx, email, "password_reset", resetHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ConsumeAccountToken(ctx, "password_reset", staleHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("older token remained active: %v", err)
	}
	consumed, err := db.ConsumeAccountToken(ctx, "password_reset", resetHash)
	if err != nil || consumed.SubjectID != account.SubjectID {
		t.Fatalf("consume token: %#v %v", consumed, err)
	}
	if _, err := db.ConsumeAccountToken(ctx, "password_reset", resetHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("token reused: %v", err)
	}
	if err := db.MarkEmailVerified(ctx, account.SubjectID); err != nil {
		t.Fatal(err)
	}
	if verified, err := db.EmailVerified(ctx, account.SubjectID); err != nil || !verified {
		t.Fatalf("email not verified: %v %v", verified, err)
	}
	newPassword, _ := bcrypt.GenerateFromPassword([]byte("updated-password"), bcrypt.DefaultCost)
	if err := db.UpdatePassword(ctx, account.SubjectID, string(newPassword)); err != nil {
		t.Fatal(err)
	}
	_, storedHash, err := db.AccountByEmail(ctx, email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("updated-password")) != nil {
		t.Fatal("password was not updated")
	}
	if err := db.DeleteAccount(ctx, account.SubjectID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AccountByEmail(ctx, email); !errors.Is(err, ErrNotFound) {
		t.Fatalf("account still exists: %v", err)
	}
	if organizations, err := db.OrganizationsForSubject(ctx, account.SubjectID); err != nil || len(organizations) != 0 {
		t.Fatalf("organizations remain after delete: %#v %v (%s)", organizations, err, organization.ID)
	}
}
