package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"licitalens.dev/backend/internal/auth"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/providers/ai"
	"licitalens.dev/backend/internal/store"
)

type Server struct {
	store    store.DataStore
	ai       ai.Provider
	log      *slog.Logger
	demo     bool
	auth     auth.Authenticator
	stripe   *billing.Stripe
	localJWT *auth.LocalJWT
	metrics  *metrics
}

type metrics struct {
	requests atomic.Uint64
	latency  atomic.Uint64 // nanos
}

func New(data store.DataStore, provider ai.Provider, authenticator auth.Authenticator, stripe *billing.Stripe, localJWT *auth.LocalJWT, logger *slog.Logger, demo bool) http.Handler {
	s := &Server{store: data, ai: provider, auth: authenticator, stripe: stripe, localJWT: localJWT, log: logger, demo: demo, metrics: &metrics{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("GET /metrics", s.metricsEndpoint)
	mux.HandleFunc("GET /v1/admin/overview", s.adminOverview)
	mux.HandleFunc("GET /v1/admin/organizations", s.adminOrganizations)
	mux.HandleFunc("GET /v1/admin/subscription-history", s.adminHistory)
	mux.HandleFunc("POST /v1/admin/notifications/run", s.adminRunNotifications)
	mux.HandleFunc("GET /v1/account/organizations", s.listOrganizations)
	mux.HandleFunc("POST /v1/account/bootstrap", s.bootstrapOrganization)
	mux.HandleFunc("GET /v1/billing/history", s.billingHistory)
	mux.HandleFunc("POST /v1/billing/checkout", s.billingCheckout)
	mux.HandleFunc("POST /v1/billing/portal", s.billingPortal)
	mux.HandleFunc("POST /v1/auth/signup", s.authSignup)
	mux.HandleFunc("POST /v1/auth/login", s.authLogin)
	mux.HandleFunc("GET /v1/me", s.authMe)
	mux.HandleFunc("GET /v1/account/notification-preferences", s.getNotificationPreferences)
	mux.HandleFunc("PATCH /v1/account/notification-preferences", s.patchNotificationPreferences)
	mux.HandleFunc("PUT /v1/account/push-token", s.putPushToken)
	mux.HandleFunc("DELETE /v1/account/push-token", s.deletePushToken)
	mux.HandleFunc("GET /v1/deals", s.listDeals)
	mux.HandleFunc("POST /v1/deals", s.createDeal)
	mux.HandleFunc("PATCH /v1/deals/{id}", s.updateDeal)
	mux.HandleFunc("GET /v1/deals/{id}/followups", s.listDealFollowUps)
	mux.HandleFunc("POST /v1/deals/{id}/followups", s.addDealFollowUp)
	mux.HandleFunc("GET /v1/profiles", s.listProfiles)
	mux.HandleFunc("POST /v1/profiles", s.createProfile)
	mux.HandleFunc("GET /v1/profiles/{id}", s.getProfile)
	mux.HandleFunc("PUT /v1/profiles/{id}", s.updateProfile)
	mux.HandleFunc("DELETE /v1/profiles/{id}", s.deleteProfile)
	mux.HandleFunc("GET /v1/opportunities", s.listOpportunities)
	mux.HandleFunc("GET /v1/opportunities/{id}", s.getOpportunity)
	mux.HandleFunc("POST /v1/opportunities/{id}/analysis", s.analyzeOpportunity)
	mux.HandleFunc("GET /v1/usage", s.usage)
	mux.HandleFunc("POST /v1/webhooks/stripe", s.stripeWebhook)
	return s.middleware(mux)
}

type contextKey string

const organizationKey contextKey = "organization"

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		defer func() {
			s.metrics.requests.Add(1)
			s.metrics.latency.Add(uint64(time.Since(started)))
		}()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newID()
		}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		s.setCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/health/") || r.URL.Path == "/metrics" || r.URL.Path == "/v1/webhooks/stripe" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/admin" {
			w.Header().Del("Content-Type")
			s.adminPanel(w, r)
			return
		}
		if publicAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/admin/") {
			if !s.adminAuthorized(r) {
				writeError(w, http.StatusUnauthorized, "admin_unauthorized", "chave administrativa inválida", nil)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		organizationID := r.Header.Get("X-Organization-ID")
		userID := ""
		if s.auth != nil {
			token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if token != "" {
				identity, err := s.auth.Authenticate(r.Context(), token)
				if err != nil {
					if !s.demo {
						writeError(w, http.StatusUnauthorized, "unauthorized", "token inválido", nil)
						return
					}
				} else {
					userID = identity.Subject
				}
			}
		}
		if s.demo {
			if organizationID == "" {
				organizationID = "00000000-0000-0000-0000-000000000001"
			}
			if userID == "" {
				userID = "demo-user"
			}
		}
		accountRoute := accountOnlyPath(r.URL.Path)
		if userID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "autenticação é obrigatória", nil)
			return
		}
		if organizationID == "" && !accountRoute {
			writeError(w, http.StatusUnauthorized, "unauthorized", "organização ativa é obrigatória", nil)
			return
		}
		ctx := withSubject(r.Context(), userID)
		if accountRoute {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if !s.demo {
			member, err := s.store.IsMember(ctx, organizationID, userID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "authorization_unavailable", "não foi possível validar a organização", nil)
				return
			}
			if !member {
				writeError(w, http.StatusForbidden, "organization_access_denied", "o usuário não pertence à organização ativa", nil)
				return
			}
		}
		if !s.demo && !subscriptionExempt(r.URL.Path) {
			subscription, err := s.store.Subscription(ctx, organizationID)
			if err != nil {
				writeError(w, http.StatusPaymentRequired, "subscription_required", "uma assinatura ativa é obrigatória", nil)
				return
			}
			if !subscriptionActive(subscription.Status) {
				writeError(w, http.StatusPaymentRequired, "subscription_required", "uma assinatura ativa é obrigatória", map[string]any{"status": subscription.Status})
				return
			}
			ctx = context.WithValue(ctx, subscriptionKey, subscription)
		}
		ctx = context.WithValue(ctx, organizationKey, organizationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	allowed := s.demo
	if !allowed {
		for _, candidate := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
			if strings.TrimSpace(candidate) == origin {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Organization-ID, X-Request-ID, X-Admin-Key")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
	}
	w.Header().Set("Vary", "Origin")
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{"api": "ok"}
	status := "ready"
	code := http.StatusOK
	if pinger, ok := s.store.(store.Pinger); ok {
		if err := pinger.Ping(r.Context()); err != nil {
			checks["postgres"] = "unavailable"
			status = "degraded"
			code = http.StatusServiceUnavailable
		} else {
			checks["postgres"] = "ok"
		}
	}
	writeJSON(w, code, map[string]any{"status": status, "checks": checks})
}

func (s *Server) metricsEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	requests := s.metrics.requests.Load()
	latency := s.metrics.latency.Load()
	_, _ = io.WriteString(w, "# HELP licitalens_http_requests_total Total de requisições HTTP.\n# TYPE licitalens_http_requests_total counter\n")
	_, _ = io.WriteString(w, "licitalens_http_requests_total "+strconv.FormatUint(requests, 10)+"\n")
	_, _ = io.WriteString(w, "# HELP licitalens_http_latency_seconds_total Soma das latências HTTP.\n# TYPE licitalens_http_latency_seconds_total counter\n")
	_, _ = io.WriteString(w, "licitalens_http_latency_seconds_total "+strconv.FormatFloat(float64(latency)/float64(time.Second), 'f', 6, 64)+"\n")
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": s.store.Profiles(org(r)), "next_cursor": nil})
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	var profile domain.CommercialProfile
	if err := decode(r, &profile); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	profile.Name, profile.Description = strings.TrimSpace(profile.Name), strings.TrimSpace(profile.Description)
	if profile.Name == "" || profile.Description == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "name e description são obrigatórios", map[string]any{"fields": []string{"name", "description"}})
		return
	}
	if len(s.store.Profiles(org(r))) >= entitlement(r).Profiles {
		writeError(w, http.StatusConflict, "profile_limit_reached", "o plano não permite mais perfis", nil)
		return
	}
	profile.ID, profile.OrganizationID, profile.CreatedAt = newID(), org(r), time.Now().UTC()
	if err := s.store.PutProfile(profile); err != nil {
		writeError(w, http.StatusInternalServerError, "persistence_error", "não foi possível salvar o perfil", nil)
		return
	}
	writeJSON(w, http.StatusCreated, profile)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := s.store.Profile(r.PathValue("id"), org(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "perfil não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	existing, err := s.store.Profile(r.PathValue("id"), org(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "perfil não encontrado", nil)
		return
	}
	var profile domain.CommercialProfile
	if err := decode(r, &profile); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	profile.Name, profile.Description = strings.TrimSpace(profile.Name), strings.TrimSpace(profile.Description)
	if profile.Name == "" || profile.Description == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "name e description são obrigatórios", map[string]any{"fields": []string{"name", "description"}})
		return
	}
	profile.ID, profile.OrganizationID, profile.CreatedAt = existing.ID, existing.OrganizationID, existing.CreatedAt
	if err := s.store.PutProfile(profile); err != nil {
		writeError(w, http.StatusInternalServerError, "persistence_error", "não foi possível atualizar o perfil", nil)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteProfile(r.PathValue("id"), org(r)); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "perfil não encontrado", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listOpportunities(w http.ResponseWriter, r *http.Request) {
	if !s.demo && r.URL.Query().Get("profile_id") != "" {
		limits := entitlement(r)
		allowed, _, err := s.store.ConsumeUsage(r.Context(), org(r), time.Now().UTC().Format("2006-01-02"), "daily_alert", limits.DailyAlerts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "usage_unavailable", "não foi possível consultar alertas", nil)
			return
		}
		if !allowed {
			writeError(w, http.StatusTooManyRequests, "alert_quota_exceeded", "o limite diário de alertas foi atingido", nil)
			return
		}
	}
	values := s.store.Opportunities()
	limit := 20
	if requested, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && requested > 0 && requested <= 100 {
		limit = requested
	}
	if len(values) > limit {
		values = values[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": values, "next_cursor": nil})
}

func (s *Server) getOpportunity(w http.ResponseWriter, r *http.Request) {
	opportunity, err := s.store.Opportunity(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "oportunidade não encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, opportunity)
}

func (s *Server) analyzeOpportunity(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProfileID string `json:"profile_id"`
	}
	if err := decode(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	profile, err := s.store.Profile(input.ProfileID, org(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "perfil não encontrado", nil)
		return
	}
	opportunity, err := s.store.Opportunity(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "oportunidade não encontrada", nil)
		return
	}
	similarity := lexicalSimilarity(profile.Description+" "+strings.Join(profile.Keywords, " "), opportunity.Object)
	match, ok := domain.Rank(domain.RankInput{Profile: profile, Opportunity: opportunity, SemanticSimilarity: similarity, CompetitionScore: .5, Now: time.Now().UTC()})
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"matched": false})
		return
	}
	if !s.demo {
		allowed, _, err := s.store.ConsumeUsage(r.Context(), org(r), time.Now().UTC().Format("2006-01")+"-01", "ai_analysis", entitlement(r).MonthlyAI)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "usage_unavailable", "não foi possível consultar o consumo", nil)
			return
		}
		if !allowed {
			writeError(w, http.StatusTooManyRequests, "ai_quota_exceeded", "o limite mensal de análises de IA foi atingido", nil)
			return
		}
	}
	if s.ai != nil {
		evidence, _ := json.Marshal(match)
		explanation, aiErr := s.ai.Explain(r.Context(), "Explique em português, de forma objetiva, por que a oportunidade combina com o perfil. Use somente as evidências recebidas. Esta é uma análise comercial informativa: não confirme habilitação, regularidade, classificação, julgamento, fraude ou chance de vitória; não interprete o edital como parecer jurídico e recomende conferir a fonte oficial.", string(evidence))
		if aiErr == nil {
			match.Explanation, match.PromptVersion = explanation.Text, "opportunity-explanation-v1"
		} else {
			s.log.Warn("AI explanation unavailable", "error", aiErr)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"matched": true, "match": match})
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	plan := entitlementPlan(r)
	limits := entitlement(r)
	now := time.Now().UTC()
	aiUsed, dailyUsed := 0, 0
	if !s.demo {
		var err error
		aiUsed, err = s.store.UsageAmount(r.Context(), org(r), now.Format("2006-01")+"-01", "ai_analysis")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "usage_unavailable", "não foi possível consultar o consumo", nil)
			return
		}
		dailyUsed, err = s.store.UsageAmount(r.Context(), org(r), now.Format("2006-01-02"), "daily_alert")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "usage_unavailable", "não foi possível consultar o consumo", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plan":         plan,
		"profiles":     map[string]int{"used": len(s.store.Profiles(org(r))), "limit": limits.Profiles},
		"ai_analyses":  map[string]int{"used": aiUsed, "limit": limits.MonthlyAI},
		"daily_alerts": map[string]int{"used": dailyUsed, "limit": limits.DailyAlerts},
	})
}

func (s *Server) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "payload inválido", nil)
		return
	}
	if err := billing.VerifyStripeSignature(payload, r.Header.Get("Stripe-Signature"), getenv("STRIPE_WEBHOOK_SECRET"), time.Now().UTC(), 5*time.Minute); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_signature", "assinatura Stripe inválida", nil)
		return
	}
	var event struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created int64  `json:"created"`
		Data    struct {
			Object struct {
				ID       string            `json:"id"`
				Status   string            `json:"status"`
				Metadata map[string]string `json:"metadata"`
				Items    struct {
					Data []struct {
						Price struct {
							LookupKey string            `json:"lookup_key"`
							Metadata  map[string]string `json:"metadata"`
						} `json:"price"`
					} `json:"data"`
				} `json:"items"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil || event.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid_event", "evento Stripe inválido", nil)
		return
	}
	if !strings.HasPrefix(event.Type, "customer.subscription.") {
		if event.Type == "checkout.session.completed" {
			s.handleCheckoutCompleted(w, r, payload)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "ignored": true})
		return
	}
	organizationID := event.Data.Object.Metadata["organization_id"]
	plan := event.Data.Object.Metadata["plan"]
	if plan == "" && len(event.Data.Object.Items.Data) > 0 {
		plan = event.Data.Object.Items.Data[0].Price.LookupKey
		if plan == "" {
			plan = event.Data.Object.Items.Data[0].Price.Metadata["plan"]
		}
	}
	created := time.Unix(event.Created, 0).UTC()
	if organizationID == "" || event.Data.Object.Status == "" || plan == "" || created.IsZero() {
		writeError(w, http.StatusBadRequest, "invalid_event", "evento não contém organização, plano ou status", nil)
		return
	}
	changed, err := s.store.ApplySubscription(r.Context(), organizationID, billing.Subscription{ExternalID: event.Data.Object.ID, Plan: plan, Status: event.Data.Object.Status, UpdatedAt: created}, event.ID, fmtHash(payload))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "webhook_processing_failed", "não foi possível processar o evento", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": true, "processed": changed})
}

func lexicalSimilarity(a, b string) float64 {
	left, right := words(a), words(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	for word := range left {
		if right[word] {
			intersection++
		}
	}
	return math.Min(1, float64(intersection)/math.Sqrt(float64(len(left)*len(right)))*2)
}
func words(value string) map[string]bool {
	result := map[string]bool{}
	for _, word := range strings.Fields(strings.ToLower(value)) {
		if len(word) > 2 {
			result[strings.Trim(word, ",.;:()[]")] = true
		}
	}
	return result
}

const subscriptionKey contextKey = "subscription"

func entitlement(r *http.Request) billing.Entitlements {
	value, ok := r.Context().Value(subscriptionKey).(billing.Subscription)
	if ok {
		if limits, valid := billing.Plan(value.Plan); valid {
			return limits
		}
	}
	return billing.Entitlements{Profiles: 1, MonthlyAI: 30, DailyAlerts: 20}
}
func entitlementPlan(r *http.Request) string {
	value, ok := r.Context().Value(subscriptionKey).(billing.Subscription)
	if ok && value.Plan != "" {
		return value.Plan
	}
	return "essential"
}
func fmtHash(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
func getenv(key string) string { return os.Getenv(key) }
func org(r *http.Request) string {
	value, _ := r.Context().Value(organizationKey).(string)
	return value
}
func newID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}
func decode(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("JSON inválido: " + err.Error())
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, map[string]any{"code": code, "message": message, "details": details})
}
