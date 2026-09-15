package store

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Memory struct {
	mu            sync.RWMutex
	profiles      map[string]domain.CommercialProfile
	opportunities map[string]domain.Opportunity
	commercial    *commercialMemory
	saas          *saasMemory
	notifications *notificationMemory
}

func (m *Memory) Ping(context.Context) error { return nil }

func NewMemory() *Memory {
	memory := &Memory{profiles: map[string]domain.CommercialProfile{}, opportunities: map[string]domain.Opportunity{}}
	memory.commercial = newCommercialMemory()
	demoOrg := domain.Organization{ID: "00000000-0000-0000-0000-000000000001", Name: "LicitaLens Demo", Status: "active", CreatedAt: time.Now().UTC()}
	memory.commercial.organizations[demoOrg.ID] = demoOrg
	memory.commercial.memberships[demoOrg.ID] = map[string]string{"demo-user": "owner", "mobile-user": "owner"}
	memory.commercial.subscriptions[demoOrg.ID] = billing.Subscription{Plan: "pro", Status: "active", UpdatedAt: time.Now().UTC()}
	memory.notifications = &notificationMemory{
		sent:          map[string]struct{}{},
		pushTokens:    map[string][]string{},
		opportunityAt: map[string]time.Time{},
	}
	return memory
}

func (m *Memory) IsMember(_ context.Context, organizationID, subjectID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return false, nil
	}
	members, ok := m.commercial.memberships[organizationID]
	if !ok {
		return false, nil
	}
	_, ok = members[subjectID]
	return ok, nil
}

func (m *Memory) Subscription(_ context.Context, organizationID string) (billing.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial != nil {
		if subscription, ok := m.commercial.subscriptions[organizationID]; ok {
			return subscription, nil
		}
	}
	return billing.Subscription{}, ErrNotFound
}

func (m *Memory) ApplySubscription(_ context.Context, organizationID string, subscription billing.Subscription, eventID, _ string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	m.commercial.subscriptions[organizationID] = subscription
	m.recordHistoryLocked(organizationID, subscription, eventID)
	return true, nil
}

func (m *Memory) ConsumeUsage(_ context.Context, organizationID, period, kind string, limit int) (bool, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	used := m.bumpUsageLocked(organizationID, period, kind)
	return used <= limit, used, nil
}

func (m *Memory) PutProfile(profile domain.CommercialProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profiles[profile.ID] = profile
	return nil
}

func (m *Memory) Profiles(organizationID string) []domain.CommercialProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]domain.CommercialProfile, 0)
	for _, profile := range m.profiles {
		if profile.OrganizationID == organizationID {
			result = append(result, profile)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (m *Memory) Profile(id, organizationID string) (domain.CommercialProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	profile, ok := m.profiles[id]
	if !ok || profile.OrganizationID != organizationID {
		return domain.CommercialProfile{}, ErrNotFound
	}
	return profile, nil
}

func (m *Memory) DeleteProfile(id, organizationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	profile, ok := m.profiles[id]
	if !ok || profile.OrganizationID != organizationID {
		return ErrNotFound
	}
	delete(m.profiles, id)
	return nil
}

func (m *Memory) PutOpportunity(opportunity domain.Opportunity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opportunities[opportunity.ID] = opportunity
	return nil
}

func (m *Memory) Opportunity(id string) (domain.Opportunity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.opportunities[id]
	if !ok {
		return domain.Opportunity{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) Opportunities() []domain.Opportunity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]domain.Opportunity, 0, len(m.opportunities))
	for _, opportunity := range m.opportunities {
		result = append(result, opportunity)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PublishedAt.After(result[j].PublishedAt) })
	return result
}

func SeedDemo(m *Memory) {
	now := time.Now().UTC()
	m.PutOpportunity(domain.Opportunity{ID: "demo-1", Source: "demo", SourceID: "demo-1", Object: "Aquisição de notebooks corporativos com 16 GB de memória", OrganizationName: "Município de Curitiba", State: "PR", Municipality: "Curitiba", ModalityCode: 6, EstimatedValueCents: 180_000_00, PublishedAt: now.Add(-2 * time.Hour), ProposalDeadline: now.Add(10 * 24 * time.Hour), UpdatedAt: now, SourceURL: "https://pncp.gov.br/"})
}
