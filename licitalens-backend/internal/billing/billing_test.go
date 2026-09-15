package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
