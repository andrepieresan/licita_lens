package store

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

type commercialMemory struct {
	organizations     map[string]domain.Organization
	memberships       map[string]map[string]string
	subscriptions     map[string]billing.Subscription
	history           []domain.SubscriptionHistoryEntry
	customers         map[string]string
	usage             map[string]map[string]map[string]int
	notificationPrefs map[string]domain.NotificationPreferences
}

func newCommercialMemory() *commercialMemory {
	return &commercialMemory{
		organizations:     map[string]domain.Organization{},
		memberships:       map[string]map[string]string{},
		subscriptions:     map[string]billing.Subscription{},
		history:           []domain.SubscriptionHistoryEntry{},
		customers:         map[string]string{},
		usage:             map[string]map[string]map[string]int{},
		notificationPrefs: map[string]domain.NotificationPreferences{},
	}
}

func (m *Memory) UsageAmount(_ context.Context, organizationID, period, kind string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return 0, nil
	}
	return m.commercial.usage[organizationID][period][kind], nil
}

func (m *Memory) OrganizationsForSubject(_ context.Context, subjectID string) ([]domain.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return nil, nil
	}
	result := []domain.Organization{}
	for orgID, members := range m.commercial.memberships {
		if _, ok := members[subjectID]; ok {
			result = append(result, m.commercial.organizations[orgID])
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) BootstrapOrganization(_ context.Context, subjectID, name string) (domain.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	org := domain.Organization{ID: uuid.NewString(), Name: name, Status: "active", CreatedAt: time.Now().UTC()}
	m.commercial.organizations[org.ID] = org
	m.commercial.memberships[org.ID] = map[string]string{subjectID: "owner"}
	m.commercial.subscriptions[org.ID] = billing.Subscription{Plan: "essential", Status: "trialing", UpdatedAt: time.Now().UTC()}
	m.commercial.notificationPrefs[org.ID] = domain.DefaultNotificationPreferences()
	return org, nil
}

func (m *Memory) OrganizationStripeCustomer(_ context.Context, organizationID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return "", ErrNotFound
	}
	value, ok := m.commercial.customers[organizationID]
	if !ok || value == "" {
		return "", nil
	}
	return value, nil
}

func (m *Memory) SetOrganizationStripeCustomer(_ context.Context, organizationID, customerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	if _, ok := m.commercial.organizations[organizationID]; !ok {
		return ErrNotFound
	}
	m.commercial.customers[organizationID] = customerID
	return nil
}

func (m *Memory) SubscriptionHistory(_ context.Context, organizationID string, limit int) ([]domain.SubscriptionHistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	result := []domain.SubscriptionHistoryEntry{}
	for _, item := range m.commercial.history {
		if item.OrganizationID == organizationID {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *Memory) AdminOverview(_ context.Context) (domain.AdminOverview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return domain.AdminOverview{}, nil
	}
	overview := domain.AdminOverview{Organizations: len(m.commercial.organizations), HistoryEvents: len(m.commercial.history)}
	for _, subscription := range m.commercial.subscriptions {
		if subscription.Status == "active" {
			overview.ActiveSubscriptions++
			switch subscription.Plan {
			case "essential":
				overview.EssentialPlans++
			case "pro":
				overview.ProPlans++
			}
		}
	}
	return overview, nil
}

func (m *Memory) AdminOrganizations(_ context.Context) ([]domain.AdminOrganization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return nil, nil
	}
	result := []domain.AdminOrganization{}
	for _, org := range m.commercial.organizations {
		item := domain.AdminOrganization{ID: org.ID, Name: org.Name, Status: org.Status, CreatedAt: org.CreatedAt, MemberCount: len(m.commercial.memberships[org.ID])}
		if m.saas != nil {
			for _, deal := range m.saas.deals {
				if deal.OrganizationID == org.ID {
					item.PipelineDeals++
				}
			}
		}
		if subscription, ok := m.commercial.subscriptions[org.ID]; ok {
			item.Plan = subscription.Plan
			item.SubscriptionStatus = subscription.Status
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) AdminSubscriptionHistory(_ context.Context, limit int) ([]domain.SubscriptionHistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	result := append([]domain.SubscriptionHistoryEntry{}, m.commercial.history...)
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *Memory) recordHistoryLocked(organizationID string, subscription billing.Subscription, eventID string) {
	entry := domain.SubscriptionHistoryEntry{
		ID:                   uuid.NewString(),
		OrganizationID:       organizationID,
		StripeSubscriptionID: subscription.ExternalID,
		Plan:                 subscription.Plan,
		Status:               subscription.Status,
		StripeEventID:        eventID,
		RecordedAt:           subscription.UpdatedAt,
	}
	if org, ok := m.commercial.organizations[organizationID]; ok {
		entry.OrganizationName = org.Name
	}
	m.commercial.history = append(m.commercial.history, entry)
}

func (m *Memory) bumpUsageLocked(organizationID, period, kind string) int {
	if m.commercial.usage[organizationID] == nil {
		m.commercial.usage[organizationID] = map[string]map[string]int{}
	}
	if m.commercial.usage[organizationID][period] == nil {
		m.commercial.usage[organizationID][period] = map[string]int{}
	}
	m.commercial.usage[organizationID][period][kind]++
	return m.commercial.usage[organizationID][period][kind]
}

func (m *Memory) NotificationPreferences(_ context.Context, organizationID string) (domain.NotificationPreferences, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return domain.DefaultNotificationPreferences(), nil
	}
	if prefs, ok := m.commercial.notificationPrefs[organizationID]; ok {
		return prefs, nil
	}
	return domain.DefaultNotificationPreferences(), nil
}

func (m *Memory) PutNotificationPreferences(_ context.Context, organizationID string, prefs domain.NotificationPreferences) (domain.NotificationPreferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	m.commercial.notificationPrefs[organizationID] = prefs
	return prefs, nil
}
