package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestSubscriptionIgnoresOutOfOrderEvent(t *testing.T) {
	now := time.Now().UTC()
	current := Subscription{ExternalID: "sub", Plan: "pro", Status: "active", UpdatedAt: now}
	next, changed, err := current.Apply(Event{ExternalID: "sub", Plan: "essential", Status: "canceled", CreatedAt: now.Add(-time.Minute)})
	if err != nil || changed || next.Status != "active" {
		t.Fatalf("out-of-order event changed state: %#v %v", next, err)
	}
}

func TestListSubscriptionsBuildsReconciliationRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/subscriptions" || r.URL.Query().Get("status") != "all" {
			t.Fatalf("unexpected Stripe request: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer sk_test" {
			t.Fatal("missing Stripe authorization")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"sub_1","status":"active","current_period_end":1900000000,"metadata":{"organization_id":"org_1"},"items":{"data":[{"price":{"lookup_key":"pro"}}]}}],"has_more":false}`))
	}))
	defer server.Close()
	client := &Stripe{SecretKey: "sk_test", HTTP: server.Client(), APIBaseURL: server.URL}
	items, err := client.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OrganizationID != "org_1" || items[0].Subscription.Plan != "pro" || items[0].Subscription.CurrentPeriodEnd == nil {
		t.Fatalf("unexpected reconciliation records: %#v", items)
	}
}

func TestVerifyStripeSignature(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	payload, secret := []byte(`{"id":"evt_1"}`), "secret"
	value := strconv.FormatInt(now.Unix(), 10) + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	header := "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	if err := VerifyStripeSignature(payload, header, secret, now, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStripeSignature([]byte("tampered"), header, secret, now, 5*time.Minute); err == nil {
		t.Fatal("expected invalid signature")
	}
}
