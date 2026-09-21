package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"licitalens.dev/backend/internal/auth"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

func TestProfilesAreIsolatedByOrganization(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	create := httptest.NewRequest(http.MethodPost, "/v1/profiles", bytes.NewBufferString(`{"name":"TI","description":"Notebooks"}`))
	create.Header.Set("X-Organization-ID", "org-a")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/v1/profiles", nil)
	list.Header.Set("X-Organization-ID", "org-b")
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || listed.Body.String() != "{\"data\":[],\"next_cursor\":null}\n" {
		t.Fatalf("data leaked: %s", listed.Body.String())
	}
}

func TestProfileCanBeUpdatedWithoutChangingTenant(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	create := httptest.NewRequest(http.MethodPost, "/v1/profiles", bytes.NewBufferString(`{"name":"TI","description":"Notebooks"}`))
	create.Header.Set("X-Organization-ID", "org-a")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	var profile struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	update := httptest.NewRequest(http.MethodPut, "/v1/profiles/"+profile.ID, bytes.NewBufferString(`{"name":"Infraestrutura","description":"Servidores e suporte"}`))
	update.Header.Set("X-Organization-ID", "org-a")
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, update)
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte(`"organization_id":"org-a"`)) {
		t.Fatalf("update returned %d: %s", updated.Code, updated.Body.String())
	}
}

func TestDealStagePatchPreservesTitle(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	create := httptest.NewRequest(http.MethodPost, "/v1/deals", bytes.NewBufferString(`{"title":"Notebook municipal","buyer_name":"Prefeitura","stage":"prospecting"}`))
	create.Header.Set("X-Organization-ID", "org-crm")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}
	var deal struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &deal); err != nil {
		t.Fatal(err)
	}
	patch := httptest.NewRequest(http.MethodPatch, "/v1/deals/"+deal.ID, bytes.NewBufferString(`{"stage":"won"}`))
	patch.Header.Set("X-Organization-ID", "org-crm")
	patched := httptest.NewRecorder()
	handler.ServeHTTP(patched, patch)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch returned %d: %s", patched.Code, patched.Body.String())
	}
	if !bytes.Contains(patched.Body.Bytes(), []byte(`"title":"Notebook municipal"`)) {
		t.Fatalf("title lost on stage patch: %s", patched.Body.String())
	}
}

func TestDealCreateIsIdempotentByOpportunity(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	body := `{"title":"Notebook municipal","buyer_name":"Prefeitura","stage":"analysis","opportunity_id":"opp-123"}`
	first := httptest.NewRequest(http.MethodPost, "/v1/deals", bytes.NewBufferString(body))
	first.Header.Set("X-Organization-ID", "org-crm")
	firstResp := httptest.NewRecorder()
	handler.ServeHTTP(firstResp, first)
	if firstResp.Code != http.StatusCreated {
		t.Fatalf("first create returned %d: %s", firstResp.Code, firstResp.Body.String())
	}
	second := httptest.NewRequest(http.MethodPost, "/v1/deals", bytes.NewBufferString(body))
	second.Header.Set("X-Organization-ID", "org-crm")
	secondResp := httptest.NewRecorder()
	handler.ServeHTTP(secondResp, second)
	if secondResp.Code != http.StatusOK {
		t.Fatalf("second create returned %d: %s", secondResp.Code, secondResp.Body.String())
	}
	if firstResp.Body.String() != secondResp.Body.String() {
		t.Fatalf("expected same deal payload: %s vs %s", firstResp.Body.String(), secondResp.Body.String())
	}
}

func TestNotificationPreferencesRoundTrip(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	get := httptest.NewRequest(http.MethodGet, "/v1/account/notification-preferences", nil)
	get.Header.Set("X-Organization-ID", "org-prefs")
	getResp := httptest.NewRecorder()
	handler.ServeHTTP(getResp, get)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get returned %d: %s", getResp.Code, getResp.Body.String())
	}
	patch := httptest.NewRequest(http.MethodPatch, "/v1/account/notification-preferences", bytes.NewBufferString(`{"push":false,"email":true,"whatsapp":false,"deadline_reminder":true}`))
	patch.Header.Set("X-Organization-ID", "org-prefs")
	patchResp := httptest.NewRecorder()
	handler.ServeHTTP(patchResp, patch)
	if patchResp.Code != http.StatusOK || !bytes.Contains(patchResp.Body.Bytes(), []byte(`"push":false`)) {
		t.Fatalf("patch returned %d: %s", patchResp.Code, patchResp.Body.String())
	}
}

func TestDemoCORSPreflight(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	request := httptest.NewRequest(http.MethodOptions, "/v1/opportunities", nil)
	request.Header.Set("Origin", "http://localhost:8082")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("preflight returned %d: %#v", response.Code, response.Header())
	}
}

func TestOpportunityPaginationReturnsNextCursor(t *testing.T) {
	memory := store.NewMemory()
	for i := 0; i < 25; i++ {
		_ = memory.PutOpportunity(domain.Opportunity{ID: fmt.Sprintf("opp-%02d", i), Object: "Oportunidade", PublishedAt: time.Now().UTC().Add(time.Duration(i) * time.Second)})
	}
	handler := New(memory, nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	request := httptest.NewRequest(http.MethodGet, "/v1/opportunities?limit=20", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"next_cursor":"20"`)) {
		t.Fatalf("pagination returned %d: %s", response.Code, response.Body.String())
	}
}

func TestSelfHostedConfigDoesNotExposeBillingOrPublicSignupByDefault(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "self_hosted")
	request := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"deployment_mode":"self_hosted"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"billing":false`)) {
		t.Fatalf("unexpected config: %d %s", response.Code, response.Body.String())
	}

	signup := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", bytes.NewBufferString(`{}`))
	signupResponse := httptest.NewRecorder()
	handler.ServeHTTP(signupResponse, signup)
	if signupResponse.Code != http.StatusForbidden {
		t.Fatalf("expected public signup to be disabled: %d %s", signupResponse.Code, signupResponse.Body.String())
	}
}

func TestCloudRejectsExpiredTrial(t *testing.T) {
	server := &Server{cloud: true}
	past := time.Now().UTC().Add(-time.Minute)
	if server.subscriptionActive(billing.Subscription{Status: "trialing", CurrentPeriodEnd: &past}) {
		t.Fatal("expired trial must not authorize cloud access")
	}
	if !server.subscriptionActive(billing.Subscription{Status: "active"}) {
		t.Fatal("active subscription must authorize cloud access")
	}
}

func TestBrowserSessionUsesCookiesAndRequiresCSRFForMutations(t *testing.T) {
	t.Setenv("PUBLIC_SIGNUP", "true")
	jwt := auth.NewLocalJWTFromEnv()
	handler := New(store.NewMemory(), nil, jwt, nil, jwt, slog.New(slog.NewTextHandler(io.Discard, nil)), "self_hosted")
	signup := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", bytes.NewBufferString(`{"email":"owner@example.com","password":"password-strong","full_name":"Owner","organization_name":"Acme","legal_accepted":true,"terms_version":"test","privacy_version":"test"}`))
	signup.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signup)
	if response.Code != http.StatusCreated {
		t.Fatalf("signup returned %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var session, csrf *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		switch cookie.Name {
		case "licitalens_session":
			session = cookie
		case "licitalens_csrf":
			csrf = cookie
		}
	}
	if session == nil || csrf == nil || !session.HttpOnly || csrf.HttpOnly {
		t.Fatalf("invalid session cookies: %#v %#v", session, csrf)
	}

	withoutCSRF := httptest.NewRequest(http.MethodPost, "/v1/profiles", bytes.NewBufferString(`{"name":"TI","description":"notebooks"}`))
	withoutCSRF.Header.Set("X-Organization-ID", payload.Organization.ID)
	withoutCSRF.AddCookie(session)
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, withoutCSRF)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF returned %d: %s", blocked.Code, blocked.Body.String())
	}

	withCSRF := httptest.NewRequest(http.MethodPost, "/v1/profiles", bytes.NewBufferString(`{"name":"TI","description":"notebooks"}`))
	withCSRF.Header.Set("X-Organization-ID", payload.Organization.ID)
	withCSRF.Header.Set("X-CSRF-Token", csrf.Value)
	withCSRF.AddCookie(session)
	withCSRF.AddCookie(csrf)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, withCSRF)
	if created.Code != http.StatusCreated {
		t.Fatalf("mutation with CSRF returned %d: %s", created.Code, created.Body.String())
	}
	export := httptest.NewRequest(http.MethodGet, "/v1/account/export", nil)
	export.AddCookie(session)
	exported := httptest.NewRecorder()
	handler.ServeHTTP(exported, export)
	if exported.Code != http.StatusOK || !bytes.Contains(exported.Body.Bytes(), []byte(`"profiles":[{`)) || !bytes.Contains(exported.Body.Bytes(), []byte(`"terms_version":"test"`)) {
		t.Fatalf("incomplete export: %d %s", exported.Code, exported.Body.String())
	}

	logout := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logout.Header.Set("X-CSRF-Token", csrf.Value)
	logout.AddCookie(session)
	logout.AddCookie(csrf)
	loggedOut := httptest.NewRecorder()
	handler.ServeHTTP(loggedOut, logout)
	if loggedOut.Code != http.StatusNoContent {
		t.Fatalf("logout returned %d: %s", loggedOut.Code, loggedOut.Body.String())
	}
	reused := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	reused.AddCookie(session)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, reused)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session returned %d: %s", denied.Code, denied.Body.String())
	}
}

func TestSelfHostedSignupRequiresRecordedLegalAcceptance(t *testing.T) {
	t.Setenv("PUBLIC_SIGNUP", "true")
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "self_hosted")
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", bytes.NewBufferString(`{"email":"legal@example.com","password":"password-strong","full_name":"Legal","organization_name":"Legal Org"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("signup without legal acceptance returned %d: %s", response.Code, response.Body.String())
	}
}

func TestAccountDeletionRequiresConfirmationAndRevokesAccess(t *testing.T) {
	t.Setenv("PUBLIC_SIGNUP", "true")
	jwt := auth.NewLocalJWTFromEnv()
	handler := New(store.NewMemory(), nil, jwt, nil, jwt, slog.New(slog.NewTextHandler(io.Discard, nil)), "self_hosted")
	signup := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", bytes.NewBufferString(`{"email":"delete@example.com","password":"password-strong","full_name":"Delete","organization_name":"Delete Org","legal_accepted":true,"terms_version":"test","privacy_version":"test"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signup)
	var session, csrf *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "licitalens_session" {
			session = cookie
		}
		if cookie.Name == "licitalens_csrf" {
			csrf = cookie
		}
	}
	bad := httptest.NewRequest(http.MethodDelete, "/v1/account", bytes.NewBufferString(`{"confirmation":"NO"}`))
	bad.Header.Set("X-CSRF-Token", csrf.Value)
	bad.AddCookie(session)
	bad.AddCookie(csrf)
	badResponse := httptest.NewRecorder()
	handler.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("delete without confirmation: %d", badResponse.Code)
	}
	request := httptest.NewRequest(http.MethodDelete, "/v1/account", bytes.NewBufferString(`{"confirmation":"DELETE"}`))
	request.Header.Set("X-CSRF-Token", csrf.Value)
	request.AddCookie(session)
	request.AddCookie(csrf)
	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, request)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	reuse := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	reuse.AddCookie(session)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, reuse)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("deleted account retained access: %d", denied.Code)
	}
}

func TestAuthenticationAttemptsAreRateLimited(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	for i := 0; i < 10; i++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(`{}`))
		request.RemoteAddr = "203.0.113.10:1234"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d: %s", i, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(`{}`))
	request.RemoteAddr = "203.0.113.10:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("unlimited authentication attempts: %d %s", response.Code, response.Body.String())
	}
}

func TestAuthRateKeyUsesForwardedAddressOnlyFromPrivateProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	request.RemoteAddr = "172.18.0.5:4321"
	request.Header.Set("X-Forwarded-For", "203.0.113.20, 172.18.0.5")
	if key := authRateKey(request); key != "203.0.113.20" {
		t.Fatalf("private proxy key = %q", key)
	}

	request.RemoteAddr = "198.51.100.10:4321"
	request.Header.Set("X-Forwarded-For", "203.0.113.21")
	if key := authRateKey(request); key != "198.51.100.10" {
		t.Fatalf("public peer trusted forwarded address: %q", key)
	}
}

func TestPasswordRecoveryTokenIsSingleUse(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	signup := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", bytes.NewBufferString(`{"email":"recover@example.com","password":"old-password","full_name":"Recover","organization_name":"Recover Org","plan":"essential"}`))
	signupResponse := httptest.NewRecorder()
	handler.ServeHTTP(signupResponse, signup)
	if signupResponse.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", signupResponse.Code, signupResponse.Body.String())
	}
	recovery := httptest.NewRequest(http.MethodPost, "/v1/auth/password-recovery", bytes.NewBufferString(`{"email":"recover@example.com"}`))
	recoveryResponse := httptest.NewRecorder()
	handler.ServeHTTP(recoveryResponse, recovery)
	var recovered struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(recoveryResponse.Body.Bytes(), &recovered)
	if recovered.Token == "" {
		t.Fatalf("missing demo recovery token: %s", recoveryResponse.Body.String())
	}
	body := fmt.Sprintf(`{"token":%q,"password":"new-password"}`, recovered.Token)
	reset := httptest.NewRequest(http.MethodPost, "/v1/auth/password-reset", bytes.NewBufferString(body))
	resetResponse := httptest.NewRecorder()
	handler.ServeHTTP(resetResponse, reset)
	if resetResponse.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", resetResponse.Code, resetResponse.Body.String())
	}
	reuse := httptest.NewRequest(http.MethodPost, "/v1/auth/password-reset", bytes.NewBufferString(body))
	reuseResponse := httptest.NewRecorder()
	handler.ServeHTTP(reuseResponse, reuse)
	if reuseResponse.Code != http.StatusBadRequest {
		t.Fatalf("reused token: %d", reuseResponse.Code)
	}
}

func TestCloudLoginRequiresVerifiedEmail(t *testing.T) {
	memory := store.NewMemory()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password-strong"), bcrypt.DefaultCost)
	account, _, _, err := memory.RegisterSaaSAccount(t.Context(), "verify@example.com", string(hash), "Verify", "Verify Org", "essential", domain.LegalAcceptance{})
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewLocalJWTFromEnv()
	handler := New(memory, nil, jwt, nil, jwt, slog.New(slog.NewTextHandler(io.Discard, nil)), "cloud")
	token, _ := jwt.Issue(account.SubjectID, account.Email, time.Hour)
	protected := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	protected.Header.Set("Authorization", "Bearer "+token)
	protectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(protectedResponse, protected)
	if protectedResponse.Code != http.StatusForbidden {
		t.Fatalf("unverified token accessed API: %d", protectedResponse.Code)
	}
	login := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(`{"email":"verify@example.com","password":"password-strong"}`))
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp
	}
	if response := login(); response.Code != http.StatusForbidden {
		t.Fatalf("unverified login: %d %s", response.Code, response.Body.String())
	}
	if err := memory.MarkEmailVerified(t.Context(), account.SubjectID); err != nil {
		t.Fatal(err)
	}
	if response := login(); response.Code != http.StatusOK {
		t.Fatalf("verified login: %d %s", response.Code, response.Body.String())
	}
}

func TestCloudSignupRequiresVerificationWithoutIssuingSession(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "cloud")
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/signup", bytes.NewBufferString(`{"email":"new-cloud@example.com","password":"password-strong","full_name":"Cloud","organization_name":"Cloud Org","plan":"essential","legal_accepted":true,"terms_version":"test","privacy_version":"test"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !bytes.Contains(response.Body.Bytes(), []byte(`"verification_required":true`)) {
		t.Fatalf("cloud signup: %d %s", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "licitalens_session" {
			t.Fatal("cloud signup issued session before verification")
		}
	}
}

func TestAdminBillingReconciliationAppliesStripeState(t *testing.T) {
	stripeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"sub_reconciled","status":"past_due","current_period_end":1900000000,"metadata":{"organization_id":"org_reconciled","plan":"essential"},"items":{"data":[]}}],"has_more":false}`))
	}))
	defer stripeAPI.Close()
	t.Setenv("ADMIN_API_KEY", "admin-key-for-test-123456")
	memory := store.NewMemory()
	stripe := &billing.Stripe{SecretKey: "sk_test", HTTP: stripeAPI.Client(), APIBaseURL: stripeAPI.URL}
	handler := New(memory, nil, nil, stripe, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "cloud")
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/billing/reconcile", nil)
	request.Header.Set("X-Admin-Key", "admin-key-for-test-123456")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("reconcile: %d %s", response.Code, response.Body.String())
	}
	subscription, err := memory.Subscription(t.Context(), "org_reconciled")
	if err != nil || subscription.Status != "past_due" || subscription.ExternalID != "sub_reconciled" {
		t.Fatalf("reconciliation was not persisted: %#v %v", subscription, err)
	}
}

func TestMetricsReportsBackupStatus(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "backup-status")
	if err := os.WriteFile(statusPath, []byte("2026-09-18T12:00:00Z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKUP_STATUS_FILE", statusPath)
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), "demo")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("licitalens_backup_configured 1")) || !bytes.Contains(response.Body.Bytes(), []byte("licitalens_backup_last_success_timestamp_seconds 1789732800")) {
		t.Fatalf("unexpected metrics: %d %s", response.Code, response.Body.String())
	}
}
