package notifications

import (
	"testing"
	"time"
)

func TestWhatsAppRequiresConsentAndHonorsOptOut(t *testing.T) {
	channel := Channel{Type: WhatsApp, Enabled: true, Destination: "5511999999999"}
	if channel.CanSend() {
		t.Fatal("WhatsApp without consent must be suppressed")
	}
	channel.ConsentedAt = time.Now()
	if !channel.CanSend() {
		t.Fatal("consented channel should send")
	}
	channel.OptedOutAt = time.Now()
	if channel.CanSend() {
		t.Fatal("opted-out channel must be suppressed")
	}
}
