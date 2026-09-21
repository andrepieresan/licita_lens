CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS tenancy;

CREATE TABLE tenancy.organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'past_due', 'canceled')),
    stripe_customer_id text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenancy.memberships (
    organization_id uuid NOT NULL REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    subject_id text NOT NULL,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'analyst', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, subject_id)
);

CREATE TABLE tenancy.subscriptions (
    organization_id uuid PRIMARY KEY REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    stripe_subscription_id text UNIQUE,
    plan text NOT NULL CHECK (plan IN ('essential', 'pro')),
    status text NOT NULL,
    current_period_end timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenancy.commercial_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL,
    keywords text[] NOT NULL DEFAULT '{}',
    categories text[] NOT NULL DEFAULT '{}',
    states text[] NOT NULL DEFAULT '{}',
    municipalities text[] NOT NULL DEFAULT '{}',
    modalities integer[] NOT NULL DEFAULT '{}',
    required_terms text[] NOT NULL DEFAULT '{}',
    excluded_terms text[] NOT NULL DEFAULT '{}',
    minimum_value_cents bigint NOT NULL DEFAULT 0,
    maximum_value_cents bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX commercial_profiles_organization_idx ON tenancy.commercial_profiles(organization_id);

CREATE TABLE tenancy.webhook_events (
    provider text NOT NULL,
    external_id text NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    payload_hash text NOT NULL,
    PRIMARY KEY (provider, external_id)
);

CREATE TABLE tenancy.usage_counters (
    organization_id uuid NOT NULL REFERENCES tenancy.organizations(id) ON DELETE CASCADE,
    period_start date NOT NULL,
    kind text NOT NULL,
    used integer NOT NULL DEFAULT 0 CHECK (used >= 0),
    PRIMARY KEY (organization_id, period_start, kind)
);
