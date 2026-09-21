package store

import (
	"context"
	"time"

	"licitalens.dev/backend/internal/domain"
)

type notificationMemory struct {
	sent          map[string]struct{}
	pushTokens    map[string][]string
	opportunityAt map[string]time.Time
	failures      map[string]int
}

func (m *Memory) notificationState() *notificationMemory {
	if m.notifications == nil {
		m.notifications = &notificationMemory{
			sent:          map[string]struct{}{},
			pushTokens:    map[string][]string{},
			opportunityAt: map[string]time.Time{},
		}
	}
	return m.notifications
}

func alertKey(organizationID, kind, dedupeID string) string {
	return organizationID + "|" + kind + "|" + dedupeID
}

func (m *Memory) NotificationTargets(_ context.Context) ([]domain.NotificationTarget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.commercial == nil {
		return nil, nil
	}
	state := m.notificationState()
	result := []domain.NotificationTarget{}
	for orgID, org := range m.commercial.organizations {
		email := m.ownerEmailLocked(orgID)
		tokens := append([]string(nil), state.pushTokens[orgID]...)
		if email == "" && len(tokens) == 0 {
			continue
		}
		prefs := domain.DefaultNotificationPreferences()
		if p, ok := m.commercial.notificationPrefs[orgID]; ok {
			prefs = p
		}
		result = append(result, domain.NotificationTarget{
			OrganizationID:   orgID,
			OrganizationName: org.Name,
			OwnerEmail:       email,
			PushTokens:       tokens,
			Preferences:      prefs,
		})
	}
	return result, nil
}

func (m *Memory) ownerEmailLocked(organizationID string) string {
	if m.commercial == nil || m.saas == nil {
		return ""
	}
	for subjectID := range m.commercial.memberships[organizationID] {
		for _, account := range m.saas.accounts {
			if account.SubjectID == subjectID {
				return account.Email
			}
		}
	}
	return ""
}

func (m *Memory) NotificationAlertSent(_ context.Context, organizationID, kind, dedupeID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.notificationState()
	_, ok := state.sent[alertKey(organizationID, kind, dedupeID)]
	return ok, nil
}

func (m *Memory) RecordNotificationAlert(_ context.Context, organizationID, kind, dedupeID, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.notificationState()
	state.sent[alertKey(organizationID, kind, dedupeID)] = struct{}{}
	return nil
}

func (m *Memory) RecordNotificationFailure(_ context.Context, organizationID, kind, dedupeID, _, _ string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.notifications == nil {
		m.notifications = &notificationMemory{sent: map[string]struct{}{}, pushTokens: map[string][]string{}, opportunityAt: map[string]time.Time{}, failures: map[string]int{}}
	}
	if m.notifications.failures == nil {
		m.notifications.failures = map[string]int{}
	}
	key := organizationID + ":" + alertDedupeKey(kind, dedupeID)
	m.notifications.failures[key]++
	if m.notifications.failures[key] >= 5 {
		m.notifications.sent[alertKey(organizationID, kind, dedupeID)] = struct{}{}
	}
	return m.notifications.failures[key], nil
}

func (m *Memory) RegisterPushToken(_ context.Context, organizationID, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.notificationState()
	for _, existing := range state.pushTokens[organizationID] {
		if existing == token {
			return nil
		}
	}
	state.pushTokens[organizationID] = append(state.pushTokens[organizationID], token)
	return nil
}

func (m *Memory) RemovePushToken(_ context.Context, organizationID, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.notificationState()
	tokens := state.pushTokens[organizationID]
	next := make([]string, 0, len(tokens))
	for _, item := range tokens {
		if item != token {
			next = append(next, item)
		}
	}
	state.pushTokens[organizationID] = next
	return nil
}

func (m *Memory) NotificationOpportunityCursor(_ context.Context, organizationID string) (time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.notificationState()
	return state.opportunityAt[organizationID], nil
}

func (m *Memory) AdvanceNotificationOpportunityCursor(_ context.Context, organizationID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.notificationState()
	current := state.opportunityAt[organizationID]
	if at.After(current) {
		state.opportunityAt[organizationID] = at.UTC()
	}
	return nil
}
