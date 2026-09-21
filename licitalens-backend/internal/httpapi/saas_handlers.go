package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/notifications"
	"licitalens.dev/backend/internal/store"
)

func publicAuthPath(path string) bool {
	switch path {
	case "/v1/auth/signup", "/v1/auth/login", "/v1/auth/verify-email", "/v1/auth/email-verification", "/v1/auth/password-recovery", "/v1/auth/password-reset":
		return true
	default:
		return false
	}
}

func (s *Server) authVerificationRequest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if decode(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "solicitação inválida", nil)
		return
	}
	token, hash, err := accountToken()
	if err == nil {
		err = s.store.CreateAccountToken(r.Context(), input.Email, "verify_email", hash, time.Now().Add(24*time.Hour))
	}
	if err == nil {
		if sendErr := sendAccountEmail(r.Context(), input.Email, "Confirme seu e-mail", "Confirme sua conta: "+accountURL("/verificar-email?token="+token)); sendErr != nil {
			s.log.Error("send verification email", "error", sendErr)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "se a conta existir, as instruções serão enviadas"})
}

func accountToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}
func (s *Server) authVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if decode(r, &in) != nil {
		writeError(w, 400, "invalid_request", "token inválido", nil)
		return
	}
	sum := sha256.Sum256([]byte(in.Token))
	a, err := s.store.ConsumeAccountToken(r.Context(), "verify_email", hex.EncodeToString(sum[:]))
	if err != nil {
		writeError(w, 400, "invalid_token", "token inválido ou expirado", nil)
		return
	}
	if err = s.store.MarkEmailVerified(r.Context(), a.SubjectID); err != nil {
		writeError(w, 500, "verification_failed", "não foi possível confirmar e-mail", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) authPasswordRecovery(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if decode(r, &in) != nil {
		writeError(w, 400, "invalid_request", "solicitação inválida", nil)
		return
	}
	token, hash, err := accountToken()
	if err == nil {
		err = s.store.CreateAccountToken(r.Context(), in.Email, "password_reset", hash, time.Now().Add(time.Hour))
	}
	if err == nil && s.demo {
		writeJSON(w, 202, map[string]string{"token": token})
		return
	}
	if err == nil {
		if sendErr := sendAccountEmail(r.Context(), in.Email, "Redefina sua senha", "Redefina sua senha: "+accountURL("/recuperar-senha?token="+token)); sendErr != nil {
			s.log.Error("send password recovery email", "error", sendErr)
		}
	}
	writeJSON(w, 202, map[string]string{"message": "se a conta existir, as instruções serão enviadas"})
}

func accountURL(path string) string {
	base := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	return base + path
}
func sendAccountEmail(parent context.Context, destination, subject, body string) error {
	timeout, err := time.ParseDuration(envOr("SMTP_TIMEOUT", "15s"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return notifications.SendSMTPText(ctx, notifications.SMTPConfig{
		Host: os.Getenv("SMTP_HOST"), Port: envOr("SMTP_PORT", "587"), From: os.Getenv("SMTP_FROM"),
		User: os.Getenv("SMTP_USER"), Password: os.Getenv("SMTP_PASSWORD"), TLSMode: envOr("SMTP_TLS_MODE", "auto"), Timeout: timeout,
	}, destination, subject, body)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func (s *Server) authPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if decode(r, &in) != nil || len(in.Password) < 8 {
		writeError(w, 400, "invalid_request", "dados inválidos", nil)
		return
	}
	sum := sha256.Sum256([]byte(in.Token))
	a, err := s.store.ConsumeAccountToken(r.Context(), "password_reset", hex.EncodeToString(sum[:]))
	if err != nil {
		writeError(w, 400, "invalid_token", "token inválido ou expirado", nil)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil || s.store.UpdatePassword(r.Context(), a.SubjectID, string(hash)) != nil {
		writeError(w, 500, "password_reset_failed", "não foi possível atualizar a senha", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func accountOnlyPath(path string) bool {
	return path == "/v1/account/organizations" || path == "/v1/account/bootstrap" || path == "/v1/account/export" || path == "/v1/account" || path == "/v1/me" || path == "/v1/auth/logout"
}

func (s *Server) accountExport(w http.ResponseWriter, r *http.Request) {
	account, err := s.store.AccountBySubject(r.Context(), subject(r))
	if err != nil {
		writeError(w, 500, "export_failed", "não foi possível exportar", nil)
		return
	}
	orgs, err := s.store.OrganizationsForSubject(r.Context(), subject(r))
	if err != nil {
		writeError(w, 500, "export_failed", "não foi possível exportar", nil)
		return
	}
	organizations := make([]map[string]any, 0, len(orgs))
	for _, organization := range orgs {
		profiles, profileErr := s.store.Profiles(r.Context(), organization.ID)
		deals, dealErr := s.store.Deals(r.Context(), organization.ID)
		preferences, prefsErr := s.store.NotificationPreferences(r.Context(), organization.ID)
		history, historyErr := s.store.SubscriptionHistory(r.Context(), organization.ID, 100)
		if profileErr != nil || dealErr != nil || prefsErr != nil || historyErr != nil {
			writeError(w, 500, "export_failed", "não foi possível exportar todos os dados", nil)
			return
		}
		dealExport := make([]map[string]any, 0, len(deals))
		for _, deal := range deals {
			followups, followupErr := s.store.DealFollowUps(r.Context(), organization.ID, deal.ID)
			if followupErr != nil {
				writeError(w, 500, "export_failed", "não foi possível exportar todos os dados", nil)
				return
			}
			dealExport = append(dealExport, map[string]any{"deal": deal, "followups": followups})
		}
		organizations = append(organizations, map[string]any{"organization": organization, "profiles": profiles, "deals": dealExport, "notification_preferences": preferences, "subscription_history": history})
	}
	writeJSON(w, 200, map[string]any{"exported_at": time.Now().UTC(), "account": account, "organizations": organizations})
}
func (s *Server) accountDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirmation string `json:"confirmation"`
	}
	if decode(r, &in) != nil || in.Confirmation != "DELETE" {
		writeError(w, 400, "confirmation_required", "envie confirmation=DELETE", nil)
		return
	}
	if err := s.store.DeleteAccount(r.Context(), subject(r)); err != nil {
		writeError(w, 500, "delete_failed", "não foi possível excluir a conta", nil)
		return
	}
	s.clearSessionCookies(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authSignup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		FullName         string `json:"full_name"`
		OrganizationName string `json:"organization_name"`
		Plan             string `json:"plan"`
		LegalAccepted    bool   `json:"legal_accepted"`
		TermsVersion     string `json:"terms_version"`
		PrivacyVersion   string `json:"privacy_version"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if len(strings.TrimSpace(input.Password)) < 8 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "senha deve ter ao menos 8 caracteres", nil)
		return
	}
	if !s.demo && (!input.LegalAccepted || strings.TrimSpace(input.TermsVersion) == "" || strings.TrimSpace(input.PrivacyVersion) == "") {
		writeError(w, http.StatusUnprocessableEntity, "legal_acceptance_required", "aceite dos termos e da política de privacidade é obrigatório", nil)
		return
	}
	if s.demo && !input.LegalAccepted {
		input.TermsVersion = "demo"
		input.PrivacyVersion = "demo"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signup_failed", "não foi possível criar a conta", nil)
		return
	}
	account, organization, subscription, err := s.store.RegisterSaaSAccount(r.Context(), input.Email, string(hash), input.FullName, input.OrganizationName, strings.ToLower(input.Plan), domain.LegalAcceptance{TermsVersion: strings.TrimSpace(input.TermsVersion), PrivacyVersion: strings.TrimSpace(input.PrivacyVersion), AcceptedAt: time.Now().UTC()})
	if err != nil {
		writeError(w, http.StatusConflict, "signup_failed", err.Error(), nil)
		return
	}
	emailSent := false
	if verifyToken, verifyHash, tokenErr := accountToken(); tokenErr == nil {
		if tokenErr = s.store.CreateAccountToken(r.Context(), account.Email, "verify_email", verifyHash, time.Now().Add(24*time.Hour)); tokenErr == nil {
			if tokenErr = sendAccountEmail(r.Context(), account.Email, "Confirme seu e-mail", "Confirme sua conta: "+accountURL("/verificar-email?token="+verifyToken)); tokenErr == nil {
				emailSent = true
			}
		}
		if tokenErr != nil {
			s.log.Error("prepare signup verification", "error", tokenErr)
		}
	}
	if s.cloud {
		writeJSON(w, http.StatusAccepted, map[string]any{"verification_required": true, "email": account.Email, "email_sent": emailSent})
		return
	}
	token, err := s.localJWT.Issue(account.SubjectID, account.Email, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signup_failed", "não foi possível emitir sessão", nil)
		return
	}
	s.setSessionCookies(w, r, token)
	writeJSON(w, http.StatusCreated, s.sessionPayload(token, account, organization, subscription))
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	account, hash, err := s.store.AccountByEmail(r.Context(), input.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "e-mail ou senha inválidos", nil)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "e-mail ou senha inválidos", nil)
		return
	}
	if s.cloud {
		verified, verifyErr := s.store.EmailVerified(r.Context(), account.SubjectID)
		if verifyErr != nil || !verified {
			writeError(w, http.StatusForbidden, "email_not_verified", "confirme seu e-mail antes de entrar", nil)
			return
		}
	}
	organizations, err := s.store.OrganizationsForSubject(r.Context(), account.SubjectID)
	if err != nil || len(organizations) == 0 {
		writeError(w, http.StatusForbidden, "organization_missing", "nenhuma organização vinculada à conta", nil)
		return
	}
	subscription, err := s.store.Subscription(r.Context(), organizations[0].ID)
	if err != nil {
		writeError(w, http.StatusPaymentRequired, "subscription_required", "assinatura não encontrada", nil)
		return
	}
	token, err := s.localJWT.Issue(account.SubjectID, account.Email, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login_failed", "não foi possível emitir sessão", nil)
		return
	}
	s.setSessionCookies(w, r, token)
	writeJSON(w, http.StatusOK, s.sessionPayload(token, account, organizations[0], subscription))
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeSessions(r.Context(), subject(r), time.Now().UTC()); err != nil && err != store.ErrNotFound {
		writeError(w, http.StatusInternalServerError, "logout_failed", "não foi possível encerrar a sessão", nil)
		return
	}
	s.clearSessionCookies(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	organizations, err := s.store.OrganizationsForSubject(r.Context(), subject(r))
	if err != nil || len(organizations) == 0 {
		writeError(w, http.StatusNotFound, "organization_not_found", "organização não encontrada", nil)
		return
	}
	orgID := org(r)
	active := organizations[0]
	for _, item := range organizations {
		if item.ID == orgID {
			active = item
			break
		}
	}
	subscription, err := s.store.Subscription(r.Context(), active.ID)
	if err != nil {
		writeError(w, http.StatusPaymentRequired, "subscription_required", "assinatura não encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"organization": active,
		"subscription": map[string]any{"plan": subscription.Plan, "status": subscription.Status, "active": s.subscriptionActive(subscription)},
	})
}

func (s *Server) sessionPayload(token string, account domain.Account, organization domain.Organization, subscription any) map[string]any {
	return map[string]any{
		"access_token": token,
		"account":      account,
		"organization": organization,
		"subscription": subscription,
	}
}

func (s *Server) listDeals(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Deals(r.Context(), org(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "deals_unavailable", "não foi possível listar oportunidades comerciais", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) createDeal(w http.ResponseWriter, r *http.Request) {
	var deal domain.Deal
	if err := decode(r, &deal); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	deal.OrganizationID = org(r)
	if strings.TrimSpace(deal.Title) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "title é obrigatório", nil)
		return
	}
	if opportunityID := strings.TrimSpace(deal.OpportunityID); opportunityID != "" {
		if existing, err := s.store.DealByOpportunity(r.Context(), deal.OrganizationID, opportunityID); err == nil {
			writeJSON(w, http.StatusOK, existing)
			return
		} else if err != store.ErrNotFound {
			writeError(w, http.StatusInternalServerError, "deal_lookup_failed", "não foi possível verificar o pipeline", nil)
			return
		}
	}
	created, err := s.store.CreateDeal(r.Context(), deal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "deal_create_failed", "não foi possível criar o card", nil)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateDeal(w http.ResponseWriter, r *http.Request) {
	existing, err := s.store.Deal(r.Context(), org(r), r.PathValue("id"))
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "card não encontrado", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "deal_unavailable", "não foi possível carregar o card", nil)
		return
	}
	var patch domain.Deal
	if err := decode(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	deal := mergeDealPatch(existing, patch)
	deal.ID = existing.ID
	deal.OrganizationID = org(r)
	updated, err := s.store.UpdateDeal(r.Context(), deal)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "card não encontrado", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "deal_update_failed", "não foi possível atualizar o card", nil)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func mergeDealPatch(existing, patch domain.Deal) domain.Deal {
	if strings.TrimSpace(patch.Title) != "" {
		existing.Title = patch.Title
	}
	if strings.TrimSpace(patch.BuyerName) != "" {
		existing.BuyerName = patch.BuyerName
	}
	if patch.OpportunityID != "" {
		existing.OpportunityID = patch.OpportunityID
	}
	if patch.EstimatedValueCents != 0 {
		existing.EstimatedValueCents = patch.EstimatedValueCents
	}
	if patch.NextFollowUpAt != nil {
		existing.NextFollowUpAt = patch.NextFollowUpAt
	}
	if patch.Stage.Valid() {
		existing.Stage = patch.Stage
	}
	return existing
}

func (s *Server) getNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	prefs, err := s.store.NotificationPreferences(r.Context(), org(r))
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "organização não encontrada", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "preferences_unavailable", "não foi possível carregar preferências", nil)
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

func (s *Server) patchNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	var prefs domain.NotificationPreferences
	if err := decode(r, &prefs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	updated, err := s.store.PutNotificationPreferences(r.Context(), org(r), prefs)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "organização não encontrada", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "preferences_update_failed", "não foi possível salvar preferências", nil)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) putPushToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "token é obrigatório", nil)
		return
	}
	if !strings.HasPrefix(token, "ExponentPushToken[") && !strings.HasPrefix(token, "ExpoPushToken[") {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "token Expo inválido", nil)
		return
	}
	if err := s.store.RegisterPushToken(r.Context(), org(r), token); err != nil {
		writeError(w, http.StatusInternalServerError, "push_token_failed", "não foi possível registrar push", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (s *Server) deletePushToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "token é obrigatório", nil)
		return
	}
	if err := s.store.RemovePushToken(r.Context(), org(r), token); err != nil {
		writeError(w, http.StatusInternalServerError, "push_token_failed", "não foi possível remover push", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listDealFollowUps(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.DealFollowUps(r.Context(), org(r), r.PathValue("id"))
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "card não encontrado", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "followups_unavailable", "não foi possível listar follow-ups", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) addDealFollowUp(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Note        string     `json:"note"`
		ScheduledAt *time.Time `json:"scheduled_at"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if strings.TrimSpace(input.Note) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "note é obrigatória", nil)
		return
	}
	followUp, err := s.store.AddDealFollowUp(r.Context(), org(r), r.PathValue("id"), input.Note, input.ScheduledAt)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "card não encontrado", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "followup_failed", "não foi possível registrar follow-up", nil)
		return
	}
	writeJSON(w, http.StatusCreated, followUp)
}
