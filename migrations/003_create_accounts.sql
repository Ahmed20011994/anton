CREATE TABLE accounts (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_id      TEXT NOT NULL,
    name           TEXT,
    tier           TEXT,
    arr            NUMERIC(12,2),
    renewal_date   DATE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, source_id)
);

CREATE INDEX idx_accounts_tenant ON accounts(tenant_id);
