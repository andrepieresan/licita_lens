package httpapi

import (
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

func publicAuthPath(path string) bool {
	switch path {
	case "/v1/auth/signup", "/v1/auth/login":
		return true
	default:
		return false
	}
}

func accountOnlyPath(path string) bool {
	return path == "/v1/account/organizations" || path == "/v1/account/bootstrap" || path == "/v1/me"
}

func (s *Server) authSignup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		FullName         string `json:"full_name"`
		OrganizationName string `json:"organization_name"`
		Plan             string `json:"plan"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if len(strings.TrimSpace(input.Password)) < 8 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "senha deve ter ao menos 8 caracteres", nil)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signup_failed", "não foi possível criar a conta", nil)
		return
	}
	account, organization, subscription, err := s.store.RegisterSaaSAccount(r.Context(), input.Email, string(hash), input.FullName, input.OrganizationName, strings.ToLower(input.Plan))
	if err != nil {
		writeError(w, http.StatusConflict, "signup_failed", err.Error(), nil)
		return
	}
	token, err := s.localJWT.Issue(account.SubjectID, account.Email, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signup_failed", "não foi possível emitir sessão", nil)
		return
	}
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
	writeJSON(w, http.StatusOK, s.sessionPayload(token, account, organizations[0], subscription))
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
		"subscription": map[string]any{"plan": subscription.Plan, "status": subscription.Status, "active": subscriptionActive(subscription.Status)},
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
