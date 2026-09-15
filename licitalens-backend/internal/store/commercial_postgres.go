package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"licitalens.dev/backend/internal/domain"
)

func (p *Postgres) UsageAmount(ctx context.Context, organizationID, period, kind string) (int, error) {
	var used int
	err := p.pool.QueryRow(ctx, `SELECT COALESCE((SELECT used FROM tenancy.usage_counters WHERE organization_id=$1::uuid AND period_start=$2::date AND kind=$3), 0)`, organizationID, period, kind).Scan(&used)
	return used, err
}

func (p *Postgres) OrganizationsForSubject(ctx context.Context, subjectID string) ([]domain.Organization, error) {
	rows, err := p.pool.Query(ctx, `SELECT o.id::text, o.name, o.status, o.created_at FROM tenancy.organizations o INNER JOIN tenancy.memberships m ON m.organization_id=o.id WHERE m.subject_id=$1 ORDER BY o.created_at`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Organization{}
	for rows.Next() {
		var item domain.Organization
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (p *Postgres) BootstrapOrganization(ctx context.Context, subjectID, name string) (domain.Organization, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Organization{}, err
	}
	defer tx.Rollback(ctx)
	var org domain.Organization
	err = tx.QueryRow(ctx, `INSERT INTO tenancy.organizations(name, status) VALUES ($1, 'active') RETURNING id::text, name, status, created_at`, name).Scan(&org.ID, &org.Name, &org.Status, &org.CreatedAt)
	if err != nil {
		return domain.Organization{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenancy.memberships(organization_id, subject_id, role) VALUES ($1::uuid, $2, 'owner')`, org.ID, subjectID)
	if err != nil {
		return domain.Organization{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenancy.subscriptions(organization_id, plan, status) VALUES ($1::uuid, 'essential', 'trialing') ON CONFLICT (organization_id) DO NOTHING`, org.ID)
	if err != nil {
		return domain.Organization{}, err
	}
	return org, tx.Commit(ctx)
}

func (p *Postgres) OrganizationStripeCustomer(ctx context.Context, organizationID string) (string, error) {
	var customerID string
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(stripe_customer_id,'') FROM tenancy.organizations WHERE id=$1::uuid`, organizationID).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return customerID, err
}

func (p *Postgres) SetOrganizationStripeCustomer(ctx context.Context, organizationID, customerID string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE tenancy.organizations SET stripe_customer_id=$2 WHERE id=$1::uuid`, organizationID, customerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) SubscriptionHistory(ctx context.Context, organizationID string, limit int) ([]domain.SubscriptionHistoryEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := p.pool.Query(ctx, `SELECT id::text, organization_id::text, COALESCE(stripe_subscription_id,''), plan, status, COALESCE(stripe_event_id,''), recorded_at FROM tenancy.subscription_history WHERE organization_id=$1::uuid ORDER BY recorded_at DESC LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptionHistory(rows)
}

func (p *Postgres) AdminOverview(ctx context.Context) (domain.AdminOverview, error) {
	var overview domain.AdminOverview
	err := p.pool.QueryRow(ctx, `
SELECT
  (SELECT COUNT(*) FROM tenancy.organizations),
  (SELECT COUNT(*) FROM tenancy.subscriptions WHERE status='active'),
  (SELECT COUNT(*) FROM tenancy.subscriptions WHERE status='active' AND plan='essential'),
  (SELECT COUNT(*) FROM tenancy.subscriptions WHERE status='active' AND plan='pro'),
  (SELECT COUNT(*) FROM tenancy.subscription_history)
`).Scan(&overview.Organizations, &overview.ActiveSubscriptions, &overview.EssentialPlans, &overview.ProPlans, &overview.HistoryEvents)
	return overview, err
}

func (p *Postgres) AdminOrganizations(ctx context.Context) ([]domain.AdminOrganization, error) {
	rows, err := p.pool.Query(ctx, `
SELECT o.id::text, o.name, o.status, COALESCE(s.plan,''), COALESCE(s.status,''), COUNT(m.subject_id),
  COALESCE((SELECT COUNT(*)::int FROM crm.deals d WHERE d.organization_id=o.id), 0),
  o.created_at
FROM tenancy.organizations o
LEFT JOIN tenancy.subscriptions s ON s.organization_id=o.id
LEFT JOIN tenancy.memberships m ON m.organization_id=o.id
GROUP BY o.id, o.name, o.status, s.plan, s.status, o.created_at
ORDER BY o.created_at DESC
LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.AdminOrganization{}
	for rows.Next() {
		var item domain.AdminOrganization
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.Plan, &item.SubscriptionStatus, &item.MemberCount, &item.PipelineDeals, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (p *Postgres) AdminSubscriptionHistory(ctx context.Context, limit int) ([]domain.SubscriptionHistoryEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := p.pool.Query(ctx, `
SELECT h.id::text, h.organization_id::text, o.name, COALESCE(h.stripe_subscription_id,''), h.plan, h.status, COALESCE(h.stripe_event_id,''), h.recorded_at
FROM tenancy.subscription_history h
INNER JOIN tenancy.organizations o ON o.id=h.organization_id
ORDER BY h.recorded_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.SubscriptionHistoryEntry{}
	for rows.Next() {
		var item domain.SubscriptionHistoryEntry
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.OrganizationName, &item.StripeSubscriptionID, &item.Plan, &item.Status, &item.StripeEventID, &item.RecordedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func scanSubscriptionHistory(rows pgx.Rows) ([]domain.SubscriptionHistoryEntry, error) {
	result := []domain.SubscriptionHistoryEntry{}
	for rows.Next() {
		var item domain.SubscriptionHistoryEntry
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.StripeSubscriptionID, &item.Plan, &item.Status, &item.StripeEventID, &item.RecordedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
