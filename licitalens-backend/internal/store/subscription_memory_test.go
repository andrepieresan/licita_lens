package store

import (
	"context"
	"testing"
	"time"

	"licitalens.dev/backend/internal/billing"
)

func TestMemorySubscriptionIgnoresOutOfOrderUpdate(t *testing.T) {
	memory := NewMemory()
	organizationID := "organization"
	now := time.Now().UTC()
	if changed, err := memory.ApplySubscription(context.Background(), organizationID, billing.Subscription{ExternalID: "sub", Plan: "pro", Status: "active", UpdatedAt: now}, "evt_new", "hash"); err != nil || !changed {
		t.Fatalf("initial subscription update failed: changed=%v err=%v", changed, err)
	}
	changed, err := memory.ApplySubscription(context.Background(), organizationID, billing.Subscription{ExternalID: "sub", Plan: "essential", Status: "canceled", UpdatedAt: now.Add(-time.Minute)}, "evt_old", "hash")
	if err != nil || changed {
		t.Fatalf("out-of-order update must be ignored: changed=%v err=%v", changed, err)
	}
	current, err := memory.Subscription(context.Background(), organizationID)
	if err != nil || current.Plan != "pro" || current.Status != "active" {
		t.Fatalf("out-of-order update changed subscription: %#v %v", current, err)
	}
	changed, err = memory.ApplySubscription(context.Background(), organizationID, billing.Subscription{ExternalID: "sub", Plan: "pro", Status: "active", UpdatedAt: now.Add(time.Minute)}, "evt_new", "hash")
	if err != nil || changed {
		t.Fatalf("duplicate event must be ignored: changed=%v err=%v", changed, err)
	}
}
