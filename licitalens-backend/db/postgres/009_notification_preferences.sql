ALTER TABLE tenancy.organizations
  ADD COLUMN IF NOT EXISTS notification_preferences jsonb NOT NULL DEFAULT '{"push":true,"email":true,"whatsapp":false,"deadline_reminder":true}'::jsonb;

CREATE UNIQUE INDEX IF NOT EXISTS deals_organization_opportunity_uidx
  ON crm.deals (organization_id, opportunity_id)
  WHERE opportunity_id IS NOT NULL AND opportunity_id <> '';
