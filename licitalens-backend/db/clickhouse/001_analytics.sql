CREATE DATABASE IF NOT EXISTS licitalens;

CREATE TABLE IF NOT EXISTS licitalens.procurement_facts (
    opportunity_id String,
    item_code LowCardinality(String),
    state FixedString(2),
    supplier_document String,
    supplier_name String,
    unit_price_cents Int64,
    quantity Decimal(18, 4),
    published_at DateTime64(3, 'UTC'),
    source_updated_at DateTime64(3, 'UTC')
) ENGINE = ReplacingMergeTree(source_updated_at)
PARTITION BY toYYYYMM(published_at)
ORDER BY (item_code, state, published_at, opportunity_id);
