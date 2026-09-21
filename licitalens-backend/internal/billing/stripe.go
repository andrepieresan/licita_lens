package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Stripe struct {
	SecretKey      string
	HTTP           *http.Client
	APIBaseURL     string
	SuccessURL     string
	CancelURL      string
	PortalURL      string
	PriceEssential string
	PricePro       string
}

func NewStripeFromEnv() *Stripe {
	return &Stripe{
		SecretKey:      strings.TrimSpace(getenv("STRIPE_SECRET_KEY")),
		SuccessURL:     strings.TrimSpace(getenv("STRIPE_SUCCESS_URL")),
		CancelURL:      strings.TrimSpace(getenv("STRIPE_CANCEL_URL")),
		PortalURL:      strings.TrimSpace(getenv("STRIPE_PORTAL_RETURN_URL")),
		PriceEssential: strings.TrimSpace(getenv("STRIPE_PRICE_ESSENTIAL")),
		PricePro:       strings.TrimSpace(getenv("STRIPE_PRICE_PRO")),
		APIBaseURL:     "https://api.stripe.com",
		HTTP:           &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Stripe) Enabled() bool {
	return s != nil && s.SecretKey != ""
}

func (s *Stripe) priceID(plan string) (string, error) {
	switch strings.ToLower(plan) {
	case "essential":
		if s.PriceEssential == "" {
			return "", errors.New("STRIPE_PRICE_ESSENTIAL não configurado")
		}
		return s.PriceEssential, nil
	case "pro":
		if s.PricePro == "" {
			return "", errors.New("STRIPE_PRICE_PRO não configurado")
		}
		return s.PricePro, nil
	default:
		return "", errors.New("plano inválido")
	}
}

type CheckoutInput struct {
	OrganizationID string
	CustomerID     string
	CustomerEmail  string
	Plan           string
}

func (s *Stripe) CreateCheckoutSession(input CheckoutInput) (string, error) {
	if !s.Enabled() {
		return "", errors.New("stripe não configurado")
	}
	priceID, err := s.priceID(input.Plan)
	if err != nil {
		return "", err
	}
	if s.SuccessURL == "" || s.CancelURL == "" {
		return "", errors.New("STRIPE_SUCCESS_URL e STRIPE_CANCEL_URL são obrigatórios")
	}
	values := url.Values{}
	values.Set("mode", "subscription")
	values.Set("success_url", s.SuccessURL)
	values.Set("cancel_url", s.CancelURL)
	values.Set("client_reference_id", input.OrganizationID)
	values.Set("line_items[0][price]", priceID)
	values.Set("line_items[0][quantity]", "1")
	values.Set("subscription_data[metadata][organization_id]", input.OrganizationID)
	values.Set("subscription_data[metadata][plan]", input.Plan)
	values.Set("metadata[organization_id]", input.OrganizationID)
	values.Set("metadata[plan]", input.Plan)
	if input.CustomerID != "" {
		values.Set("customer", input.CustomerID)
	}
	if input.CustomerEmail != "" {
		values.Set("customer_email", input.CustomerEmail)
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := s.postForm(s.apiURL("/v1/checkout/sessions"), values, &payload); err != nil {
		return "", err
	}
	if payload.URL == "" {
		return "", errors.New("stripe não retornou URL de checkout")
	}
	return payload.URL, nil
}

func (s *Stripe) CreatePortalSession(customerID string) (string, error) {
	if !s.Enabled() {
		return "", errors.New("stripe não configurado")
	}
	if customerID == "" {
		return "", errors.New("cliente Stripe não vinculado à organização")
	}
	returnURL := s.PortalURL
	if returnURL == "" {
		returnURL = s.SuccessURL
	}
	if returnURL == "" {
		return "", errors.New("STRIPE_PORTAL_RETURN_URL ou STRIPE_SUCCESS_URL é obrigatório")
	}
	values := url.Values{}
	values.Set("customer", customerID)
	values.Set("return_url", returnURL)
	var payload struct {
		URL string `json:"url"`
	}
	if err := s.postForm(s.apiURL("/v1/billing_portal/sessions"), values, &payload); err != nil {
		return "", err
	}
	if payload.URL == "" {
		return "", errors.New("stripe não retornou URL do portal")
	}
	return payload.URL, nil
}

func (s *Stripe) apiURL(path string) string {
	base := strings.TrimRight(s.APIBaseURL, "/")
	if base == "" {
		base = "https://api.stripe.com"
	}
	return base + path
}

type ReconciledSubscription struct {
	OrganizationID string
	Subscription   Subscription
	EventID        string
}

// ListSubscriptions obtains the current Stripe state for every subscription
// carrying LicitaLens organization metadata. It is intended for an operator or
// scheduled reconciler to repair missed webhooks.
func (s *Stripe) ListSubscriptions(ctx context.Context) ([]ReconciledSubscription, error) {
	if !s.Enabled() {
		return nil, errors.New("stripe não configurado")
	}
	result := []ReconciledSubscription{}
	startingAfter := ""
	for {
		query := url.Values{"status": {"all"}, "limit": {"100"}, "expand[]": {"data.items.data.price"}}
		if startingAfter != "" {
			query.Set("starting_after", startingAfter)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL("/v1/subscriptions")+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+s.SecretKey)
		resp, err := s.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("stripe retornou %d ao reconciliar assinaturas", resp.StatusCode)
		}
		var page struct {
			Data []struct {
				ID               string            `json:"id"`
				Status           string            `json:"status"`
				CurrentPeriodEnd int64             `json:"current_period_end"`
				Metadata         map[string]string `json:"metadata"`
				Items            struct {
					Data []struct {
						Price struct {
							LookupKey string            `json:"lookup_key"`
							Metadata  map[string]string `json:"metadata"`
						} `json:"price"`
					} `json:"data"`
				} `json:"items"`
			} `json:"data"`
			HasMore bool `json:"has_more"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode stripe subscriptions: %w", err)
		}
		for _, item := range page.Data {
			organizationID := item.Metadata["organization_id"]
			plan := item.Metadata["plan"]
			if plan == "" && len(item.Items.Data) > 0 {
				plan = item.Items.Data[0].Price.LookupKey
				if plan == "" {
					plan = item.Items.Data[0].Price.Metadata["plan"]
				}
			}
			if organizationID == "" || item.ID == "" || item.Status == "" || plan == "" {
				continue
			}
			var periodEnd *time.Time
			if item.CurrentPeriodEnd > 0 {
				value := time.Unix(item.CurrentPeriodEnd, 0).UTC()
				periodEnd = &value
			}
			eventID := fmt.Sprintf("stripe-reconcile:%s:%s:%s:%d", item.ID, item.Status, plan, item.CurrentPeriodEnd)
			result = append(result, ReconciledSubscription{OrganizationID: organizationID, Subscription: Subscription{ExternalID: item.ID, Plan: plan, Status: item.Status, CurrentPeriodEnd: periodEnd, UpdatedAt: time.Now().UTC()}, EventID: eventID})
		}
		if !page.HasMore || len(page.Data) == 0 {
			return result, nil
		}
		startingAfter = page.Data[len(page.Data)-1].ID
	}
}

func (s *Stripe) postForm(endpoint string, values url.Values, target any) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var stripeErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &stripeErr)
		if stripeErr.Error.Message != "" {
			return fmt.Errorf("stripe: %s", stripeErr.Error.Message)
		}
		return fmt.Errorf("stripe retornou %d", resp.StatusCode)
	}
	return json.Unmarshal(body, target)
}

func getenv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
