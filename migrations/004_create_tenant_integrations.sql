CREATE TABLE tenant_integrations (
    id                     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id              UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_type            TEXT NOT NULL,
    credentials_encrypted  BYTEA NOT NULL,
    field_mapping          JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    last_sync_at           TIMESTAMPTZ,
    last_sync_status       TEXT,
    last_sync_error        TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, source_type)
);

CREATE INDEX idx_integrations_tenant ON tenant_integrations(tenant_id);
CREATE INDEX idx_integrations_enabled ON tenant_integrations(enabled) WHERE enabled = TRUE;
