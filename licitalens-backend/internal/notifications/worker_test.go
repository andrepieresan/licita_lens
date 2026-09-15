package notifications

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

func TestWorkerSendsMatchedOpportunityEmail(t *testing.T) {
	mem := store.NewMemory()
	store.SeedDemo(mem)
	_, org, _, err := mem.RegisterSaaSAccount(context.Background(), "alerts@test.com", "hash", "Alerts User", "Alerts Org", "pro")
	if err != nil {
		t.Fatal(err)
	}
	mem.PutProfile(domain.CommercialProfile{
		ID:             "profile-1",
		OrganizationID: org.ID,
		Name:           "TI",
		Description:    "notebooks corporativos",
		Keywords:       []string{"notebooks"},
		States:         []string{"PR"},
		CreatedAt:      time.Now().UTC(),
	})
	worker := NewWorker(mem, slog.New(slog.NewTextHandler(io.Discard, nil)), WorkerConfig{
		DemoMode:       true,
		SMTP:           SMTPConfig{Host: ""},
		UseLogExpoPush: true,
	})
	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.OpportunitySent == 0 {
		t.Fatalf("expected opportunity alert, got %#v", result)
	}
}

func TestWorkerSkipsStaleAfterCursor(t *testing.T) {
	mem := store.NewMemory()
	store.SeedDemo(mem)
	_, org, _, err := mem.RegisterSaaSAccount(context.Background(), "alerts2@test.com", "hash", "Alerts User", "Alerts Org", "pro")
	if err != nil {
		t.Fatal(err)
	}
	mem.PutProfile(domain.CommercialProfile{
		ID:             "profile-2",
		OrganizationID: org.ID,
		Name:           "TI",
		Description:    "notebooks corporativos",
		Keywords:       []string{"notebooks"},
		States:         []string{"PR"},
		CreatedAt:      time.Now().UTC(),
	})
	cfg := WorkerConfig{DemoMode: true, SMTP: SMTPConfig{Host: ""}, UseLogExpoPush: true}
	worker := NewWorker(mem, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	first, err := worker.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.OpportunitySent == 0 {
		t.Fatalf("expected first cycle alert, got %#v", first)
	}
	second, err := worker.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.OpportunitySent != 0 {
		t.Fatalf("expected no second alert, got %#v", second)
	}
	if second.SkippedStale == 0 {
		t.Fatalf("expected stale skips, got %#v", second)
	}
}
