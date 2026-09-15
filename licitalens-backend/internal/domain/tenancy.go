package domain

import "time"

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type SubscriptionHistoryEntry struct {
	ID                   string    `json:"id"`
	OrganizationID       string    `json:"organization_id"`
	OrganizationName     string    `json:"organization_name,omitempty"`
	StripeSubscriptionID string  `json:"stripe_subscription_id,omitempty"`
	Plan                 string    `json:"plan"`
	Status               string    `json:"status"`
	StripeEventID        string    `json:"stripe_event_id,omitempty"`
	RecordedAt           time.Time `json:"recorded_at"`
}

type AdminOverview struct {
	Organizations      int `json:"organizations"`
	ActiveSubscriptions int `json:"active_subscriptions"`
	EssentialPlans     int `json:"essential_plans"`
	ProPlans           int `json:"pro_plans"`
	HistoryEvents      int `json:"history_events"`
}

type AdminOrganization struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Status             string    `json:"status"`
	Plan               string    `json:"plan,omitempty"`
	SubscriptionStatus string    `json:"subscription_status,omitempty"`
	MemberCount        int       `json:"member_count"`
	PipelineDeals      int       `json:"pipeline_deals"`
	CreatedAt          time.Time `json:"created_at"`
}

type NotificationPreferences struct {
	Push             bool `json:"push"`
	Email            bool `json:"email"`
	WhatsApp         bool `json:"whatsapp"`
	DeadlineReminder bool `json:"deadline_reminder"`
}

func DefaultNotificationPreferences() NotificationPreferences {
	return NotificationPreferences{Push: true, Email: true, WhatsApp: false, DeadlineReminder: true}
}
