package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"licitalens.dev/backend/internal/auth"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

func TestPlatformOperatorLoginAndUpdateSubscription(t *testing.T) {
	memory := store.NewMemory()
	hash, err := store.HashPassword("platform-secret")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := memory.RegisterPlatformOperator(t.Context(), "ops@licitalens.dev", hash, "Ops Team")
	if err != nil {
		t.Fatal(err)
	}
	accountHash, err := store.HashPassword("client-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, org, _, err := memory.RegisterAccount(t.Context(), "client@example.com", accountHash, "Client", "ACME", "essential", domain.LegalAcceptance{
		TermsVersion: "test", PrivacyVersion: "test", AcceptedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	jwt := auth.NewLocalJWTFromEnv()
	handler := New(memory, nil, jwt, nil, jwt, slog.New(slog.NewTextHandler(io.Discard, nil)), "self_hosted")

	login := httptest.NewRequest(http.MethodPost, "/v1/admin/auth/login", bytes.NewBufferString(`{"email":"ops@licitalens.dev","password":"platform-secret"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginPayload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginPayload); err != nil || loginPayload.Token == "" {
		t.Fatalf("login token missing: %v", err)
	}

	update := httptest.NewRequest(http.MethodPut, "/v1/admin/organizations/"+org.ID+"/subscription", bytes.NewBufferString(`{"plan":"pro","status":"active","note":"contrato consultivo"}`))
	update.Header.Set("Content-Type", "application/json")
	update.Header.Set("Authorization", "Bearer "+loginPayload.Token)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, update)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}

	subscription, err := memory.Subscription(t.Context(), org.ID)
	if err != nil || subscription.Plan != "pro" || subscription.Status != "active" {
		t.Fatalf("subscription not updated: %#v err=%v", subscription, err)
	}
	_ = operator.SubjectID
}
