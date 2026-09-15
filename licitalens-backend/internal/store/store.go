package store

import (
	"context"
	"time"

	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

// DataStore is the persistence boundary shared by the HTTP gateway and the
// procurement consumer. Memory is used only for the demo mode.
type DataStore interface {
	IsMember(context.Context, string, string) (bool, error)
	Subscription(context.Context, string) (billing.Subscription, error)
	ApplySubscription(context.Context, string, billing.Subscription, string, string) (bool, error)
	ConsumeUsage(context.Context, string, string, string, int) (bool, int, error)
	UsageAmount(context.Context, string, string, string) (int, error)
	OrganizationsForSubject(context.Context, string) ([]domain.Organization, error)
	BootstrapOrganization(context.Context, string, string) (domain.Organization, error)
	OrganizationStripeCustomer(context.Context, string) (string, error)
	SetOrganizationStripeCustomer(context.Context, string, string) error
	SubscriptionHistory(context.Context, string, int) ([]domain.SubscriptionHistoryEntry, error)
	AdminOverview(context.Context) (domain.AdminOverview, error)
	AdminOrganizations(context.Context) ([]domain.AdminOrganization, error)
	AdminSubscriptionHistory(context.Context, int) ([]domain.SubscriptionHistoryEntry, error)
	RegisterSaaSAccount(context.Context, string, string, string, string, string) (domain.Account, domain.Organization, billing.Subscription, error)
	AccountByEmail(context.Context, string) (domain.Account, string, error)
	NotificationPreferences(context.Context, string) (domain.NotificationPreferences, error)
	PutNotificationPreferences(context.Context, string, domain.NotificationPreferences) (domain.NotificationPreferences, error)
	NotificationTargets(context.Context) ([]domain.NotificationTarget, error)
	NotificationAlertSent(context.Context, string, string, string) (bool, error)
	RecordNotificationAlert(context.Context, string, string, string, string, string) error
	RegisterPushToken(context.Context, string, string) error
	RemovePushToken(context.Context, string, string) error
	NotificationOpportunityCursor(context.Context, string) (time.Time, error)
	AdvanceNotificationOpportunityCursor(context.Context, string, time.Time) error
	Deals(context.Context, string) ([]domain.Deal, error)
	Deal(context.Context, string, string) (domain.Deal, error)
	DealByOpportunity(context.Context, string, string) (domain.Deal, error)
	CreateDeal(context.Context, domain.Deal) (domain.Deal, error)
	UpdateDeal(context.Context, domain.Deal) (domain.Deal, error)
	DealFollowUps(context.Context, string, string) ([]domain.DealFollowUp, error)
	AddDealFollowUp(context.Context, string, string, string, *time.Time) (domain.DealFollowUp, error)
	PutProfile(domain.CommercialProfile) error
	Profiles(string) []domain.CommercialProfile
	Profile(string, string) (domain.CommercialProfile, error)
	DeleteProfile(string, string) error
	PutOpportunity(domain.Opportunity) error
	Opportunity(string) (domain.Opportunity, error)
	Opportunities() []domain.Opportunity
}
