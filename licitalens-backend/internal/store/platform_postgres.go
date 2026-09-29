package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

func (p *Postgres) RegisterPlatformOperator(ctx context.Context, email, passwordHash, fullName string) (domain.PlatformOperator, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || passwordHash == "" || strings.TrimSpace(fullName) == "" {
		return domain.PlatformOperator{}, errors.New("invalid platform operator")
	}
	subjectID := uuid.NewString()
	var operator domain.PlatformOperator
	err := p.pool.QueryRow(ctx, `INSERT INTO tenancy.platform_operators(email,password_hash,full_name,subject_id) VALUES ($1,$2,$3,$4) RETURNING id::text,email,full_name,subject_id,created_at`, email, passwordHash, fullName, subjectID).Scan(&operator.ID, &operator.Email, &operator.FullName, &operator.SubjectID, &operator.CreatedAt)
	return operator, err
}

func (p *Postgres) PlatformOperatorByEmail(ctx context.Context, email string) (domain.PlatformOperator, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var operator domain.PlatformOperator
	var passwordHash string
	err := p.pool.QueryRow(ctx, `SELECT id::text,email,full_name,subject_id,created_at,password_hash FROM tenancy.platform_operators WHERE lower(email)=$1`, email).Scan(&operator.ID, &operator.Email, &operator.FullName, &operator.SubjectID, &operator.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return operator, "", ErrNotFound
	}
	return operator, passwordHash, err
}

func (p *Postgres) PlatformOperatorBySubject(ctx context.Context, subjectID string) (domain.PlatformOperator, error) {
	var operator domain.PlatformOperator
	err := p.pool.QueryRow(ctx, `SELECT id::text,email,full_name,subject_id,created_at FROM tenancy.platform_operators WHERE subject_id=$1`, subjectID).Scan(&operator.ID, &operator.Email, &operator.FullName, &operator.SubjectID, &operator.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return operator, ErrNotFound
	}
	return operator, err
}

func (p *Postgres) PlatformSessionActive(ctx context.Context, subjectID string, issuedAt time.Time) (bool, error) {
	var active bool
	err := p.pool.QueryRow(ctx, `SELECT sessions_revoked_at IS NULL OR sessions_revoked_at < $2 FROM tenancy.platform_operators WHERE subject_id=$1`, subjectID, issuedAt).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func (p *Postgres) RevokePlatformSessions(ctx context.Context, subjectID string, at time.Time) error {
	command, err := p.pool.Exec(ctx, `UPDATE tenancy.platform_operators SET sessions_revoked_at=$2 WHERE subject_id=$1`, subjectID, at.UTC())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) ApplyPlatformSubscription(ctx context.Context, organizationID string, subscription billing.Subscription, operatorSubjectID, note string) error {
	if _, ok := billing.Plan(subscription.Plan); !ok {
		return errors.New("invalid plan")
	}
	switch subscription.Status {
	case "active", "past_due", "canceled", "trialing":
	default:
		return errors.New("invalid subscription status")
	}
	if subscription.ExternalID == "" {
		subscription.ExternalID = "manual"
	}
	now := time.Now().UTC()
	subscription.UpdatedAt = now
	eventID := "platform:" + uuid.NewString()
	auditRef := "operator:" + operatorSubjectID
	if strings.TrimSpace(note) != "" {
		auditRef += "|" + strings.TrimSpace(note)
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO tenancy.webhook_events(provider, external_id, payload_hash) VALUES ('platform',$1,$2) ON CONFLICT DO NOTHING`, eventID, auditRef)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenancy.subscriptions(organization_id,stripe_subscription_id,plan,status,current_period_end,updated_at) VALUES ($1::uuid,NULLIF($2,''),$3,$4,$5,$6) ON CONFLICT(organization_id) DO UPDATE SET plan=EXCLUDED.plan,status=EXCLUDED.status,current_period_end=EXCLUDED.current_period_end,updated_at=EXCLUDED.updated_at`, organizationID, subscription.ExternalID, subscription.Plan, subscription.Status, subscription.CurrentPeriodEnd, subscription.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenancy.subscription_history(organization_id,stripe_subscription_id,plan,status,stripe_event_id,recorded_at) VALUES ($1::uuid,NULLIF($2,''),$3,$4,$5,$6)`, organizationID, subscription.ExternalID, subscription.Plan, subscription.Status, auditRef, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tenancy.organizations SET status=$2 WHERE id=$1::uuid`, organizationID, organizationStatus(subscription.Status))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tenancy.webhook_events SET processed_at=now() WHERE provider='platform' AND external_id=$1`, eventID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
