package notifications

import (
	"time"

	"licitalens.dev/backend/internal/domain"
)

const defaultOpportunityLookback = 24 * time.Hour

func opportunityCutoff(cursor time.Time, now time.Time, lookback time.Duration) time.Time {
	if lookback <= 0 {
		lookback = defaultOpportunityLookback
	}
	if cursor.IsZero() || cursor.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return now.Add(-lookback)
	}
	return cursor
}

func isNewOpportunity(opportunity domain.Opportunity, cutoff time.Time) bool {
	if opportunity.PublishedAt.IsZero() {
		return false
	}
	return opportunity.PublishedAt.After(cutoff)
}
