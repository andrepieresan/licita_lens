CREATE TABLE IF NOT EXISTS tenancy.notification_scan (
    organization_id uuid PRIMARY KEY REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    last_opportunity_at timestamptz NOT NULL DEFAULT '1970-01-01T00:00:00Z'
);
