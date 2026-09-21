CREATE SCHEMA IF NOT EXISTS procurement;

CREATE TABLE procurement.opportunities (
    id text PRIMARY KEY,
    source text NOT NULL,
    source_id text NOT NULL,
    content_hash text NOT NULL,
    object text NOT NULL,
    organization_name text NOT NULL,
    state char(2),
    municipality text,
    modality_code integer,
    estimated_value_cents bigint NOT NULL DEFAULT 0,
    published_at timestamptz,
    proposal_deadline timestamptz,
    source_updated_at timestamptz,
    source_url text,
    ingested_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source, source_id)
);

CREATE INDEX opportunities_published_idx ON procurement.opportunities(published_at DESC);
CREATE INDEX opportunities_state_deadline_idx ON procurement.opportunities(state, proposal_deadline);
CREATE INDEX opportunities_object_search_idx ON procurement.opportunities USING gin(to_tsvector('portuguese', object));

CREATE TABLE procurement.ingestion_checkpoints (
    source text NOT NULL,
    partition_key text NOT NULL,
    cursor text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, partition_key)
);
