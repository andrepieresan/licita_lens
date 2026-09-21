CREATE TABLE IF NOT EXISTS tenancy.subscription_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    stripe_subscription_id text,
    plan text NOT NULL CHECK (plan IN ('essential', 'pro')),
    status text NOT NULL,
    stripe_event_id text,
    recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS subscription_history_org_idx
    ON tenancy.subscription_history (organization_id, recorded_at DESC);

CREATE INDEX IF NOT EXISTS subscription_history_recorded_idx
    ON tenancy.subscription_history (recorded_at DESC);
