CREATE SCHEMA IF NOT EXISTS notifications;

CREATE TABLE notifications.channels (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    type text NOT NULL CHECK (type IN ('expo_push', 'email', 'whatsapp')),
    destination text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    consented_at timestamptz,
    opted_out_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, type, destination)
);

CREATE TABLE notifications.deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    channel_id uuid NOT NULL REFERENCES notifications.channels(id),
    opportunity_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'suppressed')),
    attempts integer NOT NULL DEFAULT 0,
    provider_message_id text,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (channel_id, opportunity_id)
);
