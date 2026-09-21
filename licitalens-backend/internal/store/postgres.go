package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}
func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}
func (p *Postgres) IngestionCheckpoint(ctx context.Context, source, partition string) (string, error) {
	var cursor *string
	err := p.pool.QueryRow(ctx, `SELECT cursor FROM procurement.ingestion_checkpoints WHERE source=$1 AND partition_key=$2`, source, partition).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if cursor == nil {
		return "", err
	}
	return *cursor, err
}
func (p *Postgres) SaveIngestionCheckpoint(ctx context.Context, source, partition, cursor string) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO procurement.ingestion_checkpoints(source,partition_key,cursor) VALUES($1,$2,$3) ON CONFLICT(source,partition_key) DO UPDATE SET cursor=EXCLUDED.cursor,updated_at=now()`, source, partition, cursor)
	return err
}

func (p *Postgres) IsMember(ctx context.Context, organizationID, subjectID string) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenancy.memberships WHERE organization_id=$1::uuid AND subject_id=$2)`, organizationID, subjectID).Scan(&exists)
	return exists, err
}

func (p *Postgres) Subscription(ctx context.Context, organizationID string) (billing.Subscription, error) {
	var value billing.Subscription
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(stripe_subscription_id,''), plan, status, current_period_end, updated_at FROM tenancy.subscriptions WHERE organization_id=$1::uuid`, organizationID).Scan(&value.ExternalID, &value.Plan, &value.Status, &value.CurrentPeriodEnd, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, ErrNotFound
	}
	return value, err
}

func (p *Postgres) ApplySubscription(ctx context.Context, organizationID string, subscription billing.Subscription, eventID, payloadHash string) (bool, error) {
	if _, ok := billing.Plan(subscription.Plan); !ok || subscription.ExternalID == "" || eventID == "" || subscription.UpdatedAt.IsZero() {
		return false, errors.New("invalid subscription update")
	}
	if subscription.Status != "active" && subscription.Status != "past_due" && subscription.Status != "canceled" && subscription.Status != "trialing" {
		return false, errors.New("invalid subscription status")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var inserted bool
	err = tx.QueryRow(ctx, `INSERT INTO tenancy.webhook_events(provider, external_id, payload_hash) VALUES ('stripe',$1,$2) ON CONFLICT DO NOTHING RETURNING true`, eventID, payloadHash).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	var updated bool
	err = tx.QueryRow(ctx, `INSERT INTO tenancy.subscriptions(organization_id,stripe_subscription_id,plan,status,current_period_end,updated_at) VALUES ($1::uuid,NULLIF($2,''),$3,$4,$5,$6) ON CONFLICT(organization_id) DO UPDATE SET stripe_subscription_id=EXCLUDED.stripe_subscription_id,plan=EXCLUDED.plan,status=EXCLUDED.status,current_period_end=EXCLUDED.current_period_end,updated_at=EXCLUDED.updated_at WHERE tenancy.subscriptions.updated_at < EXCLUDED.updated_at RETURNING true`, organizationID, subscription.ExternalID, subscription.Plan, subscription.Status, subscription.CurrentPeriodEnd, subscription.UpdatedAt).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `UPDATE tenancy.webhook_events SET processed_at=now() WHERE provider='stripe' AND external_id=$1`, eventID)
		if err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tenancy.subscription_history(organization_id,stripe_subscription_id,plan,status,stripe_event_id,recorded_at) VALUES ($1::uuid,NULLIF($2,''),$3,$4,$5,$6)`, organizationID, subscription.ExternalID, subscription.Plan, subscription.Status, eventID, subscription.UpdatedAt)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE tenancy.organizations SET status=$2 WHERE id=$1::uuid`, organizationID, organizationStatus(subscription.Status))
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE tenancy.webhook_events SET processed_at=now() WHERE provider='stripe' AND external_id=$1`, eventID)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (p *Postgres) ConsumeUsage(ctx context.Context, organizationID, period, kind string, limit int) (bool, int, error) {
	var used int
	err := p.pool.QueryRow(ctx, `INSERT INTO tenancy.usage_counters(organization_id,period_start,kind,used) VALUES ($1::uuid,$2::date,$3,1) ON CONFLICT(organization_id,period_start,kind) DO UPDATE SET used=tenancy.usage_counters.used+1 WHERE tenancy.usage_counters.used < $4 RETURNING used`, organizationID, period, kind, limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, limit, nil
	}
	return err == nil, used, err
}

func (p *Postgres) PutProfile(profile domain.CommercialProfile) error {
	profile = normalizeCommercialProfile(profile)
	_, err := p.pool.Exec(context.Background(), `INSERT INTO tenancy.commercial_profiles (id,organization_id,name,description,keywords,categories,states,municipalities,modalities,required_terms,excluded_terms,minimum_value_cents,maximum_value_cents,created_at) VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,keywords=EXCLUDED.keywords,categories=EXCLUDED.categories,states=EXCLUDED.states,municipalities=EXCLUDED.municipalities,modalities=EXCLUDED.modalities,required_terms=EXCLUDED.required_terms,excluded_terms=EXCLUDED.excluded_terms,minimum_value_cents=EXCLUDED.minimum_value_cents,maximum_value_cents=EXCLUDED.maximum_value_cents`, profile.ID, profile.OrganizationID, profile.Name, profile.Description, profile.Keywords, profile.Categories, profile.States, profile.Municipalities, profile.Modalities, profile.RequiredTerms, profile.ExcludedTerms, profile.MinimumValueCents, profile.MaximumValueCents, profile.CreatedAt)
	return err
}

func normalizeCommercialProfile(profile domain.CommercialProfile) domain.CommercialProfile {
	if profile.Keywords == nil {
		profile.Keywords = []string{}
	}
	if profile.Categories == nil {
		profile.Categories = []string{}
	}
	if profile.States == nil {
		profile.States = []string{}
	}
	if profile.Municipalities == nil {
		profile.Municipalities = []string{}
	}
	if profile.Modalities == nil {
		profile.Modalities = []int{}
	}
	if profile.RequiredTerms == nil {
		profile.RequiredTerms = []string{}
	}
	if profile.ExcludedTerms == nil {
		profile.ExcludedTerms = []string{}
	}
	return profile
}
func (p *Postgres) Profiles(ctx context.Context, organizationID string) ([]domain.CommercialProfile, error) {
	rows, err := p.pool.Query(ctx, profileQuery+` WHERE organization_id=$1 ORDER BY created_at`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values, err := scanProfiles(rows)
	return values, err
}
func (p *Postgres) Profile(id, organizationID string) (domain.CommercialProfile, error) {
	row := p.pool.QueryRow(context.Background(), profileQuery+` WHERE id=$1 AND organization_id=$2`, id, organizationID)
	return scanProfile(row)
}
func (p *Postgres) DeleteProfile(id, organizationID string) error {
	tag, err := p.pool.Exec(context.Background(), `DELETE FROM tenancy.commercial_profiles WHERE id=$1 AND organization_id=$2`, id, organizationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) PutOpportunity(opportunity domain.Opportunity) error {
	_, err := p.pool.Exec(context.Background(), `INSERT INTO procurement.opportunities (id,source,source_id,content_hash,object,organization_name,state,municipality,modality_code,estimated_value_cents,published_at,proposal_deadline,source_updated_at,source_url) VALUES ($1,$2,$3,'event',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object,organization_name=EXCLUDED.organization_name,state=EXCLUDED.state,municipality=EXCLUDED.municipality,estimated_value_cents=EXCLUDED.estimated_value_cents,published_at=EXCLUDED.published_at,proposal_deadline=EXCLUDED.proposal_deadline,source_updated_at=EXCLUDED.source_updated_at,source_url=EXCLUDED.source_url`, opportunity.ID, opportunity.Source, opportunity.SourceID, opportunity.Object, opportunity.OrganizationName, opportunity.State, opportunity.Municipality, opportunity.ModalityCode, opportunity.EstimatedValueCents, nullableTime(opportunity.PublishedAt), nullableTime(opportunity.ProposalDeadline), nullableTime(opportunity.UpdatedAt), opportunity.SourceURL)
	return err
}
func (p *Postgres) Opportunity(id string) (domain.Opportunity, error) {
	row := p.pool.QueryRow(context.Background(), opportunityQuery+` WHERE id=$1`, id)
	return scanOpportunity(row)
}
func (p *Postgres) Opportunities(ctx context.Context) ([]domain.Opportunity, error) {
	all := []domain.Opportunity{}
	for offset := 0; ; offset += 100 {
		page, more, err := p.OpportunitiesPage(ctx, offset, 100)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if !more {
			return all, nil
		}
	}
}

func (p *Postgres) OpportunitiesPage(ctx context.Context, offset, limit int) ([]domain.Opportunity, bool, error) {
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 100 {
		return nil, false, errors.New("invalid opportunity page limit")
	}
	rows, err := p.pool.Query(ctx, opportunityQuery+` ORDER BY published_at DESC NULLS LAST, id DESC LIMIT $1 OFFSET $2`, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []domain.Opportunity{}
	for rows.Next() {
		value, err := scanOpportunity(rows)
		if err != nil {
			return nil, false, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

const profileQuery = `SELECT id::text,organization_id::text,name,description,keywords,categories,states,municipalities,modalities,required_terms,excluded_terms,minimum_value_cents,maximum_value_cents,created_at FROM tenancy.commercial_profiles`
const opportunityQuery = `SELECT id,source,source_id,object,organization_name,state,municipality,modality_code,estimated_value_cents,published_at,proposal_deadline,source_updated_at,source_url FROM procurement.opportunities`

type rowScanner interface{ Scan(...any) error }

func scanProfile(row rowScanner) (domain.CommercialProfile, error) {
	var v domain.CommercialProfile
	err := row.Scan(&v.ID, &v.OrganizationID, &v.Name, &v.Description, &v.Keywords, &v.Categories, &v.States, &v.Municipalities, &v.Modalities, &v.RequiredTerms, &v.ExcludedTerms, &v.MinimumValueCents, &v.MaximumValueCents, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func scanProfiles(rows pgx.Rows) ([]domain.CommercialProfile, error) {
	result := []domain.CommercialProfile{}
	for rows.Next() {
		value, err := scanProfile(rows)
		if err == nil {
			result = append(result, value)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
func scanOpportunity(row rowScanner) (domain.Opportunity, error) {
	var v domain.Opportunity
	err := row.Scan(&v.ID, &v.Source, &v.SourceID, &v.Object, &v.OrganizationName, &v.State, &v.Municipality, &v.ModalityCode, &v.EstimatedValueCents, &v.PublishedAt, &v.ProposalDeadline, &v.UpdatedAt, &v.SourceURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func organizationStatus(subscriptionStatus string) string {
	switch subscriptionStatus {
	case "active", "trialing":
		return "active"
	case "past_due":
		return "past_due"
	default:
		return "canceled"
	}
}
