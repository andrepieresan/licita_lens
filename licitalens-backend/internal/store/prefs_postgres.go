package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"licitalens.dev/backend/internal/domain"
)

func (p *Postgres) NotificationPreferences(ctx context.Context, organizationID string) (domain.NotificationPreferences, error) {
	var raw []byte
	err := p.pool.QueryRow(ctx, `SELECT notification_preferences FROM tenancy.organizations WHERE id=$1::uuid`, organizationID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotificationPreferences{}, ErrNotFound
	}
	if err != nil {
		return domain.NotificationPreferences{}, err
	}
	var prefs domain.NotificationPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return domain.DefaultNotificationPreferences(), nil
	}
	return prefs, nil
}

func (p *Postgres) PutNotificationPreferences(ctx context.Context, organizationID string, prefs domain.NotificationPreferences) (domain.NotificationPreferences, error) {
	raw, err := json.Marshal(prefs)
	if err != nil {
		return domain.NotificationPreferences{}, err
	}
	tag, err := p.pool.Exec(ctx, `UPDATE tenancy.organizations SET notification_preferences=$2::jsonb WHERE id=$1::uuid`, organizationID, raw)
	if err != nil {
		return domain.NotificationPreferences{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.NotificationPreferences{}, ErrNotFound
	}
	return prefs, nil
}
