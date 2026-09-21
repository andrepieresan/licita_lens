package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

type saasMemory struct {
	accounts  map[string]domain.Account
	password  map[string]string
	deals     map[string]domain.Deal
	followups map[string][]domain.DealFollowUp
	revokedAt map[string]time.Time
	tokens    map[string]accountToken
	verified  map[string]bool
}
type accountToken struct {
	email, purpose string
	expiresAt      time.Time
	consumed       bool
}

func (m *Memory) ensureSaaS() {
	if m.saas == nil {
		m.saas = &saasMemory{accounts: map[string]domain.Account{}, password: map[string]string{}, deals: map[string]domain.Deal{}, followups: map[string][]domain.DealFollowUp{}, revokedAt: map[string]time.Time{}, tokens: map[string]accountToken{}, verified: map[string]bool{}}
	}
}

func (m *Memory) CreateAccountToken(_ context.Context, email, purpose, tokenHash string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	if _, ok := m.saas.accounts[strings.ToLower(email)]; !ok {
		return ErrNotFound
	}
	for hash, token := range m.saas.tokens {
		if token.email == strings.ToLower(email) && token.purpose == purpose && !token.consumed {
			token.consumed = true
			m.saas.tokens[hash] = token
		}
	}
	m.saas.tokens[tokenHash] = accountToken{email: strings.ToLower(email), purpose: purpose, expiresAt: expiresAt}
	return nil
}
func (m *Memory) ConsumeAccountToken(_ context.Context, purpose, tokenHash string) (domain.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	token, ok := m.saas.tokens[tokenHash]
	if !ok || token.consumed || token.purpose != purpose || !token.expiresAt.After(time.Now()) {
		return domain.Account{}, ErrNotFound
	}
	token.consumed = true
	m.saas.tokens[tokenHash] = token
	return m.saas.accounts[token.email], nil
}
func (m *Memory) MarkEmailVerified(_ context.Context, subjectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	m.saas.verified[subjectID] = true
	return nil
}
func (m *Memory) EmailVerified(_ context.Context, subjectID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.saas == nil {
		return false, ErrNotFound
	}
	return m.saas.verified[subjectID], nil
}
func (m *Memory) UpdatePassword(_ context.Context, subjectID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for email, a := range m.saas.accounts {
		if a.SubjectID == subjectID {
			m.saas.password[email] = passwordHash
			m.saas.revokedAt[subjectID] = time.Now().UTC()
			return nil
		}
	}
	return ErrNotFound
}
func (m *Memory) DeleteAccount(_ context.Context, subjectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	for email, account := range m.saas.accounts {
		if account.SubjectID == subjectID {
			for organizationID, members := range m.commercial.memberships {
				if members[subjectID] != "owner" {
					continue
				}
				delete(m.commercial.memberships, organizationID)
				delete(m.commercial.organizations, organizationID)
				delete(m.commercial.subscriptions, organizationID)
				delete(m.commercial.notificationPrefs, organizationID)
				for profileID, profile := range m.profiles {
					if profile.OrganizationID == organizationID {
						delete(m.profiles, profileID)
					}
				}
				for dealID, deal := range m.saas.deals {
					if deal.OrganizationID == organizationID {
						delete(m.saas.deals, dealID)
						delete(m.saas.followups, dealID)
					}
				}
			}
			delete(m.saas.accounts, email)
			delete(m.saas.password, email)
			delete(m.saas.revokedAt, subjectID)
			delete(m.saas.verified, subjectID)
			return nil
		}
	}
	return ErrNotFound
}

func (m *Memory) SessionActive(_ context.Context, subjectID string, issuedAt time.Time) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.saas == nil {
		return false, nil
	}
	for _, account := range m.saas.accounts {
		if account.SubjectID == subjectID {
			revoked := m.saas.revokedAt[subjectID]
			return revoked.IsZero() || issuedAt.After(revoked), nil
		}
	}
	return false, nil
}

func (m *Memory) RevokeSessions(_ context.Context, subjectID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	for _, account := range m.saas.accounts {
		if account.SubjectID == subjectID {
			m.saas.revokedAt[subjectID] = at.UTC()
			return nil
		}
	}
	return ErrNotFound
}

func (m *Memory) RegisterSaaSAccount(_ context.Context, email, passwordHash, fullName, organizationName, plan string, legal domain.LegalAcceptance) (domain.Account, domain.Organization, billing.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	email = strings.ToLower(strings.TrimSpace(email))
	if _, exists := m.saas.accounts[email]; exists {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, errors.New("email already registered")
	}
	if _, ok := billing.Plan(plan); !ok {
		plan = "essential"
	}
	acceptedAt := legal.AcceptedAt.UTC()
	account := domain.Account{ID: uuid.NewString(), Email: email, FullName: fullName, SubjectID: uuid.NewString(), TermsVersion: legal.TermsVersion, PrivacyVersion: legal.PrivacyVersion, CreatedAt: time.Now().UTC()}
	if !acceptedAt.IsZero() {
		account.LegalAcceptedAt = &acceptedAt
	}
	m.saas.accounts[email] = account
	m.saas.password[email] = passwordHash
	org := domain.Organization{ID: uuid.NewString(), Name: organizationName, Status: "active", CreatedAt: time.Now().UTC()}
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	m.commercial.organizations[org.ID] = org
	m.commercial.memberships[org.ID] = map[string]string{account.SubjectID: "owner"}
	subscription := billing.Subscription{Plan: plan, Status: "trialing", UpdatedAt: time.Now().UTC()}
	m.commercial.subscriptions[org.ID] = subscription
	m.commercial.notificationPrefs[org.ID] = domain.DefaultNotificationPreferences()
	return account, org, subscription, nil
}

func (m *Memory) AccountBySubject(_ context.Context, subjectID string) (domain.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.saas == nil {
		return domain.Account{}, ErrNotFound
	}
	for _, account := range m.saas.accounts {
		if account.SubjectID == subjectID {
			return account, nil
		}
	}
	return domain.Account{}, ErrNotFound
}

func (m *Memory) AccountByEmail(_ context.Context, email string) (domain.Account, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensureSaaS()
	email = strings.ToLower(strings.TrimSpace(email))
	account, ok := m.saas.accounts[email]
	if !ok {
		return domain.Account{}, "", ErrNotFound
	}
	return account, m.saas.password[email], nil
}

func (m *Memory) Deal(_ context.Context, organizationID, dealID string) (domain.Deal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensureSaaS()
	deal, ok := m.saas.deals[dealID]
	if !ok || deal.OrganizationID != organizationID {
		return domain.Deal{}, ErrNotFound
	}
	return deal, nil
}

func (m *Memory) DealFollowUps(_ context.Context, organizationID, dealID string) ([]domain.DealFollowUp, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensureSaaS()
	deal, ok := m.saas.deals[dealID]
	if !ok || deal.OrganizationID != organizationID {
		return nil, ErrNotFound
	}
	items := m.saas.followups[dealID]
	out := make([]domain.DealFollowUp, len(items))
	copy(out, items)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (m *Memory) DealByOpportunity(_ context.Context, organizationID, opportunityID string) (domain.Deal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensureSaaS()
	opportunityID = strings.TrimSpace(opportunityID)
	if opportunityID == "" {
		return domain.Deal{}, ErrNotFound
	}
	for _, deal := range m.saas.deals {
		if deal.OrganizationID == organizationID && deal.OpportunityID == opportunityID {
			return deal, nil
		}
	}
	return domain.Deal{}, ErrNotFound
}

func (m *Memory) Deals(_ context.Context, organizationID string) ([]domain.Deal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensureSaaS()
	result := []domain.Deal{}
	for _, deal := range m.saas.deals {
		if deal.OrganizationID == organizationID {
			result = append(result, deal)
		}
	}
	return result, nil
}

func (m *Memory) CreateDeal(_ context.Context, deal domain.Deal) (domain.Deal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	if !deal.Stage.Valid() {
		deal.Stage = domain.StageProspecting
	}
	now := time.Now().UTC()
	deal.ID = uuid.NewString()
	deal.CreatedAt = now
	deal.UpdatedAt = now
	m.saas.deals[deal.ID] = deal
	return deal, nil
}

func (m *Memory) UpdateDeal(_ context.Context, deal domain.Deal) (domain.Deal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	existing, ok := m.saas.deals[deal.ID]
	if !ok || existing.OrganizationID != deal.OrganizationID {
		return domain.Deal{}, ErrNotFound
	}
	if strings.TrimSpace(deal.Title) != "" {
		existing.Title = deal.Title
	}
	if strings.TrimSpace(deal.BuyerName) != "" {
		existing.BuyerName = deal.BuyerName
	}
	if deal.OpportunityID != "" {
		existing.OpportunityID = deal.OpportunityID
	}
	if deal.EstimatedValueCents != 0 {
		existing.EstimatedValueCents = deal.EstimatedValueCents
	}
	if deal.NextFollowUpAt != nil {
		existing.NextFollowUpAt = deal.NextFollowUpAt
	}
	if deal.Stage.Valid() {
		existing.Stage = deal.Stage
	}
	existing.UpdatedAt = time.Now().UTC()
	if existing.Stage == domain.StageWon || existing.Stage == domain.StageLost {
		now := time.Now().UTC()
		existing.ClosedAt = &now
	}
	m.saas.deals[deal.ID] = existing
	return existing, nil
}

func (m *Memory) AddDealFollowUp(_ context.Context, organizationID, dealID, note string, scheduledAt *time.Time) (domain.DealFollowUp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSaaS()
	deal, ok := m.saas.deals[dealID]
	if !ok || deal.OrganizationID != organizationID {
		return domain.DealFollowUp{}, ErrNotFound
	}
	followUp := domain.DealFollowUp{ID: uuid.NewString(), DealID: dealID, Note: note, ScheduledAt: scheduledAt, CreatedAt: time.Now().UTC()}
	m.saas.followups[dealID] = append(m.saas.followups[dealID], followUp)
	deal.LastFollowUpNote = note
	deal.NextFollowUpAt = scheduledAt
	deal.UpdatedAt = time.Now().UTC()
	m.saas.deals[dealID] = deal
	return followUp, nil
}

func HashPassword(password string) (string, error) {
	raw, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(raw), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
