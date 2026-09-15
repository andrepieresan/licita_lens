package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"licitalens.dev/backend/internal/auth"
	"licitalens.dev/backend/internal/store"
)

func TestProfilesAreIsolatedByOrganization(t *testing.T) {
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), true)
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
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), true)
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
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), true)
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
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), true)
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
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), true)
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
	handler := New(store.NewMemory(), nil, nil, nil, auth.NewLocalJWTFromEnv(), slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	request := httptest.NewRequest(http.MethodOptions, "/v1/opportunities", nil)
	request.Header.Set("Origin", "http://localhost:8082")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("preflight returned %d: %#v", response.Code, response.Header())
	}
}
