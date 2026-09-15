package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/notifications"
)

func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.OrganizationsForSubject(r.Context(), subject(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_unavailable", "não foi possível listar organizações", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) bootstrapOrganization(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OrganizationName string `json:"organization_name"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	name := strings.TrimSpace(input.OrganizationName)
	if name == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "organization_name é obrigatório", nil)
		return
	}
	org, err := s.store.BootstrapOrganization(r.Context(), subject(r), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bootstrap_failed", "não foi possível criar a organização", nil)
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (s *Server) billingHistory(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.SubscriptionHistory(r.Context(), org(r), 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "history_unavailable", "não foi possível carregar o histórico", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request) {
	if s.stripe == nil || !s.stripe.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "billing_unconfigured", "Stripe não está configurado neste ambiente", nil)
		return
	}
	var input struct {
		Plan string `json:"plan"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	customerID, _ := s.store.OrganizationStripeCustomer(r.Context(), org(r))
	url, err := s.stripe.CreateCheckoutSession(billing.CheckoutInput{
		OrganizationID: org(r),
		CustomerID:     customerID,
		Plan:           strings.ToLower(strings.TrimSpace(input.Plan)),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "checkout_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) billingPortal(w http.ResponseWriter, r *http.Request) {
	if s.stripe == nil || !s.stripe.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "billing_unconfigured", "Stripe não está configurado neste ambiente", nil)
		return
	}
	customerID, err := s.store.OrganizationStripeCustomer(r.Context(), org(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "organization_not_found", "organização não encontrada", nil)
		return
	}
	url, err := s.stripe.CreatePortalSession(customerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "portal_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.store.AdminOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_unavailable", "não foi possível carregar o painel", nil)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) adminOrganizations(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.AdminOrganizations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_unavailable", "não foi possível listar organizações", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) adminHistory(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.AdminSubscriptionHistory(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_unavailable", "não foi possível carregar histórico", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) adminRunNotifications(w http.ResponseWriter, r *http.Request) {
	worker := notifications.NewWorker(s.store, s.log, notifications.WorkerConfig{
		DemoMode: s.demo,
		SMTP: notifications.SMTPConfig{
			Host: os.Getenv("SMTP_HOST"),
			Port: os.Getenv("SMTP_PORT"),
			From: os.Getenv("SMTP_FROM"),
		},
	})
	result, err := worker.Run(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notifications_failed", "não foi possível executar alertas", nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) adminPanel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(adminPanelHTML)
}

func (s *Server) adminAuthorized(r *http.Request) bool {
	expected := strings.TrimSpace(getenv("ADMIN_API_KEY"))
	if expected == "" {
		return s.demo
	}
	return r.Header.Get("X-Admin-Key") == expected
}

func (s *Server) handleCheckoutCompleted(w http.ResponseWriter, r *http.Request, payload []byte) {
	var event struct {
		Data struct {
			Object struct {
				Customer          string            `json:"customer"`
				ClientReferenceID string            `json:"client_reference_id"`
				Metadata          map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &event) != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "evento Stripe inválido", nil)
		return
	}
	organizationID := event.Data.Object.ClientReferenceID
	if organizationID == "" {
		organizationID = event.Data.Object.Metadata["organization_id"]
	}
	if organizationID == "" || event.Data.Object.Customer == "" {
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "ignored": true})
		return
	}
	if err := s.store.SetOrganizationStripeCustomer(r.Context(), organizationID, event.Data.Object.Customer); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook_processing_failed", "não foi possível vincular cliente Stripe", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": true, "processed": true})
}
