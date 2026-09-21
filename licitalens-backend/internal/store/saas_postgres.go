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

func (p *Postgres) RegisterSaaSAccount(ctx context.Context, email, passwordHash, fullName, organizationName, plan string, legal domain.LegalAcceptance) (domain.Account, domain.Organization, billing.Subscription, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || passwordHash == "" || strings.TrimSpace(fullName) == "" || strings.TrimSpace(organizationName) == "" {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, errors.New("invalid registration")
	}
	if _, ok := billing.Plan(plan); !ok {
		plan = "essential"
	}
	subjectID := uuid.NewString()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, err
	}
	defer tx.Rollback(ctx)
	var account domain.Account
	var acceptedAt *time.Time
	if !legal.AcceptedAt.IsZero() {
		value := legal.AcceptedAt.UTC()
		acceptedAt = &value
	}
	err = tx.QueryRow(ctx, `INSERT INTO tenancy.accounts(email,password_hash,full_name,subject_id,terms_version,privacy_version,legal_accepted_at) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7) RETURNING id::text,email,full_name,subject_id,COALESCE(terms_version,''),COALESCE(privacy_version,''),legal_accepted_at,created_at`, email, passwordHash, fullName, subjectID, legal.TermsVersion, legal.PrivacyVersion, acceptedAt).Scan(&account.ID, &account.Email, &account.FullName, &account.SubjectID, &account.TermsVersion, &account.PrivacyVersion, &account.LegalAcceptedAt, &account.CreatedAt)
	if err != nil {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, err
	}
	var org domain.Organization
	err = tx.QueryRow(ctx, `INSERT INTO tenancy.organizations(name, status) VALUES ($1,'active') RETURNING id::text,name,status,created_at`, organizationName).Scan(&org.ID, &org.Name, &org.Status, &org.CreatedAt)
	if err != nil {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenancy.memberships(organization_id, subject_id, role) VALUES ($1::uuid,$2,'owner')`, org.ID, subjectID)
	if err != nil {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, err
	}
	var subscription billing.Subscription
	err = tx.QueryRow(ctx, `INSERT INTO tenancy.subscriptions(organization_id, plan, status, current_period_end) VALUES ($1::uuid,$2,'trialing',now() + interval '14 days') RETURNING COALESCE(stripe_subscription_id,''), plan, status, current_period_end, updated_at`, org.ID, plan).Scan(&subscription.ExternalID, &subscription.Plan, &subscription.Status, &subscription.CurrentPeriodEnd, &subscription.UpdatedAt)
	if err != nil {
		return domain.Account{}, domain.Organization{}, billing.Subscription{}, err
	}
	return account, org, subscription, tx.Commit(ctx)
}

func (p *Postgres) AccountByEmail(ctx context.Context, email string) (domain.Account, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var account domain.Account
	var passwordHash string
	err := p.pool.QueryRow(ctx, `SELECT id::text,email,full_name,subject_id,COALESCE(terms_version,''),COALESCE(privacy_version,''),legal_accepted_at,created_at,password_hash FROM tenancy.accounts WHERE lower(email)=$1`, email).Scan(&account.ID, &account.Email, &account.FullName, &account.SubjectID, &account.TermsVersion, &account.PrivacyVersion, &account.LegalAcceptedAt, &account.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, "", ErrNotFound
	}
	return account, passwordHash, err
}

func (p *Postgres) AccountBySubject(ctx context.Context, subjectID string) (domain.Account, error) {
	var account domain.Account
	err := p.pool.QueryRow(ctx, `SELECT id::text,email,full_name,subject_id,COALESCE(terms_version,''),COALESCE(privacy_version,''),legal_accepted_at,created_at FROM tenancy.accounts WHERE subject_id=$1`, subjectID).Scan(&account.ID, &account.Email, &account.FullName, &account.SubjectID, &account.TermsVersion, &account.PrivacyVersion, &account.LegalAcceptedAt, &account.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, ErrNotFound
	}
	return account, err
}

func (p *Postgres) CreateAccountToken(ctx context.Context, email, purpose, tokenHash string, expiresAt time.Time) error {
	_, err := p.pool.Exec(ctx, `WITH account AS (SELECT id FROM tenancy.accounts WHERE lower(email)=lower($1)),
invalidated AS (UPDATE tenancy.account_tokens SET consumed_at=now() WHERE account_id IN (SELECT id FROM account) AND purpose=$2 AND consumed_at IS NULL)
INSERT INTO tenancy.account_tokens(account_id,purpose,token_hash,expires_at) SELECT id,$2,$3,$4 FROM account`, email, purpose, tokenHash, expiresAt.UTC())
	return err
}

func (p *Postgres) ConsumeAccountToken(ctx context.Context, purpose, tokenHash string) (domain.Account, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	defer tx.Rollback(ctx)
	var account domain.Account
	err = tx.QueryRow(ctx, `UPDATE tenancy.account_tokens t SET consumed_at=now() FROM tenancy.accounts a
WHERE t.account_id=a.id AND t.purpose=$1 AND t.token_hash=$2 AND t.consumed_at IS NULL AND t.expires_at>now()
RETURNING a.id::text,a.email,a.full_name,a.subject_id,a.created_at`, purpose, tokenHash).Scan(&account.ID, &account.Email, &account.FullName, &account.SubjectID, &account.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, ErrNotFound
	}
	if err != nil {
		return account, err
	}
	return account, tx.Commit(ctx)
}

func (p *Postgres) MarkEmailVerified(ctx context.Context, subjectID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE tenancy.accounts SET email_verified_at=now() WHERE subject_id=$1`, subjectID)
	return err
}
func (p *Postgres) EmailVerified(ctx context.Context, subjectID string) (bool, error) {
	var verified bool
	err := p.pool.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM tenancy.accounts WHERE subject_id=$1`, subjectID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return verified, err
}
func (p *Postgres) UpdatePassword(ctx context.Context, subjectID, passwordHash string) error {
	_, err := p.pool.Exec(ctx, `UPDATE tenancy.accounts SET password_hash=$2, sessions_revoked_at=now() WHERE subject_id=$1`, subjectID, passwordHash)
	return err
}
func (p *Postgres) DeleteAccount(ctx context.Context, subjectID string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM notifications.deliveries WHERE organization_id IN (SELECT organization_id FROM tenancy.memberships WHERE subject_id=$1 AND role='owner')`, subjectID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM notifications.channels WHERE organization_id IN (SELECT organization_id FROM tenancy.memberships WHERE subject_id=$1 AND role='owner')`, subjectID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM intelligence.matches WHERE organization_id IN (SELECT organization_id FROM tenancy.memberships WHERE subject_id=$1 AND role='owner')`, subjectID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM intelligence.usage_ledger WHERE organization_id IN (SELECT organization_id FROM tenancy.memberships WHERE subject_id=$1 AND role='owner')`, subjectID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM tenancy.organizations WHERE id IN (SELECT organization_id FROM tenancy.memberships WHERE subject_id=$1 AND role='owner')`, subjectID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tenancy.accounts WHERE subject_id=$1`, subjectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (p *Postgres) SessionActive(ctx context.Context, subjectID string, issuedAt time.Time) (bool, error) {
	var active bool
	err := p.pool.QueryRow(ctx, `SELECT sessions_revoked_at IS NULL OR sessions_revoked_at < $2 FROM tenancy.accounts WHERE subject_id=$1`, subjectID, issuedAt).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func (p *Postgres) RevokeSessions(ctx context.Context, subjectID string, at time.Time) error {
	command, err := p.pool.Exec(ctx, `UPDATE tenancy.accounts SET sessions_revoked_at=$2 WHERE subject_id=$1`, subjectID, at.UTC())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) Deals(ctx context.Context, organizationID string) ([]domain.Deal, error) {
	rows, err := p.pool.Query(ctx, `
SELECT d.id::text, d.organization_id::text, COALESCE(d.opportunity_id,''), d.title, d.buyer_name, d.stage, d.estimated_value_cents, d.next_follow_up_at, d.closed_at, d.created_at, d.updated_at,
  COALESCE((SELECT note FROM crm.deal_followups f WHERE f.deal_id=d.id ORDER BY created_at DESC LIMIT 1),'')
FROM crm.deals d
WHERE d.organization_id=$1::uuid
ORDER BY d.updated_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Deal{}
	for rows.Next() {
		var deal domain.Deal
		var stage string
		if err := rows.Scan(&deal.ID, &deal.OrganizationID, &deal.OpportunityID, &deal.Title, &deal.BuyerName, &stage, &deal.EstimatedValueCents, &deal.NextFollowUpAt, &deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt, &deal.LastFollowUpNote); err != nil {
			return nil, err
		}
		deal.Stage = domain.DealStage(stage)
		result = append(result, deal)
	}
	return result, nil
}

func (p *Postgres) Deal(ctx context.Context, organizationID, dealID string) (domain.Deal, error) {
	var deal domain.Deal
	var stage string
	err := p.pool.QueryRow(ctx, `
SELECT d.id::text, d.organization_id::text, COALESCE(d.opportunity_id,''), d.title, d.buyer_name, d.stage, d.estimated_value_cents, d.next_follow_up_at, d.closed_at, d.created_at, d.updated_at,
  COALESCE((SELECT note FROM crm.deal_followups f WHERE f.deal_id=d.id ORDER BY created_at DESC LIMIT 1),'')
FROM crm.deals d
WHERE d.organization_id=$1::uuid AND d.id=$2::uuid`, organizationID, dealID).Scan(
		&deal.ID, &deal.OrganizationID, &deal.OpportunityID, &deal.Title, &deal.BuyerName, &stage, &deal.EstimatedValueCents, &deal.NextFollowUpAt, &deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt, &deal.LastFollowUpNote,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deal{}, ErrNotFound
	}
	deal.Stage = domain.DealStage(stage)
	return deal, err
}

func (p *Postgres) DealFollowUps(ctx context.Context, organizationID, dealID string) ([]domain.DealFollowUp, error) {
	rows, err := p.pool.Query(ctx, `
SELECT f.id::text, f.deal_id::text, f.note, f.scheduled_at, f.created_at
FROM crm.deal_followups f
JOIN crm.deals d ON d.id=f.deal_id
WHERE d.organization_id=$1::uuid AND d.id=$2::uuid
ORDER BY f.created_at DESC`, organizationID, dealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.DealFollowUp{}
	for rows.Next() {
		var item domain.DealFollowUp
		if err := rows.Scan(&item.ID, &item.DealID, &item.Note, &item.ScheduledAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if len(result) == 0 {
		if _, err := p.Deal(ctx, organizationID, dealID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *Postgres) DealByOpportunity(ctx context.Context, organizationID, opportunityID string) (domain.Deal, error) {
	opportunityID = strings.TrimSpace(opportunityID)
	if opportunityID == "" {
		return domain.Deal{}, ErrNotFound
	}
	var deal domain.Deal
	var stage string
	err := p.pool.QueryRow(ctx, `
SELECT d.id::text, d.organization_id::text, COALESCE(d.opportunity_id,''), d.title, d.buyer_name, d.stage, d.estimated_value_cents, d.next_follow_up_at, d.closed_at, d.created_at, d.updated_at,
  COALESCE((SELECT note FROM crm.deal_followups f WHERE f.deal_id=d.id ORDER BY created_at DESC LIMIT 1),'')
FROM crm.deals d
WHERE d.organization_id=$1::uuid AND d.opportunity_id=$2
ORDER BY d.updated_at DESC
LIMIT 1`, organizationID, opportunityID).Scan(
		&deal.ID, &deal.OrganizationID, &deal.OpportunityID, &deal.Title, &deal.BuyerName, &stage, &deal.EstimatedValueCents, &deal.NextFollowUpAt, &deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt, &deal.LastFollowUpNote,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deal{}, ErrNotFound
	}
	deal.Stage = domain.DealStage(stage)
	return deal, err
}

func (p *Postgres) CreateDeal(ctx context.Context, deal domain.Deal) (domain.Deal, error) {
	if !deal.Stage.Valid() {
		deal.Stage = domain.StageProspecting
	}
	now := time.Now().UTC()
	var stage string
	err := p.pool.QueryRow(ctx, `
INSERT INTO crm.deals(organization_id,opportunity_id,title,buyer_name,stage,estimated_value_cents,next_follow_up_at,updated_at)
VALUES ($1::uuid,NULLIF($2,''),$3,$4,$5,$6,$7,$8)
RETURNING id::text, organization_id::text, COALESCE(opportunity_id,''), title, buyer_name, stage, estimated_value_cents, next_follow_up_at, closed_at, created_at, updated_at`,
		deal.OrganizationID, deal.OpportunityID, deal.Title, deal.BuyerName, string(deal.Stage), deal.EstimatedValueCents, deal.NextFollowUpAt, now,
	).Scan(&deal.ID, &deal.OrganizationID, &deal.OpportunityID, &deal.Title, &deal.BuyerName, &stage, &deal.EstimatedValueCents, &deal.NextFollowUpAt, &deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt)
	deal.Stage = domain.DealStage(stage)
	return deal, err
}

func (p *Postgres) UpdateDeal(ctx context.Context, deal domain.Deal) (domain.Deal, error) {
	if !deal.Stage.Valid() {
		return domain.Deal{}, errors.New("invalid stage")
	}
	now := time.Now().UTC()
	var closedAt any
	if deal.Stage == domain.StageWon || deal.Stage == domain.StageLost {
		closedAt = now
	}
	var stage string
	err := p.pool.QueryRow(ctx, `
UPDATE crm.deals SET title=$3,buyer_name=$4,stage=$5,estimated_value_cents=$6,next_follow_up_at=$7,closed_at=COALESCE($8,closed_at),updated_at=$9
WHERE id=$1::uuid AND organization_id=$2::uuid
RETURNING id::text, organization_id::text, COALESCE(opportunity_id,''), title, buyer_name, stage, estimated_value_cents, next_follow_up_at, closed_at, created_at, updated_at`,
		deal.ID, deal.OrganizationID, deal.Title, deal.BuyerName, string(deal.Stage), deal.EstimatedValueCents, deal.NextFollowUpAt, closedAt, now,
	).Scan(&deal.ID, &deal.OrganizationID, &deal.OpportunityID, &deal.Title, &deal.BuyerName, &stage, &deal.EstimatedValueCents, &deal.NextFollowUpAt, &deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt)
	deal.Stage = domain.DealStage(stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deal{}, ErrNotFound
	}
	return deal, err
}

func (p *Postgres) AddDealFollowUp(ctx context.Context, organizationID, dealID, note string, scheduledAt *time.Time) (domain.DealFollowUp, error) {
	var followUp domain.DealFollowUp
	err := p.pool.QueryRow(ctx, `
WITH target AS (
  SELECT id FROM crm.deals WHERE id=$2::uuid AND organization_id=$1::uuid
)
INSERT INTO crm.deal_followups(deal_id, note, scheduled_at)
SELECT id, $3, $4 FROM target
RETURNING id::text, deal_id::text, note, scheduled_at, created_at`, organizationID, dealID, note, scheduledAt).Scan(&followUp.ID, &followUp.DealID, &followUp.Note, &followUp.ScheduledAt, &followUp.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return followUp, ErrNotFound
	}
	if err == nil {
		_, _ = p.pool.Exec(ctx, `UPDATE crm.deals SET next_follow_up_at=COALESCE($3,next_follow_up_at), updated_at=now() WHERE id=$1::uuid AND organization_id=$2::uuid`, dealID, organizationID, scheduledAt)
	}
	return followUp, err
}
