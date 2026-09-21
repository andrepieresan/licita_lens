CREATE SCHEMA IF NOT EXISTS crm;

CREATE TABLE IF NOT EXISTS crm.deals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    opportunity_id text,
    title text NOT NULL,
    buyer_name text NOT NULL DEFAULT '',
    stage text NOT NULL CHECK (stage IN ('prospecting', 'analysis', 'proposal', 'negotiation', 'won', 'lost')),
    estimated_value_cents bigint NOT NULL DEFAULT 0,
    next_follow_up_at timestamptz,
    closed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deals_organization_stage_idx ON crm.deals (organization_id, stage);

CREATE TABLE IF NOT EXISTS crm.deal_followups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deal_id uuid NOT NULL REFERENCES crm.deals(id) ON DELETE CASCADE,
    note text NOT NULL,
    scheduled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deal_followups_deal_idx ON crm.deal_followups (deal_id, created_at DESC);
