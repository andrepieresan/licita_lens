package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"licitalens.dev/backend/internal/domain"
)

func (p *Postgres) NotificationTargets(ctx context.Context) ([]domain.NotificationTarget, error) {
	rows, err := p.pool.Query(ctx, `
SELECT o.id::text, o.name, o.notification_preferences, a.email,
  COALESCE((
    SELECT array_agg(c.destination ORDER BY c.created_at)
    FROM notifications.channels c
    WHERE c.organization_id=o.id AND c.type='expo_push' AND c.enabled AND c.opted_out_at IS NULL
  ), ARRAY[]::text[])
FROM tenancy.organizations o
LEFT JOIN LATERAL (
  SELECT a2.email FROM tenancy.memberships m
  INNER JOIN tenancy.accounts a2 ON a2.subject_id=m.subject_id
  WHERE m.organization_id=o.id
  ORDER BY CASE WHEN m.role='owner' THEN 0 ELSE 1 END
  LIMIT 1
) a ON true
ORDER BY o.created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.NotificationTarget{}
	for rows.Next() {
		var item domain.NotificationTarget
		var raw []byte
		var email *string
		if err := rows.Scan(&item.OrganizationID, &item.OrganizationName, &raw, &email, &item.PushTokens); err != nil {
			return nil, err
		}
		if email != nil {
			item.OwnerEmail = *email
		}
		item.Preferences = domain.DefaultNotificationPreferences()
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &item.Preferences)
		}
		if item.OwnerEmail == "" && len(item.PushTokens) == 0 {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (p *Postgres) NotificationAlertSent(ctx context.Context, organizationID, kind, dedupeID string) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM notifications.deliveries
  WHERE organization_id=$1::uuid AND opportunity_id=$2 AND status='sent'
)`, organizationID, alertDedupeKey(kind, dedupeID)).Scan(&exists)
	return exists, err
}

func (p *Postgres) RecordNotificationAlert(ctx context.Context, organizationID, kind, dedupeID, destination, providerMessageID string) error {
	channelType := "email"
	if kind == "push" {
		channelType = "expo_push"
	}
	if destination == "" {
		var err error
		if channelType == "email" {
			destination, err = p.ownerEmail(ctx, organizationID)
		} else {
			return errors.New("push destination required")
		}
		if err != nil {
			return err
		}
	}
	channelID, err := p.ensureChannel(ctx, organizationID, channelType, destination)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
INSERT INTO notifications.deliveries(organization_id, channel_id, opportunity_id, status, provider_message_id, updated_at)
VALUES ($1::uuid, $2::uuid, $3, 'sent', $4, now())
ON CONFLICT (channel_id, opportunity_id) DO UPDATE SET status='sent', provider_message_id=EXCLUDED.provider_message_id, updated_at=now()`,
		organizationID, channelID, alertDedupeKey(kind, dedupeID), providerMessageID)
	return err
}

func (p *Postgres) RegisterPushToken(ctx context.Context, organizationID, token string) error {
	_, err := p.ensureChannel(ctx, organizationID, "expo_push", token)
	return err
}

func (p *Postgres) RemovePushToken(ctx context.Context, organizationID, token string) error {
	_, err := p.pool.Exec(ctx, `
UPDATE notifications.channels
SET enabled=false, opted_out_at=now()
WHERE organization_id=$1::uuid AND type='expo_push' AND destination=$2`, organizationID, token)
	return err
}

func (p *Postgres) NotificationOpportunityCursor(ctx context.Context, organizationID string) (time.Time, error) {
	var cursor time.Time
	err := p.pool.QueryRow(ctx, `
SELECT last_opportunity_at FROM tenancy.notification_scan WHERE organization_id=$1::uuid`, organizationID).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return cursor, err
}

func (p *Postgres) AdvanceNotificationOpportunityCursor(ctx context.Context, organizationID string, at time.Time) error {
	_, err := p.pool.Exec(ctx, `
INSERT INTO tenancy.notification_scan(organization_id, last_opportunity_at)
VALUES ($1::uuid, $2)
ON CONFLICT (organization_id) DO UPDATE SET last_opportunity_at=GREATEST(tenancy.notification_scan.last_opportunity_at, EXCLUDED.last_opportunity_at)`,
		organizationID, at.UTC())
	return err
}

func alertDedupeKey(kind, dedupeID string) string {
	return kind + ":" + dedupeID
}

func (p *Postgres) ownerEmail(ctx context.Context, organizationID string) (string, error) {
	var email string
	err := p.pool.QueryRow(ctx, `
SELECT a.email FROM tenancy.accounts a
INNER JOIN tenancy.memberships m ON m.subject_id=a.subject_id
WHERE m.organization_id=$1::uuid
ORDER BY CASE WHEN m.role='owner' THEN 0 ELSE 1 END
LIMIT 1`, organizationID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return email, err
}

func (p *Postgres) ensureEmailChannel(ctx context.Context, organizationID, destination string) (string, error) {
	return p.ensureChannel(ctx, organizationID, "email", destination)
}

func (p *Postgres) ensureChannel(ctx context.Context, organizationID, channelType, destination string) (string, error) {
	if destination == "" && channelType == "email" {
		var err error
		destination, err = p.ownerEmail(ctx, organizationID)
		if err != nil {
			return "", err
		}
	}
	var channelID string
	err := p.pool.QueryRow(ctx, `
INSERT INTO notifications.channels(organization_id, type, destination, enabled, consented_at)
VALUES ($1::uuid, $2, $3, true, now())
ON CONFLICT (organization_id, type, destination) DO UPDATE SET enabled=true, opted_out_at=NULL, consented_at=COALESCE(notifications.channels.consented_at, now())
RETURNING id::text`, organizationID, channelType, destination).Scan(&channelID)
	return channelID, err
}
