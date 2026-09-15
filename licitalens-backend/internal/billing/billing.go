package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

type Entitlements struct{ Profiles, MonthlyAI, DailyAlerts int }

var plans = map[string]Entitlements{"essential": {Profiles: 1, MonthlyAI: 30, DailyAlerts: 20}, "pro": {Profiles: 5, MonthlyAI: 200, DailyAlerts: 100}}

func Plan(name string) (Entitlements, bool) {
	value, ok := plans[strings.ToLower(name)]
	return value, ok
}

type Subscription struct {
	ExternalID string    `json:"external_id,omitempty"`
	Plan       string    `json:"plan"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type Event struct {
	ExternalID, Plan, Status string
	CreatedAt                time.Time
}

func (s Subscription) Apply(event Event) (Subscription, bool, error) {
	if event.ExternalID == "" || event.CreatedAt.IsZero() {
		return s, false, errors.New("invalid subscription event")
	}
	if !s.UpdatedAt.IsZero() && !event.CreatedAt.After(s.UpdatedAt) {
		return s, false, nil
	}
	if _, ok := Plan(event.Plan); !ok {
		return s, false, errors.New("unknown plan")
	}
	switch event.Status {
	case "active", "past_due", "canceled", "trialing":
	default:
		return s, false, errors.New("unknown subscription status")
	}
	return Subscription{ExternalID: event.ExternalID, Plan: event.Plan, Status: event.Status, UpdatedAt: event.CreatedAt}, true, nil
}

func VerifyStripeSignature(payload []byte, header, secret string, now time.Time, tolerance time.Duration) error {
	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		if pair[0] == "t" {
			timestamp, _ = strconv.ParseInt(pair[1], 10, 64)
		}
		if pair[0] == "v1" {
			signatures = append(signatures, pair[1])
		}
	}
	if timestamp == 0 || len(signatures) == 0 || secret == "" {
		return errors.New("invalid stripe signature header")
	}
	eventTime := time.Unix(timestamp, 0)
	if now.Sub(eventTime) > tolerance || eventTime.Sub(now) > tolerance {
		return errors.New("stripe signature timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		candidate, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(expected, candidate) {
			return nil
		}
	}
	return errors.New("invalid stripe signature")
}
