package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

type platformMemory struct {
	mu        sync.RWMutex
	operators map[string]domain.PlatformOperator
	password  map[string]string
	revokedAt map[string]time.Time
	byEmail   map[string]string
}

func newPlatformMemory() *platformMemory {
	return &platformMemory{
		operators: map[string]domain.PlatformOperator{},
		password:  map[string]string{},
		revokedAt: map[string]time.Time{},
		byEmail:   map[string]string{},
	}
}

func (m *Memory) ensurePlatform() {
	if m.platform == nil {
		m.platform = newPlatformMemory()
	}
}

func (m *Memory) RegisterPlatformOperator(_ context.Context, email, passwordHash, fullName string) (domain.PlatformOperator, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePlatform()
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || passwordHash == "" || strings.TrimSpace(fullName) == "" {
		return domain.PlatformOperator{}, errors.New("invalid platform operator")
	}
	if m.platform.byEmail[email] != "" {
		return domain.PlatformOperator{}, errors.New("email already registered")
	}
	subjectID := uuid.NewString()
	operator := domain.PlatformOperator{
		ID:        uuid.NewString(),
		Email:     email,
		FullName:  fullName,
		SubjectID: subjectID,
		CreatedAt: time.Now().UTC(),
	}
	m.platform.operators[subjectID] = operator
	m.platform.password[subjectID] = passwordHash
	m.platform.byEmail[email] = subjectID
	return operator, nil
}

func (m *Memory) PlatformOperatorByEmail(_ context.Context, email string) (domain.PlatformOperator, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensurePlatform()
	email = strings.ToLower(strings.TrimSpace(email))
	subjectID := m.platform.byEmail[email]
	if subjectID == "" {
		return domain.PlatformOperator{}, "", ErrNotFound
	}
	operator := m.platform.operators[subjectID]
	hash := m.platform.password[subjectID]
	return operator, hash, nil
}

func (m *Memory) PlatformOperatorBySubject(_ context.Context, subjectID string) (domain.PlatformOperator, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensurePlatform()
	operator, ok := m.platform.operators[subjectID]
	if !ok {
		return domain.PlatformOperator{}, ErrNotFound
	}
	return operator, nil
}

func (m *Memory) PlatformSessionActive(_ context.Context, subjectID string, issuedAt time.Time) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.ensurePlatform()
	if _, ok := m.platform.operators[subjectID]; !ok {
		return false, nil
	}
	revoked := m.platform.revokedAt[subjectID]
	return revoked.IsZero() || issuedAt.After(revoked), nil
}

func (m *Memory) RevokePlatformSessions(_ context.Context, subjectID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePlatform()
	if _, ok := m.platform.operators[subjectID]; !ok {
		return ErrNotFound
	}
	m.platform.revokedAt[subjectID] = at.UTC()
	return nil
}

func (m *Memory) ApplyPlatformSubscription(_ context.Context, organizationID string, subscription billing.Subscription, operatorSubjectID, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commercial == nil {
		m.commercial = newCommercialMemory()
	}
	if _, ok := m.commercial.organizations[organizationID]; !ok {
		return ErrNotFound
	}
	if _, ok := billing.Plan(subscription.Plan); !ok {
		return errors.New("invalid plan")
	}
	subscription.UpdatedAt = time.Now().UTC()
	if subscription.ExternalID == "" {
		subscription.ExternalID = "manual"
	}
	m.commercial.subscriptions[organizationID] = subscription
	auditRef := "operator:" + operatorSubjectID
	if strings.TrimSpace(note) != "" {
		auditRef += "|" + strings.TrimSpace(note)
	}
	m.recordHistoryLocked(organizationID, subscription, auditRef)
	org := m.commercial.organizations[organizationID]
	org.Status = organizationStatus(subscription.Status)
	m.commercial.organizations[organizationID] = org
	return nil
}
