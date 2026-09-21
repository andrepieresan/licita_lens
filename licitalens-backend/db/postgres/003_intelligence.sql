CREATE EXTENSION IF NOT EXISTS vector;
CREATE SCHEMA IF NOT EXISTS intelligence;

CREATE TABLE intelligence.opportunity_embeddings (
    opportunity_id text PRIMARY KEY,
    provider text NOT NULL,
    model text NOT NULL,
    embedding vector,
    content_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE intelligence.matches (
    organization_id uuid NOT NULL,
    profile_id uuid NOT NULL,
    opportunity_id text NOT NULL,
    score smallint NOT NULL CHECK (score BETWEEN 0 AND 100),
    breakdown jsonb NOT NULL,
    evidence jsonb NOT NULL,
    explanation text,
    prompt_version text,
    calculated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, opportunity_id)
);

CREATE INDEX matches_feed_idx ON intelligence.matches(organization_id, score DESC, calculated_at DESC);

CREATE TABLE intelligence.usage_ledger (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    capability text NOT NULL,
    units integer NOT NULL CHECK (units > 0),
    provider text,
    input_tokens integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
