package domain

import "time"

type NotificationTarget struct {
	OrganizationID   string                  `json:"organization_id"`
	OrganizationName string                  `json:"organization_name"`
	OwnerEmail       string                  `json:"owner_email"`
	PushTokens       []string                `json:"push_tokens,omitempty"`
	Preferences      NotificationPreferences `json:"preferences"`
}

type NotificationRunResult struct {
	Organizations   int `json:"organizations"`
	OpportunitySent int `json:"opportunity_alerts_sent"`
	FollowUpSent    int `json:"followup_reminders_sent"`
	SkippedQuota    int `json:"skipped_quota"`
	SkippedDedup    int `json:"skipped_dedup"`
	SkippedStale    int `json:"skipped_stale"`
	Errors          int `json:"errors"`
	Suppressed      int `json:"suppressed"`
}

func DealFollowUpDue(deal Deal, now time.Time) bool {
	if deal.NextFollowUpAt == nil {
		return false
	}
	switch deal.Stage {
	case StageWon, StageLost:
		return false
	default:
		return !deal.NextFollowUpAt.After(now)
	}
}
