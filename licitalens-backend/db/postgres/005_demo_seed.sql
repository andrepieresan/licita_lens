INSERT INTO tenancy.organizations (id, name, status)
VALUES ('00000000-0000-0000-0000-000000000001', 'LicitaLens Demo', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tenancy.subscriptions (organization_id, plan, status)
VALUES ('00000000-0000-0000-0000-000000000001', 'pro', 'active')
ON CONFLICT (organization_id) DO NOTHING;

INSERT INTO tenancy.memberships (organization_id, subject_id, role)
VALUES ('00000000-0000-0000-0000-000000000001', 'demo-user', 'owner')
ON CONFLICT (organization_id, subject_id) DO NOTHING;

INSERT INTO procurement.opportunities (id, source, source_id, content_hash, object, organization_name, state, municipality, modality_code, estimated_value_cents, published_at, proposal_deadline, source_updated_at, source_url)
VALUES ('demo-1', 'demo', 'demo-1', 'seed', 'Aquisição de notebooks corporativos com 16 GB de memória', 'Município de Curitiba', 'PR', 'Curitiba', 6, 18000000, now() - interval '2 hours', now() + interval '10 days', now(), 'https://pncp.gov.br/')
ON CONFLICT (id) DO NOTHING;
