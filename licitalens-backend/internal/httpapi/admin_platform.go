package httpapi

import (
	"net/http"
	"strings"
	"time"

	"licitalens.dev/backend/internal/auth"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

func adminPublicPath(path string) bool {
	switch path {
	case "/v1/admin/auth/login":
		return true
	default:
		return false
	}
}

func (s *Server) adminAuthorized(r *http.Request) bool {
	expected := strings.TrimSpace(getenv("ADMIN_API_KEY"))
	if expected != "" && r.Header.Get("X-Admin-Key") == expected {
		return true
	}
	identity, ok := s.platformIdentity(r)
	if !ok {
		return false
	}
	active, err := s.store.PlatformSessionActive(r.Context(), identity.Subject, identity.IssuedAt)
	return err == nil && active
}

func (s *Server) platformIdentity(r *http.Request) (auth.Identity, bool) {
	if s.localJWT == nil {
		return auth.Identity{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		if cookie, err := r.Cookie("licitalens_platform_session"); err == nil {
			token = cookie.Value
		}
	}
	if token == "" {
		return auth.Identity{}, false
	}
	identity, err := s.localJWT.Authenticate(r.Context(), token)
	if err != nil || !hasRole(identity, auth.PlatformRole) {
		return auth.Identity{}, false
	}
	return identity, true
}

func hasRole(identity auth.Identity, role string) bool {
	for _, item := range identity.Roles {
		if item == role {
			return true
		}
	}
	return false
}

func (s *Server) setPlatformSessionCookies(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || getenv("COOKIE_SECURE") == "true"
	base := http.Cookie{Path: "/", Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: int((24 * time.Hour).Seconds())}
	http.SetCookie(w, &http.Cookie{Name: "licitalens_platform_session", Value: token, HttpOnly: true, Path: base.Path, Secure: base.Secure, SameSite: base.SameSite, MaxAge: base.MaxAge})
	http.SetCookie(w, &http.Cookie{Name: "licitalens_platform_csrf", Value: newID(), HttpOnly: false, Path: base.Path, Secure: base.Secure, SameSite: base.SameSite, MaxAge: base.MaxAge})
}

func (s *Server) clearPlatformSessionCookies(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || getenv("COOKIE_SECURE") == "true"
	for _, name := range []string{"licitalens_platform_session", "licitalens_platform_csrf"} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, HttpOnly: name == "licitalens_platform_session"})
	}
}

func (s *Server) validPlatformCSRF(r *http.Request) bool {
	cookie, err := r.Cookie("licitalens_platform_csrf")
	return err == nil && cookie.Value != "" && r.Header.Get("X-CSRF-Token") == cookie.Value
}

func (s *Server) adminAuthLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if decode(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "solicitação inválida", nil)
		return
	}
	operator, hash, err := s.store.PlatformOperatorByEmail(r.Context(), input.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "e-mail ou senha inválidos", nil)
		return
	}
	if !store.CheckPassword(hash, input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "e-mail ou senha inválidos", nil)
		return
	}
	token, err := s.localJWT.IssuePlatform(operator.SubjectID, operator.Email, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login_failed", "não foi possível emitir sessão", nil)
		return
	}
	s.setPlatformSessionCookies(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":    token,
		"operator": operator,
	})
}

func (s *Server) adminAuthLogout(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.platformIdentity(r)
	if ok {
		if err := s.store.RevokePlatformSessions(r.Context(), identity.Subject, time.Now().UTC()); err != nil && err != store.ErrNotFound {
			writeError(w, http.StatusInternalServerError, "logout_failed", "não foi possível encerrar a sessão", nil)
			return
		}
	}
	s.clearPlatformSessionCookies(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminAuthMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.platformIdentity(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "admin_unauthorized", "sessão de operador inválida", nil)
		return
	}
	active, err := s.store.PlatformSessionActive(r.Context(), identity.Subject, identity.IssuedAt)
	if err != nil || !active {
		writeError(w, http.StatusUnauthorized, "session_revoked", "sessão revogada", nil)
		return
	}
	operator, err := s.store.PlatformOperatorBySubject(r.Context(), identity.Subject)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "admin_unauthorized", "operador não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operator": operator})
}

func (s *Server) adminUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.platformIdentity(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "admin_unauthorized", "sessão de operador inválida", nil)
		return
	}
	if strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
		if cookie, err := r.Cookie("licitalens_platform_session"); err == nil && cookie.Value != "" && !s.validPlatformCSRF(r) {
			writeError(w, http.StatusForbidden, "csrf_invalid", "token CSRF inválido", nil)
			return
		}
	}
	organizationID := strings.TrimSpace(r.PathValue("organizationID"))
	if organizationID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "organização inválida", nil)
		return
	}
	var input struct {
		Plan             string  `json:"plan"`
		Status           string  `json:"status"`
		CurrentPeriodEnd *string `json:"current_period_end"`
		Note             string  `json:"note"`
	}
	if decode(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "solicitação inválida", nil)
		return
	}
	subscription := billing.Subscription{
		Plan:   strings.ToLower(strings.TrimSpace(input.Plan)),
		Status: strings.ToLower(strings.TrimSpace(input.Status)),
	}
	if input.CurrentPeriodEnd != nil && strings.TrimSpace(*input.CurrentPeriodEnd) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*input.CurrentPeriodEnd))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "current_period_end inválido (use RFC3339)", nil)
			return
		}
		value := parsed.UTC()
		subscription.CurrentPeriodEnd = &value
	}
	if err := s.store.ApplyPlatformSubscription(r.Context(), organizationID, subscription, identity.Subject, input.Note); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "organization_not_found", "organização não encontrada", nil)
			return
		}
		writeError(w, http.StatusBadRequest, "subscription_update_failed", err.Error(), nil)
		return
	}
	updated, err := s.store.Subscription(r.Context(), organizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "subscription_update_failed", "assinatura atualizada, mas não foi possível recarregar", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": updated})
}

func (s *Server) adminOrganizationDetail(w http.ResponseWriter, r *http.Request) {
	organizationID := strings.TrimSpace(r.PathValue("organizationID"))
	if organizationID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "organização inválida", nil)
		return
	}
	items, err := s.store.AdminOrganizations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_unavailable", "não foi possível carregar organização", nil)
		return
	}
	var found *adminOrganizationDetail
	for _, item := range items {
		if item.ID == organizationID {
			found = &adminOrganizationDetail{Organization: item}
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "organization_not_found", "organização não encontrada", nil)
		return
	}
	subscription, err := s.store.Subscription(r.Context(), organizationID)
	if err == nil {
		found.Subscription = subscription
	}
	now := time.Now().UTC()
	limits, _ := billing.Plan(subscription.Plan)
	if limits.MonthlyAI == 0 {
		limits = billing.Entitlements{Profiles: 1, MonthlyAI: 30, DailyAlerts: 20}
	}
	aiUsed, _ := s.store.UsageAmount(r.Context(), organizationID, now.Format("2006-01")+"-01", "ai_analysis")
	alertUsed, _ := s.store.UsageAmount(r.Context(), organizationID, now.Format("2006-01-02"), "daily_alert")
	profiles, _ := s.store.Profiles(r.Context(), organizationID)
	found.Usage = map[string]any{
		"profiles":     map[string]int{"used": len(profiles), "limit": limits.Profiles},
		"ai_analyses":  map[string]int{"used": aiUsed, "limit": limits.MonthlyAI},
		"daily_alerts": map[string]int{"used": alertUsed, "limit": limits.DailyAlerts},
	}
	writeJSON(w, http.StatusOK, found)
}

type adminOrganizationDetail struct {
	Organization domain.AdminOrganization `json:"organization"`
	Subscription billing.Subscription     `json:"subscription,omitempty"`
	Usage        map[string]any             `json:"usage,omitempty"`
}
