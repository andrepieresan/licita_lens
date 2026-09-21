package httpapi

import (
	"context"
	"net/http"
	"time"
)

const userKey contextKey = "user"
const issuedAtKey contextKey = "issued_at"

func subject(r *http.Request) string {
	value, _ := r.Context().Value(userKey).(string)
	return value
}

func withSubject(ctx context.Context, subjectID string) context.Context {
	return context.WithValue(ctx, userKey, subjectID)
}

func withIssuedAt(ctx context.Context, issuedAt time.Time) context.Context {
	return context.WithValue(ctx, issuedAtKey, issuedAt)
}

func issuedAt(r *http.Request) time.Time {
	value, _ := r.Context().Value(issuedAtKey).(time.Time)
	return value
}

func subscriptionExempt(path string) bool {
	switch path {
	case "/v1/account/organizations", "/v1/account/bootstrap", "/v1/billing/checkout", "/v1/billing/portal", "/v1/billing/history", "/v1/me", "/v1/account/notification-preferences", "/v1/account/push-token":
		return true
	default:
		return false
	}
}
